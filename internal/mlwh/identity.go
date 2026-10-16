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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"os/user"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"
)

// clientIDBytes is the byte length of a client ID; its hex form is twice as
// long, matching wa's CLI run ID format ^[0-9a-f]{32}$.
const clientIDBytes = 16

// processClientID is 16 crypto/rand bytes as lowercase hex, made once per
// process.
var processClientID = sync.OnceValue(func() string {
	b := make([]byte, clientIDBytes)
	_, _ = rand.Read(b) // never returns an error; crashes on failure.

	return hex.EncodeToString(b)
})

// identityRemoteConfig returns cfg with UserAgent set to
// wa.UserAgentProductMCP+"/"+serverVersion and Identity replaced by
// wa.ClientIdentity{MCPTransport: transport}, plus Username from
// localUsername(user.Current, os.Getenv) when transport is "stdio". Any
// UserAgent or Identity already in cfg is discarded. Other fields are kept.
func identityRemoteConfig(cfg wa.RemoteConfig, serverVersion, transport string) wa.RemoteConfig {
	cfg.UserAgent = wa.UserAgentProductMCP + "/" + serverVersion
	cfg.Identity = wa.ClientIdentity{MCPTransport: transport}

	if transport == "stdio" {
		cfg.Identity.Username = localUsername(user.Current, os.Getenv)
	}

	return cfg
}

// localUsername returns current().Username when current returns no error
// and a non-empty name, else getenv("USER"), else "".
func localUsername(current func() (*user.User, error), getenv func(string) string) string {
	if u, err := current(); err == nil && u.Username != "" {
		return u.Username
	}

	return getenv("USER")
}

// clientIdentityMiddleware returns receiving middleware that, for method
// "tools/call" with a *mcp.CallToolRequest, calls next with
// wa.WithClientIdentity(ctx, callIdentity(req)). Other methods and request
// types pass through unchanged.
func clientIdentityMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && method == "tools/call" {
				ctx = wa.WithClientIdentity(ctx, callIdentity(call))
			}

			return next(ctx, method, req)
		}
	}
}

// callIdentity builds the per-call identity for a tools/call request:
// ClientID from sessionClientID (Session.ID(), or "" when Session is nil),
// MCPAgent from agentIdentity (InitializeParams().ClientInfo when Session
// and params are non-nil; Extra.Header when Extra is non-nil), and MCPTool
// from Params.Name when Params is non-nil. It never panics on nil fields.
func callIdentity(req *mcp.CallToolRequest) wa.ClientIdentity {
	var (
		sessionID string
		info      *mcp.Implementation
		header    http.Header
		tool      string
	)

	if req.Session != nil {
		sessionID = req.Session.ID()

		if params := req.Session.InitializeParams(); params != nil {
			info = params.ClientInfo
		}
	}

	if req.Extra != nil {
		header = req.Extra.Header
	}

	if req.Params != nil {
		tool = req.Params.Name
	}

	return wa.ClientIdentity{
		ClientID: sessionClientID(sessionID),
		MCPAgent: agentIdentity(info, header),
		MCPTool:  tool,
	}
}

// sessionClientID returns the first 32 lowercase hex characters of the
// SHA-256 of sessionID, or processClientID() when sessionID is "".
func sessionClientID(sessionID string) string {
	if sessionID == "" {
		return processClientID()
	}

	sum := sha256.Sum256([]byte(sessionID))

	return hex.EncodeToString(sum[:clientIDBytes])
}

// agentIdentity returns "<name>/<version>" from info when info is non-nil
// and info.Name is non-empty ("<name>" alone when Version is ""), else
// header.Get("User-Agent") when header is non-nil, else "".
func agentIdentity(info *mcp.Implementation, header http.Header) string {
	if info != nil && info.Name != "" {
		if info.Version == "" {
			return info.Name
		}

		return info.Name + "/" + info.Version
	}

	return header.Get("User-Agent")
}
