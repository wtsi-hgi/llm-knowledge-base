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
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"

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
		coreFactoryCalls := 0
		previousNewCoreServer := newCoreServer
		newCoreServer = func(core.Options) (coreServer, error) {
			coreFactoryCalls++

			return nil, nil
		}

		done := make(chan error, 1)

		Reset(func() {
			newCoreServer = previousNewCoreServer
		})

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

		Convey("A1.4: it opens neither stdio nor HTTP serving", func() {
			So(settled, ShouldBeTrue)
			So(coreFactoryCalls, ShouldEqual, 0)
		})
	})
}

func TestDefaultTransportResolution(t *testing.T) {
	Convey("A1.1: Given no --http flag and MLWH_HTTP_ADDR is empty", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", "")

		cfg, showVersion, err := parseArgs([]string{})

		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)
		So(cfg.HTTPAddr, ShouldEqual, "")
		So(cfg.Transport, ShouldEqual, transportModeStdio)
	})
}

func TestHTTPTransportResolution(t *testing.T) {
	Convey("A2.1: Given MLWH_HTTP_ADDR is set and no --http flag", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", ":8080")

		cfg, showVersion, err := parseArgs([]string{})

		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)
		So(cfg.HTTPAddr, ShouldEqual, ":8080")
		So(cfg.Transport, ShouldEqual, transportModeHTTP)
	})

	Convey("A2.2: Given --http is set and MLWH_HTTP_ADDR is also set", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", ":8080")

		cfg, showVersion, err := parseArgs([]string{"--http", "127.0.0.1:9090"})

		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)
		So(cfg.HTTPAddr, ShouldEqual, "127.0.0.1:9090")
		So(cfg.Transport, ShouldEqual, transportModeHTTP)
	})

	Convey("A2.3: Given --http is present with an empty value and MLWH_HTTP_ADDR is also set", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", ":8080")

		cfg, showVersion, err := parseArgs([]string{"--http", ""})

		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)
		So(cfg.HTTPAddr, ShouldEqual, "")
		So(cfg.Transport, ShouldEqual, transportModeStdio)
	})
}

type recordingCoreServer struct {
	ctx          context.Context
	httpOpts     core.HTTPOptions
	runCalls     int
	runHTTPCalls int
	started      chan struct{}
	startedOnce  sync.Once
	transport    mcp.Transport
}

func (s *recordingCoreServer) Run(ctx context.Context, transport mcp.Transport) error {
	s.ctx = ctx
	s.transport = transport
	s.runCalls++
	s.startedOnce.Do(func() { close(s.started) })
	<-ctx.Done()

	return nil
}

func (s *recordingCoreServer) RunHTTP(ctx context.Context, opts core.HTTPOptions) error {
	s.ctx = ctx
	s.httpOpts = opts
	s.runHTTPCalls++
	s.startedOnce.Do(func() { close(s.started) })
	<-ctx.Done()

	return nil
}

func TestServeDefaultsToStdio(t *testing.T) {
	Convey("A1.2: Given valid MLWH config with no --http flag and no MLWH_HTTP_ADDR", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", "")

		cfg, showVersion, err := parseArgs([]string{"--mlwh-base-url=http://stub.example"})
		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)

		signalCtx, cancelSignal := context.WithCancel(context.Background())
		stopCalls := 0
		previousSignalNotifyContext := signalNotifyContext
		signalNotifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return signalCtx, func() {
				stopCalls++
			}
		}

		fakeServer := &recordingCoreServer{
			started: make(chan struct{}),
		}
		factoryCalls := 0
		var capturedOptions core.Options
		previousNewCoreServer := newCoreServer
		newCoreServer = func(opts core.Options) (coreServer, error) {
			factoryCalls++
			capturedOptions = opts

			return fakeServer, nil
		}

		done := make(chan error, 1)

		Reset(func() {
			cancelSignal()
			signalNotifyContext = previousSignalNotifyContext
			newCoreServer = previousNewCoreServer
		})

		go func() {
			done <- serve(cfg)
		}()

		waitForCommandSignal(t, fakeServer.started, 3*time.Second, "stdio Run did not start")
		cancelSignal()

		err, settled := waitForCommandResult(done, 3*time.Second)
		_, isStdio := fakeServer.transport.(*mcp.StdioTransport)

		So(settled, ShouldBeTrue)
		So(err, ShouldBeNil)
		So(factoryCalls, ShouldEqual, 1)
		So(len(capturedOptions.Providers), ShouldEqual, 1)
		So(capturedOptions.Providers[0].Name(), ShouldEqual, "mlwh")
		So(fakeServer.ctx, ShouldEqual, signalCtx)
		So(fakeServer.runCalls, ShouldEqual, 1)
		So(fakeServer.runHTTPCalls, ShouldEqual, 0)
		So(isStdio, ShouldBeTrue)
		So(stopCalls, ShouldEqual, 1)
	})
}

func TestServeSelectsHTTP(t *testing.T) {
	Convey("A2.4: Given resolved HTTP address 127.0.0.1:0", t, func() {
		t.Setenv("MLWH_HTTP_ADDR", "")

		cfg, showVersion, err := parseArgs([]string{
			"--mlwh-base-url=http://stub.example",
			"--http", "127.0.0.1:0",
		})
		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)

		signalCtx, cancelSignal := context.WithCancel(context.Background())
		stopCalls := 0
		previousSignalNotifyContext := signalNotifyContext
		signalNotifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return signalCtx, func() {
				stopCalls++
			}
		}

		fakeServer := &recordingCoreServer{
			started: make(chan struct{}),
		}
		factoryCalls := 0
		previousNewCoreServer := newCoreServer
		newCoreServer = func(core.Options) (coreServer, error) {
			factoryCalls++

			return fakeServer, nil
		}

		done := make(chan error, 1)

		Reset(func() {
			cancelSignal()
			signalNotifyContext = previousSignalNotifyContext
			newCoreServer = previousNewCoreServer
		})

		go func() {
			done <- serve(cfg)
		}()

		waitForCommandSignal(t, fakeServer.started, 3*time.Second, "HTTP RunHTTP did not start")
		cancelSignal()

		err, settled := waitForCommandResult(done, 3*time.Second)

		So(settled, ShouldBeTrue)
		So(err, ShouldBeNil)
		So(factoryCalls, ShouldEqual, 1)
		So(fakeServer.ctx, ShouldEqual, signalCtx)
		So(fakeServer.runCalls, ShouldEqual, 0)
		So(fakeServer.runHTTPCalls, ShouldEqual, 1)
		So(fakeServer.httpOpts, ShouldResemble, core.HTTPOptions{
			Addr:       "127.0.0.1:0",
			MCPPath:    "/mcp",
			HealthPath: "/health",
		})
		So(stopCalls, ShouldEqual, 1)
	})
}

type testCmdProvider struct{}

func (testCmdProvider) Name() string { return "test" }

func (testCmdProvider) APIVersion() string { return "test 1.0.0" }

func (testCmdProvider) Register(context.Context, core.Registrar) error { return nil }

// TestMaxToolResultBytesConfig exercises command wiring: the MLWH byte-limit
// flag resolves into core.Options, and an invalid environment fallback aborts
// startup with the offending variable named before any stdio serving begins.
func TestMaxToolResultBytesConfig(t *testing.T) {
	Convey("Given --mlwh-max-tool-result-bytes=2048", t, func() {
		cfg, showVersion, err := parseArgs([]string{"--mlwh-max-tool-result-bytes=2048"})
		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)

		maxBytes, err := cfg.MLWH.ResolveMaxToolResultBytes(func(string) string { return "" })
		So(err, ShouldBeNil)

		opts := coreOptions(testCmdProvider{}, maxBytes)

		Convey("the resolved core options receive MaxToolResultBytes=2048", func() {
			So(opts.MaxToolResultBytes, ShouldEqual, 2048)
		})
	})

	Convey("Given env MLWH_MAX_TOOL_RESULT_BYTES=bad", t, func() {
		t.Setenv("MLWH_MAX_TOOL_RESULT_BYTES", "bad")
		t.Setenv("MLWH_HTTP_ADDR", "")

		cfg, showVersion, err := parseArgs([]string{})
		So(err, ShouldBeNil)
		So(showVersion, ShouldBeFalse)

		coreFactoryCalls := 0
		previousNewCoreServer := newCoreServer
		newCoreServer = func(core.Options) (coreServer, error) {
			coreFactoryCalls++

			return &recordingCoreServer{started: make(chan struct{})}, nil
		}

		done := make(chan error, 1)

		Reset(func() {
			newCoreServer = previousNewCoreServer
		})

		go func() {
			done <- serve(cfg)
		}()

		var (
			runErr  error
			settled bool
		)

		select {
		case runErr = <-done:
			settled = true
		case <-time.After(3 * time.Second):
			settled = false
		}

		Convey("A1.3: startup returns an error mentioning MLWH_MAX_TOOL_RESULT_BYTES before stdio starts", func() {
			So(settled, ShouldBeTrue)
			So(runErr, ShouldNotBeNil)
			So(runErr.Error(), ShouldContainSubstring, "MLWH_MAX_TOOL_RESULT_BYTES")
			So(coreFactoryCalls, ShouldEqual, 0)
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
