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

// Package core is the service-agnostic core of the MCP server. It builds and
// runs an MCP server, registers self-contained providers through a narrow seam,
// and surfaces version information; it knows nothing of any provider's domain,
// client, transport, or auth, and must not import any provider's package (in
// particular it must not import github.com/wtsi-hgi/wa/...).
package core

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registrar is the subset of *mcp.Server a provider may use to register its MCP
// surface. It hides everything else about the core from providers.
type Registrar interface {
	// Server returns the underlying MCP server, for mcp.AddTool[In,Out](r.Server(),
	// ...) and any other registration the typed AddTool helper requires.
	Server() *mcp.Server

	// AddResource registers an MCP resource and its read handler.
	AddResource(r *mcp.Resource, h mcp.ResourceHandler)
}

// Provider is a self-contained backing service. It owns its config, client
// construction, and MCP tool/resource set. The core knows nothing of any
// provider's domain, client, transport, or auth.
//
// The seam is Provider + Registrar only: a provider need not expose a
// Go-importable package to the core.
type Provider interface {
	// Name is a short, stable provider identifier (e.g. "mlwh"). It keys this
	// provider's entry in VersionInfo.APIVersions.
	Name() string

	// APIVersion is the provider's compile-time targeted upstream API version
	// (e.g. the MLWH provider returns mlwh.APIVersion, currently "1.6.0").
	// Reading it must not contact any server. The core asks every provider for
	// this so it can assemble VersionInfo without knowing any provider's domain;
	// this keeps the seam service-agnostic (no wa/MLWH types leak into the core)
	// while still letting the core surface each upstream version in the server
	// Instructions (G3), the version resource (G2), and the startup log (G5).
	APIVersion() string

	// Register adds this provider's tools and resources via the Registrar. ctx
	// bounds any setup; it returns an error if the provider cannot start.
	Register(ctx context.Context, r Registrar) error
}

// VersionInfo is the server's own version and the per-provider targeted upstream
// API versions, surfaced to clients (e.g. via the version resource, the server
// Instructions, and the startup log).
type VersionInfo struct {
	// ServerVersion is this server's build version.
	ServerVersion string `json:"server_version"`

	// APIVersions maps each provider's Name to its targeted upstream API version.
	APIVersions map[string]string `json:"api_versions"`
}
