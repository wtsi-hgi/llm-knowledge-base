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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run registers every configured provider then serves over the transport until
// ctx is cancelled or the peer disconnects.
//
// The transport is the seam: Run accepts ANY mcp.Transport (the binary passes
// &mcp.StdioTransport{}; tests pass an in-memory transport). This keeps the core
// transport-agnostic so a streamable-HTTP transport can be supplied later with no
// core change. No HTTP transport, configuration, flag, or listener exists in the
// core; supplying one is the caller's concern via this single mcp.Transport
// argument.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error {
	if err := s.registerProviders(ctx); err != nil {
		return err
	}

	// Emit the startup version line (Story G5) once, as serving begins, so the
	// running server's versions are visible in the logs.
	s.logStartupVersion()

	return s.mcpServer.Run(ctx, t)
}

func (s *Server) registerProviders(ctx context.Context) error {
	r := &registrar{server: s.mcpServer}

	for _, p := range s.providers {
		if err := p.Register(ctx, r); err != nil {
			return fmt.Errorf("registering provider %q: %w", p.Name(), err)
		}
	}

	return nil
}
