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

package main

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
	"github.com/wtsi-hgi/llm-knowledge-base/internal/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// TestRunVersionFlag exercises Story G4: `mlwh-mcp-server --version` prints the
// server version and the targeted MLWH API version to stdout and exits 0 without
// opening the transport, requiring config, or blocking on stdin. It drives the
// refactored run(args, stdout) directly so no subprocess or real stdio is
// involved, and so the no-config requirement is exercised (no MLWH_BASE_URL is
// set).
func TestRunVersionFlag(t *testing.T) {
	Convey("Given --version and a captured stdout, with no MLWH_BASE_URL configured", t, func() {
		t.Setenv("MLWH_BASE_URL", "")

		var stdout bytes.Buffer

		done := make(chan error, 1)

		go func() {
			done <- run([]string{"--version"}, &stdout)
		}()

		var (
			err     error
			prompt  bool
			settled bool
		)

		select {
		case err = <-done:
			prompt = true
			settled = true
		case <-time.After(3 * time.Second):
			settled = false
		}

		Convey("G4.2: it returns promptly without blocking on stdin or serving", func() {
			So(settled, ShouldBeTrue)
			So(prompt, ShouldBeTrue)
		})

		Convey("G4.1: it returns nil (exit 0) and prints both versions to stdout", func() {
			So(settled, ShouldBeTrue)
			So(err, ShouldBeNil)

			out := stdout.String()
			So(out, ShouldContainSubstring, core.ServerVersion)
			So(out, ShouldContainSubstring, wa.APIVersion)
		})
	})
}

type testCmdProvider struct{}

func (testCmdProvider) Name() string { return "test" }

func (testCmdProvider) APIVersion() string { return "test 1.0.0" }

func (testCmdProvider) Register(context.Context, core.Registrar) error { return nil }

// TestMaxToolResultBytesConfig exercises A2 command wiring: the MLWH byte-limit
// flag resolves into core.Options, and an invalid environment fallback aborts
// startup with the offending variable named before any stdio serving begins.
func TestMaxToolResultBytesConfig(t *testing.T) {
	Convey("Given --mlwh-max-tool-result-bytes=2048", t, func() {
		cfg, showVersion, err := parseArgs([]string{"--mlwh-max-tool-result-bytes=2048"})
		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)

		maxBytes, err := cfg.ResolveMaxToolResultBytes(func(string) string { return "" })
		So(err, ShouldBeNil)

		opts := coreOptions(testCmdProvider{}, maxBytes)

		Convey("A2.3: the resolved core options receive MaxToolResultBytes=2048", func() {
			So(opts.MaxToolResultBytes, ShouldEqual, 2048)
		})
	})

	Convey("Given env MLWH_MAX_TOOL_RESULT_BYTES=bad", t, func() {
		t.Setenv("MLWH_MAX_TOOL_RESULT_BYTES", "bad")

		done := make(chan error, 1)

		go func() {
			done <- serve(mlwh.Config{})
		}()

		var (
			err     error
			settled bool
		)

		select {
		case err = <-done:
			settled = true
		case <-time.After(3 * time.Second):
			settled = false
		}

		Convey("A2.4: startup returns an error mentioning MLWH_MAX_TOOL_RESULT_BYTES", func() {
			So(settled, ShouldBeTrue)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "MLWH_MAX_TOOL_RESULT_BYTES")
		})
	})
}

type recordingHTTPCoreServer struct {
	ctx     context.Context
	opts    core.HTTPOptions
	started chan struct{}
}

func (s *recordingHTTPCoreServer) RunHTTP(ctx context.Context, opts core.HTTPOptions) error {
	s.ctx = ctx
	s.opts = opts
	close(s.started)
	<-ctx.Done()

	return nil
}

func TestServeHTTPUsesSignalNotifyContext(t *testing.T) {
	Convey("B3.5: Given HTTP serving uses an injected signal.NotifyContext wrapper", t, func() {
		signalCtx, cancelSignal := context.WithCancel(context.Background())
		stopCalls := 0
		previousSignalNotifyContext := signalNotifyContext
		signalNotifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return signalCtx, func() {
				stopCalls++
			}
		}

		fakeServer := &recordingHTTPCoreServer{
			started: make(chan struct{}),
		}
		done := make(chan error, 1)

		Reset(func() {
			cancelSignal()
			signalNotifyContext = previousSignalNotifyContext
		})

		go func() {
			done <- serveHTTP(fakeServer, core.HTTPOptions{
				Addr:       "127.0.0.1:0",
				MCPPath:    "/mcp",
				HealthPath: "/health",
			})
		}()

		waitForCommandSignal(t, fakeServer.started, 3*time.Second, "RunHTTP did not start")
		cancelSignal()

		err, settled := waitForCommandResult(done, 3*time.Second)

		So(fakeServer.ctx, ShouldEqual, signalCtx)
		So(settled, ShouldBeTrue)
		So(err, ShouldBeNil)
		So(stopCalls, ShouldEqual, 1)
	})
}

func waitForCommandSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func waitForCommandResult(results <-chan error, timeout time.Duration) (error, bool) {
	select {
	case err := <-results:
		return err, true
	case <-time.After(timeout):
		return nil, false
	}
}
