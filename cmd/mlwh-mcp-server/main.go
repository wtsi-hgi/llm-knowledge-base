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

// Command mlwh-mcp-server is the entrypoint for the MLWH MCP server.
//
// It is the composition root: it parses flags, builds the MLWH provider from its
// configuration, wraps it in the service-agnostic core, and serves over the
// stdio transport by default so a local agent CLI (Claude Code, Codex) can
// launch it. A --version flag prints this server's build version and the
// targeted MLWH API version without opening any transport or requiring
// configuration.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
	"github.com/wtsi-hgi/llm-knowledge-base/internal/mlwh"
)

type signalNotifyContextFunc func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)

var signalNotifyContext signalNotifyContextFunc = signal.NotifyContext

type transportMode string

const (
	envHTTPAddr                      = "MLWH_HTTP_ADDR"
	transportModeHTTP  transportMode = "http"
	transportModeStdio transportMode = "stdio"
)

type coreServerFactory func(core.Options) (coreServer, error)

var newCoreServer coreServerFactory = func(opts core.Options) (coreServer, error) {
	return core.New(opts)
}

type stdioCoreServer interface {
	Run(context.Context, mcp.Transport) error
}

func serveStdio(srv stdioCoreServer) error {
	ctx, stop := signalNotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx, &mcp.StdioTransport{})
}

type httpCoreServer interface {
	RunHTTP(context.Context, core.HTTPOptions) error
}

func serveHTTP(srv httpCoreServer, opts core.HTTPOptions) error {
	ctx, stop := signalNotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return srv.RunHTTP(ctx, opts)
}

type coreServer interface {
	stdioCoreServer
	httpCoreServer
}

type commandConfig struct {
	MLWH      mlwh.Config
	HTTPAddr  string
	Transport transportMode
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mlwh-mcp-server:", err)
		os.Exit(1)
	}
}

// run parses args, then either prints version information (--version) or builds
// and serves the MCP server over the configured transport. stdout receives the
// --version output; operational logs go to the core's logger (stderr by
// default). It is factored out of main so a test can drive --version without a
// subprocess or real stdio.
//
// The --version path is handled before any configuration is resolved or any
// transport is opened, so `mlwh-mcp-server --version` works with no MLWH_BASE_URL set
// and returns promptly without serving or blocking on stdin.
func run(args []string, stdout io.Writer) error {
	cfg, showVersion, err := parseArgs(args)
	if err != nil {
		return err
	}

	if showVersion {
		return printVersion(stdout)
	}

	return serve(cfg)
}

func parseArgs(args []string) (commandConfig, bool, error) {
	fs := flag.NewFlagSet("mlwh-mcp-server", flag.ContinueOnError)

	showVersion := fs.Bool("version", false, "print the server version and the targeted MLWH API version, then exit")
	httpAddr := fs.String("http", "", "serve streamable HTTP on this address (env MLWH_HTTP_ADDR)")

	cfg := commandConfig{
		Transport: transportModeStdio,
	}
	cfg.MLWH.BindFlags(fs)

	if err := fs.Parse(args); err != nil {
		return commandConfig{}, false, err
	}

	cfg.HTTPAddr = os.Getenv(envHTTPAddr)
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "http" {
			cfg.HTTPAddr = *httpAddr
		}
	})

	if cfg.HTTPAddr != "" {
		cfg.Transport = transportModeHTTP
	}

	return cfg, *showVersion, nil
}

// printVersion writes this server's build version (core.ServerVersion) and the
// compile-time targeted MLWH API version (wa.APIVersion) to w. Reading either
// version contacts no server, so --version never needs configuration or the
// network.
func printVersion(w io.Writer) error {
	_, err := fmt.Fprintf(w, "mlwh-mcp-server version %s\nMLWH API version %s\n", core.ServerVersion, wa.APIVersion)

	return err
}

// serve resolves the MLWH provider configuration, builds the provider and the
// core server, and serves over the configured transport until the process is
// signalled or the peer disconnects. Operational output (including the startup
// version line) goes to the core's logger, not stdout, so it does not corrupt
// the stdio MCP stream.
func serve(cfg commandConfig) error {
	remoteCfg, err := cfg.MLWH.Resolve(nil)
	if err != nil {
		return err
	}

	maxToolResultBytes, err := cfg.MLWH.ResolveMaxToolResultBytes(nil)
	if err != nil {
		return err
	}

	provider, err := mlwh.New(remoteCfg)
	if err != nil {
		return err
	}

	srv, err := newCoreServer(coreOptions(provider, maxToolResultBytes))
	if err != nil {
		return err
	}

	if cfg.Transport == transportModeHTTP {
		return serveHTTP(srv, core.HTTPOptions{
			Addr:       cfg.HTTPAddr,
			MCPPath:    "/mcp",
			HealthPath: "/health",
			LogWriter:  os.Stderr,
		})
	}

	return serveStdio(srv)
}

func coreOptions(provider core.Provider, maxToolResultBytes int) core.Options {
	return core.Options{
		ServerVersion:          core.ServerVersion,
		Providers:              []core.Provider{provider},
		MaxToolResultBytes:     maxToolResultBytes,
		ToolResultSizeGuidance: mlwh.ToolResultSizeGuidance,
	}
}
