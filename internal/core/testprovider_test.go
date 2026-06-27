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
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testProvider is a minimal, service-agnostic Provider double used to drive the
// core's behaviour without depending on any real provider (the MLWH provider
// does not exist yet, and the core must stay provider-agnostic). It deliberately
// resembles neither MLWH nor wa: it registers a single trivial tool and a single
// trivial resource, and reports an arbitrary upstream API version.
type testProvider struct {
	name         string
	apiVersion   string
	toolName     string
	resourceURI  string
	resourceText string
}

func (p *testProvider) Name() string { return p.name }

func (p *testProvider) APIVersion() string { return p.apiVersion }

func (p *testProvider) Register(_ context.Context, r Registrar) error {
	type pingInput struct {
		Message string `json:"message,omitempty" jsonschema:"optional message to echo"`
	}

	type pingOutput struct {
		Pong string `json:"pong"`
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        p.toolName,
		Description: "a trivial test tool",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in pingInput) (*mcp.CallToolResult, pingOutput, error) {
		return nil, pingOutput{Pong: "pong:" + in.Message}, nil
	})

	if p.resourceURI != "" {
		r.AddResource(&mcp.Resource{
			URI:      p.resourceURI,
			Name:     p.name + "-resource",
			MIMEType: "text/plain",
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "text/plain",
					Text:     p.resourceText,
				}},
			}, nil
		})
	}

	return nil
}

// newTestProvider returns a testProvider with sensible defaults that callers may
// override per field.
func newTestProvider() *testProvider {
	return &testProvider{
		name:         "testsvc",
		apiVersion:   "TESTAPI 9.9.9",
		toolName:     "test_ping",
		resourceURI:  "testsvc://thing",
		resourceText: "trivial resource body",
	}
}

// runServerWithClient builds a core server from opts, runs it over one half of an
// in-memory transport pair (proving Run accepts any mcp.Transport), connects a
// test MCP client over the other half, and returns the connected client session.
// The returned cleanup cancels Run and waits for it to stop; it must be deferred.
// Any failure fails the test via t.Fatalf.
func runServerWithClient(t *testing.T, opts Options) (*mcp.ClientSession, func()) {
	t.Helper()

	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	runErr := make(chan error, 1)

	go func() {
		runErr <- srv.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()

		t.Fatalf("client Connect() returned error: %v", err)
	}

	cleanup := func() {
		_ = clientSession.Close()

		cancel()

		select {
		case <-runErr:
		case <-time.After(5 * time.Second):
			t.Errorf("Run did not return after context cancellation")
		}
	}

	return clientSession, cleanup
}
