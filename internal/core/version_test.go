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

// This is an EXTERNAL test package (core_test) on purpose: Stories G2 and G5
// exercise the core with the REAL MLWH provider, and internal/mlwh imports
// internal/core. An in-package (package core) test could not import
// internal/mlwh without creating an import cycle, so these tests live in
// core_test, which may import internal/core, internal/mlwh, and wa together. The
// MLWH provider is built hermetically (mlwh.New with a dummy base URL): reading
// the version resource and emitting the startup log contact no server.
package core_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
	"github.com/wtsi-hgi/llm-knowledge-base/internal/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// TestVersionResource exercises Story G2: the core registers a version resource
// at mcp-server://version whose JSON body carries this server's version and each
// provider's targeted upstream API version. It reads the resource end-to-end
// over a real in-memory MCP client with the real MLWH provider configured.
func TestVersionResource(t *testing.T) {
	Convey("Given a core server built with ServerVersion 0.1.0 and the MLWH provider", t, func() {
		provider := newMLWHProvider(t)

		clientSession, cleanup := runCoreWithClient(t, core.Options{
			ServerVersion: "0.1.0",
			Providers:     []core.Provider{provider},
		})
		defer cleanup()

		res, err := clientSession.ReadResource(context.Background(), &mcp.ReadResourceParams{
			URI: "mcp-server://version",
		})
		So(err, ShouldBeNil)
		So(res.Contents, ShouldNotBeEmpty)

		contents := res.Contents[0]

		Convey("G2.1: its JSON parses to server_version 0.1.0 and api_versions.mlwh == wa.APIVersion", func() {
			var parsed struct {
				ServerVersion string            `json:"server_version"`
				APIVersions   map[string]string `json:"api_versions"`
			}

			err := json.Unmarshal([]byte(contents.Text), &parsed)
			So(err, ShouldBeNil)
			So(parsed.ServerVersion, ShouldEqual, "0.1.0")
			So(parsed.APIVersions["mlwh"], ShouldEqual, wa.APIVersion)
		})

		Convey("G2.2: the resource's MIMEType is application/json", func() {
			So(contents.MIMEType, ShouldEqual, "application/json")
		})
	})
}

// newMLWHProvider builds the real MLWH provider against a dummy base URL. No
// network is touched: the remote client is constructed lazily and these tests
// never invoke a tool, only read the version resource / observe the startup log.
func newMLWHProvider(t *testing.T) core.Provider {
	t.Helper()

	provider, err := mlwh.New(wa.RemoteConfig{BaseURL: "http://mlwh.invalid"})
	if err != nil {
		t.Fatalf("mlwh.New() returned error: %v", err)
	}

	return provider
}

// runCoreWithClient builds a core server from opts, runs it over one half of an
// in-memory transport pair (proving Run takes any mcp.Transport), connects a
// real MCP client over the other half, and returns the connected session. The
// returned cleanup cancels Run and waits for it to stop; it must be deferred.
func runCoreWithClient(t *testing.T, opts core.Options) (*mcp.ClientSession, func()) {
	t.Helper()

	srv, err := core.New(opts)
	if err != nil {
		t.Fatalf("core.New() returned error: %v", err)
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
