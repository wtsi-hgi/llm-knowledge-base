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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerVersion is this server's build version. It defaults to "dev" and is
// overridable at link time, e.g.:
//
//	go build -ldflags "-X github.com/wtsi-hgi/llm-knowledge-base/internal/core.ServerVersion=1.2.3"
//
// It is the default used by New when Options.ServerVersion is empty, and is the
// value reported by the --version flag (Story G4).
var ServerVersion = "dev"

// versionResourceMIMEType is the MIME type of the version resource body (Story
// G2): the body is VersionInfo marshaled to JSON.
const versionResourceMIMEType = "application/json"

// registerVersionResource registers the version MCP resource (Story G2) on the
// MCP server. The resource lives at versionResourceURI and its body is the
// supplied VersionInfo marshaled to JSON
// (e.g. {"server_version":"0.1.0","api_versions":{"mlwh":"1.6.0"}}), so a client
// can read this server's version and each provider's targeted upstream API
// version at runtime. The core registers it itself, not via a provider, so it is
// present even with no providers; the per-provider versions still arrive only
// through the Provider seam (in the assembled VersionInfo), keeping the core
// service-agnostic.
//
// The body is marshaled once at registration: VersionInfo is assembled at New
// time and never mutates, so a marshal failure here is a programming error and
// is returned so New can surface it.
func registerVersionResource(server *mcp.Server, info VersionInfo) error {
	body, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal version info: %w", err)
	}

	text := string(body)

	server.AddResource(&mcp.Resource{
		URI:         versionResourceURI,
		Name:        "version",
		Title:       "Server and upstream API versions",
		Description: "This server's build version and each provider's targeted upstream API version, as JSON.",
		MIMEType:    versionResourceMIMEType,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: versionResourceMIMEType,
				Text:     text,
			}},
		}, nil
	})

	return nil
}
