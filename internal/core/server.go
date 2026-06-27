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
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// implementationName is the MCP implementation name this server advertises to
// connecting clients (Story G3).
const implementationName = "mlwh-mcp-server"

// workflowResourceURI and versionResourceURI are the URIs the Instructions point
// clients at. The version resource is registered by the core (Story G2); the
// workflow resource is registered by the MLWH provider (Story G1). They are named
// here only so the Instructions can mention them.
const (
	workflowResourceURI = "mlwh://workflow"
	versionResourceURI  = "mcp-server://version"
)

// Options configures the core server.
type Options struct {
	// ServerVersion is this server's build version. If empty, the build-time
	// ServerVersion package variable is used.
	ServerVersion string

	// Logger receives operational output (e.g. the startup version line, Story
	// G5). If nil, a default logger is used so a configured logger always sees the
	// line.
	Logger *slog.Logger

	// Providers are the backing services to register when the server runs. The
	// shipped binary configures exactly one (MLWH); tests may configure others.
	Providers []Provider
}

// Server is a configured core MCP server: implementation info, instructions,
// logger, and the providers to register. Build it with New and start it with
// Run.
type Server struct {
	mcpServer *mcp.Server
	logger    *slog.Logger
	providers []Provider
	version   VersionInfo
}

// New builds a configured core server (implementation info, instructions,
// logger, version info) with no providers registered yet; providers are
// registered when Run is called. It never contacts any network.
func New(opts Options) (*Server, error) {
	serverVersion := opts.ServerVersion
	if serverVersion == "" {
		serverVersion = ServerVersion
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	version := buildVersionInfo(serverVersion, opts.Providers)

	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: implementationName, Version: serverVersion},
		&mcp.ServerOptions{
			Instructions: buildInstructions(serverVersion, opts.Providers),
			Logger:       logger,
		},
	)

	// The core registers the version resource itself (Story G2), not via a
	// provider, so it is present regardless of which providers are configured.
	if err := registerVersionResource(mcpServer, version); err != nil {
		return nil, fmt.Errorf("registering version resource: %w", err)
	}

	return &Server{
		mcpServer: mcpServer,
		logger:    logger,
		providers: opts.Providers,
		version:   version,
	}, nil
}

// VersionInfo returns the server's assembled version information: this server's
// version and each configured provider's targeted upstream API version, keyed by
// provider name. It is assembled at New time by asking each provider, and is the
// single source for the version resource (Story G2), the Instructions (Story
// G3), and the startup log line (Story G5).
func (s *Server) VersionInfo() VersionInfo {
	return s.version
}

// logStartupVersion emits the single startup version line (Story G5) when the
// server begins serving. It is sourced from the same VersionInfo the core
// assembled at New time, so it names this server's version and each provider's
// targeted upstream API version (e.g. mlwh=1.6.0) without the core knowing any
// provider's domain. If no logger was configured the default logger is used, so
// a configured logger always receives the line. The per-provider versions are
// attached as one structured attribute so the line carries each as
// "<name>=<version>".
func (s *Server) logStartupVersion() {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}

	apiVersions := make([]any, 0, len(s.version.APIVersions)*2)
	for _, name := range slices.Sorted(maps.Keys(s.version.APIVersions)) {
		apiVersions = append(apiVersions, name, s.version.APIVersions[name])
	}

	logger.Info("starting MLWH MCP server",
		slog.String("server_version", s.version.ServerVersion),
		slog.Group("api_versions", apiVersions...),
	)
}

// registrar adapts a *mcp.Server to the Registrar seam exposed to providers.
type registrar struct {
	server *mcp.Server
}

// Server returns the underlying MCP server.
func (r *registrar) Server() *mcp.Server { return r.server }

// AddResource registers an MCP resource and its read handler.
func (r *registrar) AddResource(res *mcp.Resource, h mcp.ResourceHandler) {
	r.server.AddResource(res, h)
}

// buildVersionInfo assembles the server's VersionInfo by asking each provider for
// its targeted upstream API version. The core stays service-agnostic: it learns
// each version solely through the Provider seam.
func buildVersionInfo(serverVersion string, providers []Provider) VersionInfo {
	apiVersions := make(map[string]string, len(providers))
	for _, p := range providers {
		apiVersions[p.Name()] = p.APIVersion()
	}

	return VersionInfo{
		ServerVersion: serverVersion,
		APIVersions:   apiVersions,
	}
}

// buildInstructions renders the server Instructions (Story G3): they state this
// server's version and each provider's targeted upstream API version, and point
// at the workflow and version resources. Providers are rendered in configured
// order so the Instructions are deterministic.
func buildInstructions(serverVersion string, providers []Provider) string {
	var b strings.Builder

	b.WriteString("MLWH MCP server version ")
	b.WriteString(serverVersion)
	b.WriteString(".")

	for _, p := range providers {
		b.WriteString(" ")
		b.WriteString(strings.ToUpper(p.Name()))
		b.WriteString(" API ")
		b.WriteString(p.APIVersion())
		b.WriteString(".")
	}

	b.WriteString(" See the ")
	b.WriteString(workflowResourceURI)
	b.WriteString(" resource for how the endpoints compose into workflows, and the ")
	b.WriteString(versionResourceURI)
	b.WriteString(" resource for these versions at runtime.")

	return b.String()
}
