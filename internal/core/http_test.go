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

package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go/parser"
	"go/token"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
	gas "github.com/wtsi-hgi/go-authserver"
)

var errUnexpectedStructuredContent = errors.New("unexpected structured content")

type httpPingProvider struct{}

func (httpPingProvider) Name() string { return "testsvc" }

func (httpPingProvider) APIVersion() string { return "TESTAPI 9.9.9" }

func (httpPingProvider) Register(_ context.Context, r Registrar) error {
	type pingInput struct{}

	type pingOutput struct {
		Message string `json:"message" jsonschema:"a fixed ping acknowledgement"`
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        "test_ping",
		Description: "returns a fixed pong message",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ pingInput) (*mcp.CallToolResult, pingOutput, error) {
		return nil, pingOutput{Message: "pong"}, nil
	})

	return nil
}

func TestStreamableHTTPHandlerServesSharedCore(t *testing.T) {
	Convey("Given a core HTTP server with a fake test_ping provider", t, func() {
		httpServer, authServer, cleanup := runHTTPTestServer(t, []Provider{httpPingProvider{}})
		defer cleanup()

		firstSession, firstCleanup := connectHTTPMCPClient(t, httpServer.URL+"/mcp")
		defer firstCleanup()

		Convey("B1.1: a streamable HTTP client lists exactly the provider tool", func() {
			res, err := firstSession.ListTools(context.Background(), &mcp.ListToolsParams{})
			So(err, ShouldBeNil)
			So(res.Tools, ShouldHaveLength, 1)
			So(res.Tools[0].Name, ShouldEqual, "test_ping")
		})

		Convey("B1.2: the version resource is readable over streamable HTTP", func() {
			res, err := firstSession.ReadResource(context.Background(), &mcp.ReadResourceParams{
				URI: "mcp-server://version",
			})
			So(err, ShouldBeNil)
			So(res.Contents, ShouldHaveLength, 1)

			var version VersionInfo
			err = json.Unmarshal([]byte(res.Contents[0].Text), &version)
			So(err, ShouldBeNil)
			So(version.ServerVersion, ShouldEqual, "0.1.0")
			So(version.APIVersions["testsvc"], ShouldEqual, "TESTAPI 9.9.9")
		})

		Convey("B1.3: two HTTP client sessions can concurrently call test_ping", func() {
			secondSession, secondCleanup := connectHTTPMCPClient(t, httpServer.URL+"/mcp")
			defer secondCleanup()

			results := callPingConcurrently(t, firstSession, secondSession)

			So(results, ShouldHaveLength, 2)
			So(results[0].err, ShouldBeNil)
			So(results[1].err, ShouldBeNil)
			So(results[0].message, ShouldEqual, "pong")
			So(results[1].message, ShouldEqual, "pong")
		})

		Reset(func() {
			authServer.Stop()
		})
	})
}

func runHTTPTestServer(t *testing.T, providers []Provider) (*httptest.Server, httpAuthServer, func()) {
	t.Helper()

	srv, err := New(Options{
		ServerVersion: "0.1.0",
		Logger:        discardTestLogger(),
		Providers:     providers,
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	authServer, _, err := srv.buildHTTPServer(context.Background(), HTTPOptions{
		MCPPath:    "/mcp",
		HealthPath: "/health",
		LogWriter:  io.Discard,
	})
	if err != nil {
		t.Fatalf("buildHTTPServer() returned error: %v", err)
	}

	httpServer := httptest.NewServer(authServer.Router())

	cleanup := func() {
		httpServer.Close()
		authServer.Stop()
	}

	return httpServer, authServer, cleanup
}

func discardTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func connectHTTPMCPClient(t *testing.T, endpoint string) (*mcp.ClientSession, func()) {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	session, err := client.Connect(ctx, transport, nil)
	cancel()

	if err != nil {
		t.Fatalf("client Connect() returned error: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
	}

	return session, cleanup
}

func callPingConcurrently(
	t *testing.T,
	firstSession *mcp.ClientSession,
	secondSession *mcp.ClientSession,
) []pingCallResult {
	t.Helper()

	results := make(chan pingCallResult, 2)
	var wg sync.WaitGroup

	call := func(session *mcp.ClientSession) {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "test_ping",
			Arguments: map[string]any{},
		})
		if err != nil {
			results <- pingCallResult{err: err}

			return
		}

		structured, ok := res.StructuredContent.(map[string]any)
		if !ok {
			results <- pingCallResult{err: errUnexpectedStructuredContent}

			return
		}

		message, ok := structured["message"].(string)
		if !ok {
			results <- pingCallResult{err: errUnexpectedStructuredContent}

			return
		}

		results <- pingCallResult{message: message}
	}

	wg.Add(2)
	go call(firstSession)
	go call(secondSession)
	wg.Wait()
	close(results)

	got := make([]pingCallResult, 0, 2)
	for result := range results {
		got = append(got, result)
	}

	return got
}

func TestRunHTTPValidation(t *testing.T) {
	Convey("B1.4: Given RunHTTP is called with empty HTTPOptions.Addr", t, func() {
		srv, err := New(Options{
			ServerVersion: "0.1.0",
			Logger:        discardTestLogger(),
			Providers:     []Provider{httpPingProvider{}},
		})
		So(err, ShouldBeNil)

		err = srv.RunHTTP(context.Background(), HTTPOptions{LogWriter: io.Discard})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "core: HTTP addr is required")
	})
}

func TestStreamableHTTPHandlerFactoryInjection(t *testing.T) {
	Convey("Given an injected streamable handler factory", t, func() {
		srv, err := New(Options{
			ServerVersion: "0.1.0",
			Logger:        discardTestLogger(),
			Providers:     []Provider{httpPingProvider{}},
		})
		So(err, ShouldBeNil)

		var recordedOptions *mcp.StreamableHTTPOptions
		var recordedGetServer func(*http.Request) *mcp.Server

		previousFactory := streamableHTTPHandlerFactory
		streamableHTTPHandlerFactory = func(
			getServer func(*http.Request) *mcp.Server,
			opts *mcp.StreamableHTTPOptions,
		) http.Handler {
			recordedGetServer = getServer
			recordedOptions = opts

			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})
		}

		Reset(func() {
			streamableHTTPHandlerFactory = previousFactory
		})

		authServer, _, err := srv.buildHTTPServer(context.Background(), HTTPOptions{LogWriter: io.Discard})
		So(err, ShouldBeNil)

		Convey("B1.5: the SDK handler options are stateless and use the core logger", func() {
			So(recordedOptions, ShouldNotBeNil)
			So(recordedOptions.Stateless, ShouldBeTrue)
			So(recordedOptions.Logger, ShouldEqual, srv.logger)
		})

		Convey("B1.6: the getServer callback returns the shared registered MCP server", func() {
			So(recordedGetServer, ShouldNotBeNil)

			gotServer := recordedGetServer(httptest.NewRequest(http.MethodPost, "/mcp", nil))
			So(gotServer, ShouldEqual, srv.mcpServer)

			session, cleanup := connectInMemoryMCPServer(t, gotServer)
			defer cleanup()

			res, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
			So(err, ShouldBeNil)
			So(res.Tools, ShouldHaveLength, 1)
			So(res.Tools[0].Name, ShouldEqual, "test_ping")
		})

		Reset(func() {
			authServer.Stop()
		})
	})
}

func connectInMemoryMCPServer(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("client Connect() returned error: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()

		select {
		case <-runErr:
		case <-time.After(5 * time.Second):
			t.Errorf("mcp.Server.Run did not return after context cancellation")
		}
	}

	return session, cleanup
}

func TestGoAuthserverFoundationPlainRoutes(t *testing.T) {
	Convey("Given a fake auth server adapter and a sentinel SDK handler", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		fakeAuthServer := newFakeHTTPAuthServer()
		restoreAuthServerFactory := replaceAuthServerFactory(fakeAuthServer)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())
		restoreStartHTTPServer := replaceStartHTTPServer(func(
			_ context.Context,
			_ string,
			handler http.Handler,
			_ time.Duration,
		) error {
			So(handler, ShouldEqual, fakeAuthServer.router)

			return nil
		})

		Reset(func() {
			restoreStartHTTPServer()
			restoreHandlerFactory()
			restoreAuthServerFactory()
		})

		err := srv.RunHTTP(context.Background(), HTTPOptions{
			Addr:      "127.0.0.1:0",
			LogWriter: io.Discard,
		})
		So(err, ShouldBeNil)

		Convey("B2.1: /mcp and /health are registered on Router without auth grouping", func() {
			So(fakeAuthServer.router.methodsForPath("/mcp"), ShouldResemble, []string{
				http.MethodDelete,
				http.MethodGet,
				http.MethodPost,
			})
			So(fakeAuthServer.router.methodsForPath("/health"), ShouldResemble, []string{
				http.MethodGet,
			})
			So(fakeAuthServer.routerCalls, ShouldBeGreaterThanOrEqualTo, 1)
			So(fakeAuthServer.authRouterCalls, ShouldEqual, 0)
			So(fakeAuthServer.authRoutes.routeCallCount(), ShouldEqual, 0)
		})

		Convey("B2.2: auth, TLS, token, JWT, and auth-group paths stay unused", func() {
			So(fakeAuthServer.enableAuthWithServerTokenCalls, ShouldEqual, 0)
			So(fakeAuthServer.startCalls, ShouldEqual, 0)
			So(fakeAuthServer.authRouterCalls, ShouldEqual, 0)
			So(fakeAuthServer.authRoutes.routeCallCount(), ShouldEqual, 0)
			So(fakeAuthServer.jwtMiddlewareCalls, ShouldEqual, 0)
		})
	})
}

func newHTTPTestCore(t *testing.T, providers []Provider) *Server {
	t.Helper()

	srv, err := New(Options{
		ServerVersion: "0.1.0",
		Logger:        discardTestLogger(),
		Providers:     providers,
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	return srv
}

func newFakeHTTPAuthServer() *fakeHTTPAuthServer {
	return &fakeHTTPAuthServer{
		router:     newRecordingHTTPRouter(),
		authRoutes: &recordingHTTPRouteGroup{},
	}
}

func newRecordingHTTPRouter() *recordingHTTPRouter {
	return &recordingHTTPRouter{
		engine: gin.New(),
	}
}

func replaceAuthServerFactory(fakeAuthServer *fakeHTTPAuthServer) func() {
	previousFactory := authServerFactory
	authServerFactory = func(io.Writer) httpAuthServer {
		return fakeAuthServer
	}

	return func() {
		authServerFactory = previousFactory
	}
}

func replaceStreamableHTTPHandlerFactory(handler http.Handler) func() {
	previousFactory := streamableHTTPHandlerFactory
	streamableHTTPHandlerFactory = func(
		func(*http.Request) *mcp.Server,
		*mcp.StreamableHTTPOptions,
	) http.Handler {
		return handler
	}

	return func() {
		streamableHTTPHandlerFactory = previousFactory
	}
}

func sentinelHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Sentinel-MCP", "wrapped")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("sentinel mcp handler"))
	})
}

func replaceStartHTTPServer(
	replacement func(context.Context, string, http.Handler, time.Duration) error,
) func() {
	previousStart := startHTTPServer
	startHTTPServer = replacement

	return func() {
		startHTTPServer = previousStart
	}
}

func TestMCPRouteUsesWrappedSDKHTTPHandler(t *testing.T) {
	Convey("B2.3: Given a sentinel MCP handler is mounted", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		fakeAuthServer := newFakeHTTPAuthServer()
		restoreAuthServerFactory := replaceAuthServerFactory(fakeAuthServer)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())

		Reset(func() {
			restoreHandlerFactory()
			restoreAuthServerFactory()
		})

		_, _, err := srv.buildHTTPServer(context.Background(), HTTPOptions{LogWriter: io.Discard})
		So(err, ShouldBeNil)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
		fakeAuthServer.router.ServeHTTP(recorder, request)

		So(recorder.Code, ShouldEqual, http.StatusAccepted)
		So(recorder.Header().Get("X-Sentinel-MCP"), ShouldEqual, "wrapped")
		So(recorder.Body.String(), ShouldEqual, "sentinel mcp handler")
	})
}

func TestPlainHTTPRouteNotFound(t *testing.T) {
	Convey("B2.5: Given GET /not-found", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		fakeAuthServer := newFakeHTTPAuthServer()
		restoreAuthServerFactory := replaceAuthServerFactory(fakeAuthServer)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())

		Reset(func() {
			restoreHandlerFactory()
			restoreAuthServerFactory()
		})

		_, _, err := srv.buildHTTPServer(context.Background(), HTTPOptions{LogWriter: io.Discard})
		So(err, ShouldBeNil)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/not-found", nil)
		fakeAuthServer.router.ServeHTTP(recorder, request)

		So(recorder.Code, ShouldEqual, http.StatusNotFound)
	})
}

func TestRunHTTPGracefulShutdownNoInflightRequests(t *testing.T) {
	Convey("B3.1: Given RunHTTP is serving on a local listener with no requests in flight", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())
		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)
		addr := reserveLocalHTTPAddr(t)

		Reset(func() {
			cancel()
			restoreHandlerFactory()
		})

		go func() {
			runErr <- srv.RunHTTP(ctx, HTTPOptions{
				Addr:      addr,
				LogWriter: io.Discard,
			})
		}()

		waitForHTTPStatus(t, "http://"+addr+"/health", http.StatusOK)
		cancel()

		err, settled := waitForHTTPResult(runErr, 3*time.Second)

		So(settled, ShouldBeTrue)
		So(err, ShouldBeNil)
	})
}

func reserveLocalHTTPAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving local HTTP addr: %v", err)
	}

	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("closing reserved listener: %v", err)
	}

	return addr
}

func waitForHTTPStatus(t *testing.T, url string, status int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == status {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s to return status %d", url, status)
}

func waitForHTTPResult(results <-chan error, timeout time.Duration) (error, bool) {
	select {
	case err := <-results:
		return err, true
	case <-time.After(timeout):
		return nil, false
	}
}

func TestRunHTTPGracefulShutdownDrainsInflightRequest(t *testing.T) {
	Convey("B3.2: Given a request is in flight when RunHTTP context is cancelled", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		started := make(chan struct{})
		release := make(chan struct{})
		handler := blockingStatusHandler(started, release, http.StatusOK)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(handler)
		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)
		requestResult := make(chan httpStatusResult, 1)
		addr := reserveLocalHTTPAddr(t)

		Reset(func() {
			cancel()
			closeIfOpen(release)
			restoreHandlerFactory()
		})

		go func() {
			runErr <- srv.RunHTTP(ctx, HTTPOptions{
				Addr:            addr,
				ShutdownTimeout: time.Second,
				LogWriter:       io.Discard,
			})
		}()

		waitForHTTPStatus(t, "http://"+addr+"/health", http.StatusOK)

		go func() {
			requestResult <- doHTTPStatus(http.MethodGet, "http://"+addr+"/mcp")
		}()

		waitForSignal(t, started, 3*time.Second, "MCP handler did not start")
		cancel()
		closeIfOpen(release)

		gotResponse, responseSettled := waitForHTTPStatusResult(requestResult, 3*time.Second)
		err, runSettled := waitForHTTPResult(runErr, 3*time.Second)

		So(responseSettled, ShouldBeTrue)
		So(gotResponse.err, ShouldBeNil)
		So(gotResponse.status, ShouldEqual, http.StatusOK)
		So(runSettled, ShouldBeTrue)
		So(err, ShouldBeNil)
	})
}

func blockingStatusHandler(started chan<- struct{}, release <-chan struct{}, status int) http.Handler {
	var startedOnce sync.Once

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		startedOnce.Do(func() {
			close(started)
		})
		<-release
		w.WriteHeader(status)
	})
}

func closeIfOpen(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func doHTTPStatus(method, url string) httpStatusResult {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return httpStatusResult{err: err}
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return httpStatusResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	return httpStatusResult{status: resp.StatusCode}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func waitForHTTPStatusResult(
	results <-chan httpStatusResult,
	timeout time.Duration,
) (httpStatusResult, bool) {
	select {
	case result := <-results:
		return result, true
	case <-time.After(timeout):
		return httpStatusResult{}, false
	}
}

func TestRunHTTPShutdownTimeoutReturnsDeadlineExceeded(t *testing.T) {
	Convey("B3.3: Given ShutdownTimeout expires while an MCP handler remains blocked", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		started := make(chan struct{})
		release := make(chan struct{})
		handler := blockingStatusHandler(started, release, http.StatusOK)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(handler)
		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)
		requestResult := make(chan httpStatusResult, 1)
		addr := reserveLocalHTTPAddr(t)

		Reset(func() {
			cancel()
			closeIfOpen(release)
			restoreHandlerFactory()
		})

		go func() {
			runErr <- srv.RunHTTP(ctx, HTTPOptions{
				Addr:            addr,
				ShutdownTimeout: 50 * time.Millisecond,
				LogWriter:       io.Discard,
			})
		}()

		waitForHTTPStatus(t, "http://"+addr+"/health", http.StatusOK)

		go func() {
			requestResult <- doHTTPStatus(http.MethodPost, "http://"+addr+"/mcp")
		}()

		waitForSignal(t, started, 3*time.Second, "MCP handler did not start")
		cancel()

		clientErr := waitForHTTPClientError(t, "http://"+addr+"/health", 500*time.Millisecond)
		err, settled := waitForHTTPResult(runErr, 500*time.Millisecond)

		So(clientErr, ShouldNotBeNil)
		So(settled, ShouldBeTrue)
		So(errors.Is(err, context.DeadlineExceeded), ShouldBeTrue)

		select {
		case <-requestResult:
			t.Fatalf("blocked MCP request completed before the test released it")
		default:
		}
	})
}

func waitForHTTPClientError(t *testing.T, url string, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:   100 * time.Millisecond,
		Transport: transport,
	}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			return err
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s to fail", url)

	return nil
}

func TestRunHTTPStopsAuthServerOnExit(t *testing.T) {
	Convey("B3.4: Given the fake auth server records calls", t, func() {
		srv := newHTTPTestCore(t, []Provider{httpPingProvider{}})
		fakeAuthServer := newFakeHTTPAuthServer()
		var recordedShutdownTimeout time.Duration
		restoreAuthServerFactory := replaceAuthServerFactory(fakeAuthServer)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())
		restoreStartHTTPServer := replaceStartHTTPServer(func(
			_ context.Context,
			_ string,
			_ http.Handler,
			shutdownTimeout time.Duration,
		) error {
			recordedShutdownTimeout = shutdownTimeout

			return nil
		})

		Reset(func() {
			restoreStartHTTPServer()
			restoreHandlerFactory()
			restoreAuthServerFactory()
		})

		err := srv.RunHTTP(context.Background(), HTTPOptions{
			Addr:      "127.0.0.1:0",
			LogWriter: io.Discard,
		})

		So(err, ShouldBeNil)
		So(fakeAuthServer.stopCalls, ShouldEqual, 1)
		So(recordedShutdownTimeout, ShouldEqual, 5*time.Second)
	})
}

type pingCallResult struct {
	message string
	err     error
}

type countingHTTPProvider struct {
	toolCalls *int
}

func (p countingHTTPProvider) Name() string { return "testsvc" }

func (p countingHTTPProvider) APIVersion() string { return "TESTAPI 9.9.9" }

func (p countingHTTPProvider) Register(_ context.Context, r Registrar) error {
	type pingInput struct{}

	type pingOutput struct {
		Message string `json:"message" jsonschema:"a fixed ping acknowledgement"`
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        "test_ping",
		Description: "returns a fixed pong message",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ pingInput) (*mcp.CallToolResult, pingOutput, error) {
		*p.toolCalls++

		return nil, pingOutput{Message: "pong"}, nil
	})

	return nil
}

func TestHealthRouteBeforeMCPUse(t *testing.T) {
	Convey("B2.4: Given GET /health before any MCP request", t, func() {
		toolCalls := 0
		srv := newHTTPTestCore(t, []Provider{countingHTTPProvider{toolCalls: &toolCalls}})
		fakeAuthServer := newFakeHTTPAuthServer()
		restoreAuthServerFactory := replaceAuthServerFactory(fakeAuthServer)
		restoreHandlerFactory := replaceStreamableHTTPHandlerFactory(sentinelHTTPHandler())

		Reset(func() {
			restoreHandlerFactory()
			restoreAuthServerFactory()
		})

		_, _, err := srv.buildHTTPServer(context.Background(), HTTPOptions{LogWriter: io.Discard})
		So(err, ShouldBeNil)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		fakeAuthServer.router.ServeHTTP(recorder, request)

		So(recorder.Code, ShouldEqual, http.StatusOK)
		So(recorder.Body.String(), ShouldEqual, `{"status":"ok"}`)
		So(toolCalls, ShouldEqual, 0)
	})
}

type httpStatusResult struct {
	status int
	err    error
}

type recordedHTTPRoute struct {
	method string
	path   string
}

type recordingHTTPRouter struct {
	engine *gin.Engine
	routes []recordedHTTPRoute
}

func (r *recordingHTTPRouter) GET(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	r.record(http.MethodGet, path)

	return r.engine.GET(path, handlers...)
}

func (r *recordingHTTPRouter) POST(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	r.record(http.MethodPost, path)

	return r.engine.POST(path, handlers...)
}

func (r *recordingHTTPRouter) DELETE(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	r.record(http.MethodDelete, path)

	return r.engine.DELETE(path, handlers...)
}

func (r *recordingHTTPRouter) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.engine.ServeHTTP(w, request)
}

func (r *recordingHTTPRouter) methodsForPath(path string) []string {
	methods := make([]string, 0)
	for _, route := range r.routes {
		if route.path == path {
			methods = append(methods, route.method)
		}
	}

	slices.Sort(methods)

	return methods
}

func (r *recordingHTTPRouter) record(method, path string) {
	r.routes = append(r.routes, recordedHTTPRoute{method: method, path: path})
}

type recordingHTTPRouteGroup struct {
	routes []recordedHTTPRoute
}

func (g *recordingHTTPRouteGroup) GET(path string, _ ...gin.HandlerFunc) gin.IRoutes {
	g.record(http.MethodGet, path)

	return nil
}

func (g *recordingHTTPRouteGroup) POST(path string, _ ...gin.HandlerFunc) gin.IRoutes {
	g.record(http.MethodPost, path)

	return nil
}

func (g *recordingHTTPRouteGroup) DELETE(path string, _ ...gin.HandlerFunc) gin.IRoutes {
	g.record(http.MethodDelete, path)

	return nil
}

func (g *recordingHTTPRouteGroup) routeCallCount() int {
	return len(g.routes)
}

func (g *recordingHTTPRouteGroup) record(method, path string) {
	g.routes = append(g.routes, recordedHTTPRoute{method: method, path: path})
}

type fakeHTTPAuthServer struct {
	router                         *recordingHTTPRouter
	authRoutes                     *recordingHTTPRouteGroup
	routerCalls                    int
	authRouterCalls                int
	enableAuthWithServerTokenCalls int
	startCalls                     int
	stopCalls                      int
	jwtMiddlewareCalls             int
}

func (s *fakeHTTPAuthServer) Router() httpRouter {
	s.routerCalls++

	return s.router
}

func (s *fakeHTTPAuthServer) AuthRouter() httpRouteGroup {
	s.authRouterCalls++

	return s.authRoutes
}

func (s *fakeHTTPAuthServer) EnableAuthWithServerToken(
	_, _, _ string,
	_ gas.AuthCallback,
) error {
	s.enableAuthWithServerTokenCalls++
	s.jwtMiddlewareCalls++

	return nil
}

func (s *fakeHTTPAuthServer) Start(_, _, _ string) error {
	s.startCalls++

	return nil
}

func (s *fakeHTTPAuthServer) Stop() {
	s.stopCalls++
}

func TestCoreHTTPImportBoundaries(t *testing.T) {
	Convey("B1.7: production core files stay service-agnostic", t, func() {
		imports := productionCoreImports(t)

		for _, importPath := range imports {
			So(strings.HasPrefix(importPath, "github.com/wtsi-hgi/wa/"), ShouldBeFalse)
			So(importPath, ShouldNotEqual, "github.com/wtsi-hgi/llm-knowledge-base/internal/mlwh")
		}
	})
}

func productionCoreImports(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing core files: %v", err)
	}

	imports := make([]string, 0)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s imports: %v", file, err)
		}

		for _, spec := range parsed.Imports {
			imports = append(imports, strings.Trim(spec.Path.Value, `"`))
		}
	}

	return imports
}

func TestCoreHTTPImportsGoAuthserverAsGas(t *testing.T) {
	Convey("B2.6: production core imports go-authserver as gas", t, func() {
		aliases := productionCoreImportAliases(t)
		alias, ok := aliases["github.com/wtsi-hgi/go-authserver"]

		So(ok, ShouldBeTrue)
		So(alias, ShouldEqual, "gas")
	})
}

func productionCoreImportAliases(t *testing.T) map[string]string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing core files: %v", err)
	}

	aliases := make(map[string]string)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s imports: %v", file, err)
		}

		for _, spec := range parsed.Imports {
			alias := ""
			if spec.Name != nil {
				alias = spec.Name.Name
			}

			aliases[strings.Trim(spec.Path.Value, `"`)] = alias
		}
	}

	return aliases
}
