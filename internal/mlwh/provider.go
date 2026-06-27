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

package mlwh

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// Environment variables recognised for MLWH provider configuration. Each has a
// matching flag (see Config.BindFlags); a flag value, when set, wins over its
// environment fallback.
const (
	envBaseURL = "MLWH_BASE_URL"
	envCACert  = "MLWH_CA_CERT"
	envTimeout = "MLWH_TIMEOUT"
)

// ErrBaseURLRequired is returned by New when no MLWH base URL was configured
// through any source. The MLWH API is reached only by its base URL, so the
// provider cannot start without one.
var ErrBaseURLRequired = errors.New("mlwh: base URL is required (set MLWH_BASE_URL or --mlwh-base-url)")

// provider must satisfy the core.Provider seam (Name, APIVersion, Register)
// exactly; this fails to compile if the interface and provider drift.
var _ core.Provider = (*provider)(nil)

// Config holds the MLWH provider's recognised settings as raw, unresolved
// strings, populated from command-line flags (BindFlags) and/or the
// environment (Resolve). Keeping the provider's own config self-contained lets
// each provider expose its own distinct settings without the core knowing them.
//
// Timeout is the raw string form (e.g. "5s") so a flag and its environment
// fallback share one Go-duration parse in Resolve.
type Config struct {
	// BaseURL is the MLWH REST base URL (flag --mlwh-base-url, env
	// MLWH_BASE_URL). Required.
	BaseURL string

	// CACert is an optional path to a PEM CA certificate trusted for the MLWH
	// TLS connection (flag --mlwh-ca-cert, env MLWH_CA_CERT).
	CACert string

	// Timeout is an optional per-request timeout as a Go duration string, e.g.
	// "5s" (flag --mlwh-timeout, env MLWH_TIMEOUT). Empty leaves the wa client
	// default in force.
	Timeout string
}

// BindFlags registers the --mlwh-base-url, --mlwh-ca-cert, and --mlwh-timeout
// flags on fs, writing parsed values back into c. cmd/mcp-server calls this to
// wire the flags, then Resolve to fold in the environment fallbacks. Flags
// default to empty so Resolve can tell "flag not given" from "flag given".
func (c *Config) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.BaseURL, "mlwh-base-url", "", "MLWH REST base URL (required; env "+envBaseURL+")")
	fs.StringVar(&c.CACert, "mlwh-ca-cert", "", "path to a PEM CA cert trusted for the MLWH TLS connection (env "+envCACert+")")
	fs.StringVar(&c.Timeout, "mlwh-timeout", "", "MLWH per-request timeout as a Go duration, e.g. 5s (env "+envTimeout+")")
}

// Resolve folds the environment into the (flag-sourced) Config and returns the
// wa.RemoteConfig the provider is built from. For each setting a non-empty
// Config field (typically from a flag) takes precedence; otherwise getenv
// supplies the value. getenv may be nil, in which case os.Getenv is used, so a
// test can inject a hermetic environment.
//
// Only BaseURL, CACert, and Timeout are populated: CacheTTL is inert for the
// remote client and is deliberately left zero, and Token is not surfaced
// because the MLWH API is unauthenticated. A non-empty but unparseable Timeout
// is an error. Resolve does not require BaseURL; that is enforced by New so the
// requirement is checked once, at construction.
func (c Config) Resolve(getenv func(string) string) (wa.RemoteConfig, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	cfg := wa.RemoteConfig{
		BaseURL: firstNonEmpty(c.BaseURL, getenv(envBaseURL)),
		CACert:  firstNonEmpty(c.CACert, getenv(envCACert)),
	}

	rawTimeout := firstNonEmpty(c.Timeout, getenv(envTimeout))
	if rawTimeout != "" {
		timeout, err := time.ParseDuration(rawTimeout)
		if err != nil {
			return wa.RemoteConfig{}, fmt.Errorf("mlwh: invalid timeout %q: %w", rawTimeout, err)
		}

		cfg.Timeout = timeout
	}

	return cfg, nil
}

// firstNonEmpty returns the first of its arguments that is not the empty
// string, or "" if all are empty. It expresses the flag-then-environment
// precedence used by Resolve.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

// provider is the MLWH core.Provider. It owns the remote client the tools
// (added in later phases) query through; the core sees only the Provider seam.
type provider struct {
	client *wa.RemoteClient
}

// Name returns the stable provider identifier "mlwh", which keys this provider's
// entry in the server's VersionInfo.APIVersions.
func (p *provider) Name() string {
	return "mlwh"
}

// APIVersion returns the compile-time MLWH API version this provider targets
// (wa.APIVersion). The core reads it to assemble VersionInfo without depending
// on any provider's domain; reading it contacts no server.
func (p *provider) APIVersion() string {
	return wa.APIVersion
}

// Register adds the provider's MCP tools and resources through the Registrar. It
// is modularised by tool group so each phase's batch wires its own tools without
// conflict: the sample/study search and count tools via registerSearchTools and
// the resolve/classify, unified find-samples, and expand tools via
// registerResolveTools. The detail/fan-out, freshness, and escape-hatch tool
// groups, and the workflow resource, are added by their own registrar helpers in
// later batches (still empty shells until then).
func (p *provider) Register(_ context.Context, r core.Registrar) error {
	if err := p.registerSearchTools(r); err != nil {
		return err
	}

	if err := p.registerResolveTools(r); err != nil {
		return err
	}

	return nil
}

// New builds the MLWH provider from a resolved RemoteConfig (see Config.Resolve)
// and returns it as a core.Provider. It is an error, ErrBaseURLRequired, for the
// base URL to be empty. The wa remote client is constructed here but contacts no
// server until a tool is invoked, so New performs no network I/O.
func New(cfg wa.RemoteConfig) (core.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, ErrBaseURLRequired
	}

	client, err := wa.NewRemoteClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("mlwh: build remote client: %w", err)
	}

	return &provider{client: client}, nil
}
