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
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gas "github.com/wtsi-hgi/go-authserver"
)

const (
	defaultMCPPath         = "/mcp"
	defaultHealthPath      = "/health"
	defaultShutdownTimeout = 5 * time.Second
	readHeaderTimeout      = 30 * time.Second
)

var errHTTPAddrRequired = errors.New("core: HTTP addr is required")

var authServerFactory = func(logWriter io.Writer) httpAuthServer {
	return &gasAuthServerAdapter{server: gas.New(logWriter)}
}

var startHTTPServer = startPlainHTTPServer

type streamableHTTPHandlerFactoryFunc func(
	func(*http.Request) *mcp.Server,
	*mcp.StreamableHTTPOptions,
) http.Handler

var streamableHTTPHandlerFactory streamableHTTPHandlerFactoryFunc = func(
	getServer func(*http.Request) *mcp.Server,
	opts *mcp.StreamableHTTPOptions,
) http.Handler {
	return mcp.NewStreamableHTTPHandler(getServer, opts)
}

// HTTPOptions configures the core streamable HTTP server.
type HTTPOptions struct {
	// Addr is the TCP address to listen on. It is required by RunHTTP.
	Addr string

	// MCPPath is the streamable MCP endpoint path. Empty defaults to /mcp.
	MCPPath string

	// HealthPath is the health endpoint path. Empty defaults to /health.
	HealthPath string

	// ShutdownTimeout bounds graceful HTTP shutdown after ctx is cancelled.
	// Empty defaults to 5 seconds.
	ShutdownTimeout time.Duration

	// LogWriter receives go-authserver/gin HTTP logs. Nil defaults to
	// io.Discard.
	LogWriter io.Writer
}

func normalizeHTTPOptions(opts HTTPOptions) HTTPOptions {
	if opts.MCPPath == "" {
		opts.MCPPath = defaultMCPPath
	}

	if opts.HealthPath == "" {
		opts.HealthPath = defaultHealthPath
	}

	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = defaultShutdownTimeout
	}

	if opts.LogWriter == nil {
		opts.LogWriter = io.Discard
	}

	return opts
}

// RunHTTP registers providers and serves the shared MCP server over streamable
// HTTP until ctx is cancelled or the listener fails.
func (s *Server) RunHTTP(ctx context.Context, opts HTTPOptions) error {
	opts = normalizeHTTPOptions(opts)
	if opts.Addr == "" {
		return errHTTPAddrRequired
	}

	authServer, opts, err := s.buildHTTPServer(ctx, opts)
	if err != nil {
		return err
	}
	defer authServer.Stop()

	s.logStartupVersion(
		slog.String("transport", "http"),
		slog.String("addr", opts.Addr),
		slog.String("mcp_path", opts.MCPPath),
		slog.String("health_path", opts.HealthPath),
	)

	return startHTTPServer(ctx, opts.Addr, authServer.Router(), opts.ShutdownTimeout)
}

type httpAuthServer interface {
	Router() httpRouter
	AuthRouter() httpRouteGroup
	EnableAuthWithServerToken(string, string, string, gas.AuthCallback) error
	Start(string, string, string) error
	Stop()
}

func (s *Server) buildHTTPServer(ctx context.Context, opts HTTPOptions) (httpAuthServer, HTTPOptions, error) {
	opts = normalizeHTTPOptions(opts)

	if err := s.registerProviders(ctx); err != nil {
		return nil, opts, err
	}

	authServer := authServerFactory(opts.LogWriter)
	router := authServer.Router()

	router.GET(opts.HealthPath, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	handler := streamableHTTPHandlerFactory(
		func(*http.Request) *mcp.Server { return s.mcpServer },
		&mcp.StreamableHTTPOptions{
			Stateless: true,
			Logger:    s.logger,
		},
	)
	wrappedHandler := gin.WrapH(handler)

	router.GET(opts.MCPPath, wrappedHandler)
	router.POST(opts.MCPPath, wrappedHandler)
	router.DELETE(opts.MCPPath, wrappedHandler)

	return authServer, opts, nil
}

type httpRouteGroup interface {
	GET(string, ...gin.HandlerFunc) gin.IRoutes
	POST(string, ...gin.HandlerFunc) gin.IRoutes
	DELETE(string, ...gin.HandlerFunc) gin.IRoutes
}

type httpRouter interface {
	http.Handler
	httpRouteGroup
}

type gasAuthServerAdapter struct {
	server *gas.Server
}

func (a *gasAuthServerAdapter) Router() httpRouter {
	return a.server.Router()
}

func (a *gasAuthServerAdapter) AuthRouter() httpRouteGroup {
	return a.server.AuthRouter()
}

func (a *gasAuthServerAdapter) EnableAuthWithServerToken(
	certFile string,
	keyFile string,
	tokenBasename string,
	acb gas.AuthCallback,
) error {
	return a.server.EnableAuthWithServerToken(certFile, keyFile, tokenBasename, acb)
}

func (a *gasAuthServerAdapter) Start(addr, certFile, keyFile string) error {
	return a.server.Start(addr, certFile, keyFile)
}

func (a *gasAuthServerAdapter) Stop() {
	a.server.Stop()
}

func startPlainHTTPServer(
	ctx context.Context,
	addr string,
	handler http.Handler,
	shutdownTimeout time.Duration,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		return normalizeHTTPServeError(err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErr := httpServer.Shutdown(shutdownCtx)
		err := <-serveErr
		if shutdownErr != nil {
			return shutdownErr
		}

		return normalizeHTTPServeError(err)
	}
}

func normalizeHTTPServeError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}
