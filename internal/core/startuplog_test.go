/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sendu Bala <sb10@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package core_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"

	. "github.com/smartystreets/goconvey/convey"
)

// syncBuffer is a bytes.Buffer guarded by a mutex so the slog handler goroutine
// (writing the startup line during Run) and the test goroutine (reading it after
// a round-trip) never race on the underlying buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// TestStartupLogVersionLine exercises Story G5: when Run reaches the serving
// phase the core emits one startup log line, through the configured logger, that
// names this server's version and each provider's targeted upstream API version.
// It runs the real MLWH provider over an in-memory transport with a
// buffer-backed slog logger and asserts the buffer captured both versions.
func TestStartupLogVersionLine(t *testing.T) {
	Convey("Given a core server with ServerVersion 0.1.0, the MLWH provider, and a "+
		"buffer-backed logger", t, func() {
		provider := newMLWHProvider(t)

		buf := &syncBuffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))

		srv, err := core.New(core.Options{
			ServerVersion: "0.1.0",
			Logger:        logger,
			Providers:     []core.Provider{provider},
		})
		So(err, ShouldBeNil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serverTransport, clientTransport := mcp.NewInMemoryTransports()

		runErr := make(chan error, 1)

		go func() {
			runErr <- srv.Run(ctx, serverTransport)
		}()

		client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)

		clientSession, err := client.Connect(ctx, clientTransport, nil)
		So(err, ShouldBeNil)

		// A successful round-trip proves Run reached the serving phase, so the
		// startup line (emitted synchronously before serving begins) is in the
		// buffer by now.
		_, err = clientSession.ListTools(ctx, &mcp.ListToolsParams{})
		So(err, ShouldBeNil)

		Convey("G5.1: the buffer contains a startup line naming both versions", func() {
			logged := buf.String()
			So(logged, ShouldContainSubstring, "0.1.0")
			So(logged, ShouldContainSubstring, wa.APIVersion)
		})

		Reset(func() {
			_ = clientSession.Close()
			cancel()
			<-runErr
		})
	})
}

func TestHTTPStartupLogVersionLine(t *testing.T) {
	Convey("Given HTTP mode with ServerVersion 0.1.0, the MLWH provider, and a "+
		"buffer-backed logger", t, func() {
		provider := newMLWHProvider(t)

		buf := &syncBuffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))

		srv, err := core.New(core.Options{
			ServerVersion: "0.1.0",
			Logger:        logger,
			Providers:     []core.Provider{provider},
		})
		So(err, ShouldBeNil)

		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)
		runSettled := false

		go func() {
			runErr <- srv.RunHTTP(ctx, core.HTTPOptions{
				Addr:      "127.0.0.1:0",
				LogWriter: io.Discard,
			})
		}()

		Reset(func() {
			cancel()
			if !runSettled {
				_, _ = waitForRunHTTPResult(t, runErr, 5*time.Second)
			}
		})

		logged := waitForStartupLog(t, buf, "health_path=/health", 5*time.Second)
		cancel()
		err, settled := waitForRunHTTPResult(t, runErr, 5*time.Second)
		runSettled = true

		So(settled, ShouldBeTrue)
		So(err, ShouldBeNil)

		Convey("C2.4: the buffer contains the HTTP startup fields and version fields", func() {
			So(logged, ShouldContainSubstring, "server_version=0.1.0")
			So(logged, ShouldContainSubstring, "api_versions.mlwh")
			So(logged, ShouldContainSubstring, wa.APIVersion)
			So(logged, ShouldContainSubstring, "transport=http")
			So(logged, ShouldContainSubstring, "addr=127.0.0.1:0")
			So(logged, ShouldContainSubstring, "mcp_path=/mcp")
			So(logged, ShouldContainSubstring, "health_path=/health")
		})
	})
}

func waitForRunHTTPResult(t *testing.T, results <-chan error, timeout time.Duration) (error, bool) {
	t.Helper()

	select {
	case err := <-results:
		return err, true
	case <-time.After(timeout):
		t.Errorf("RunHTTP did not return after context cancellation")

		return nil, false
	}
}

func waitForStartupLog(t *testing.T, buf *syncBuffer, substring string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		logged := buf.String()
		if strings.Contains(logged, substring) {
			return logged
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for startup log containing %q", substring)

	return ""
}
