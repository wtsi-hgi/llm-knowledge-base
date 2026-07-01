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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

var errUnexpectedAllStudiesResult = errors.New("unexpected mlwh_call_endpoint AllStudies result")

type allStudiesCallResult struct {
	err          error
	nextOffset   float64
	resultLength int
	total        float64
}

func callAllStudiesWithTimeout(session *mcp.ClientSession) allStudiesCallResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "mlwh_call_endpoint",
		Arguments: map[string]any{
			"method":       "AllStudies",
			"query_params": map[string]any{"limit": "100", "offset": "0"},
		},
	})
	if err != nil {
		return allStudiesCallResult{err: err}
	}

	if res.IsError {
		return allStudiesCallResult{err: errUnexpectedAllStudiesResult}
	}

	obj, ok := res.StructuredContent.(map[string]any)
	if !ok {
		return allStudiesCallResult{err: errUnexpectedAllStudiesResult}
	}

	studies, ok := obj["result"].([]any)
	if !ok {
		return allStudiesCallResult{err: errUnexpectedAllStudiesResult}
	}

	return allStudiesCallResult{
		total:        numericJSON(obj["total"]),
		nextOffset:   numericJSON(obj["next_offset"]),
		resultLength: len(studies),
	}
}

func callAllStudiesConcurrently(sessions ...*mcp.ClientSession) []allStudiesCallResult {
	results := make(chan allStudiesCallResult, len(sessions))

	var wg sync.WaitGroup
	for _, session := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()

			results <- callAllStudiesWithTimeout(session)
		}()
	}

	wg.Wait()
	close(results)

	got := make([]allStudiesCallResult, 0, len(sessions))
	for result := range results {
		got = append(got, result)
	}

	return got
}

func TestMLWHHTTPSurfaceMatchesInMemoryHarness(t *testing.T) {
	Convey("Given the same stub warehouse backs in-memory and HTTP MLWH clients", t, func() {
		stub := newStubMLWH(t)
		inMemorySession, inMemoryCleanup := runMLWHServerWithClient(t, stub)
		defer inMemoryCleanup()

		httpSession, _, httpCleanup := runMLWHHTTPServerWithClient(t, stub)
		defer httpCleanup()

		Convey("C1.1: ListTools returns the same normalized metadata and no name diff", func() {
			inMemoryTools := sortedToolMetadata(t, inMemorySession)
			httpTools := sortedToolMetadata(t, httpSession)

			So(httpTools, ShouldResemble, inMemoryTools)

			missingFromHTTP, extraInHTTP := toolMetadataNameDiff(inMemoryTools, httpTools)
			So(missingFromHTTP, ShouldResemble, []string{})
			So(extraInHTTP, ShouldResemble, []string{})
		})

		Convey("C1.2: ListResources returns the same normalized metadata and no URI diff", func() {
			inMemoryResources := sortedResourceMetadata(t, inMemorySession)
			httpResources := sortedResourceMetadata(t, httpSession)

			So(httpResources, ShouldResemble, inMemoryResources)

			missingFromHTTP, extraInHTTP := resourceMetadataURIDiff(inMemoryResources, httpResources)
			So(missingFromHTTP, ShouldResemble, []string{})
			So(extraInHTTP, ShouldResemble, []string{})
		})

		Convey("C1.3: mlwh://workflow content matches and keeps workflow guidance", func() {
			inMemoryContent := readResource(t, inMemorySession, "mlwh://workflow")
			httpContent := readResource(t, httpSession, "mlwh://workflow")

			So(httpContent, ShouldResemble, inMemoryContent)
			So(httpContent.MIMEType, ShouldEqual, "text/markdown")
			So(httpContent.Text, ShouldContainSubstring, "# MLWH workflows")
			So(httpContent.Text, ShouldContainSubstring, "wa mlwh API endpoint reference")
			So(httpContent.Text, ShouldContainSubstring, "/resolve/sample")
			So(httpContent.Text, ShouldContainSubstring, "/study/:id/overview")
		})

		Convey("Given the stub returns one paged study for /studies", func() {
			stub.respondJSONWithHeaders("/studies", http.StatusOK, []wa.Study{
				{IDStudyTmp: 1, IDStudyLims: "S1", Name: "Alpha"},
			}, http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})

			args := map[string]any{
				"method":       "AllStudies",
				"query_params": map[string]any{"limit": "100", "offset": "0"},
			}
			inMemoryToolResult := callTool(t, inMemorySession, "mlwh_call_endpoint", args)
			httpToolResult := callTool(t, httpSession, "mlwh_call_endpoint", args)

			Convey("C1.4: mlwh_call_endpoint wraps paged AllStudies over HTTP", func() {
				inMemoryResult := structuredObject(inMemoryToolResult)
				httpResult := structuredObject(httpToolResult)

				So(numericJSON(httpResult["total"]), ShouldEqual, float64(1))
				So(numericJSON(httpResult["next_offset"]), ShouldEqual, float64(-1))
				httpStudies, ok := httpResult["result"].([]any)
				So(ok, ShouldBeTrue)
				So(len(httpStudies), ShouldEqual, 1)

				So(numericJSON(inMemoryResult["total"]), ShouldEqual, numericJSON(httpResult["total"]))
				So(numericJSON(inMemoryResult["next_offset"]), ShouldEqual, numericJSON(httpResult["next_offset"]))
				inMemoryStudies, ok := inMemoryResult["result"].([]any)
				So(ok, ShouldBeTrue)
				So(len(inMemoryStudies), ShouldEqual, len(httpStudies))
			})

			Convey("C1.5: the stub records the AllStudies path and query", func() {
				req, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(req.Path, ShouldEqual, "/studies")
				So(req.Query.Get("limit"), ShouldEqual, "100")
				So(req.Query.Get("offset"), ShouldEqual, "0")
			})
		})
	})
}

func TestMLWHHTTPVersionSurfaces(t *testing.T) {
	Convey("Given an HTTP MLWH client connected to a server versioned 0.1.0", t, func() {
		stub := newStubMLWH(t)
		session, _, cleanup := runMLWHHTTPServerWithClient(t, stub)
		defer cleanup()

		init := session.InitializeResult()

		Convey("C2.1: InitializeResult advertises the MLWH server name and version", func() {
			So(init.ServerInfo, ShouldNotBeNil)
			So(init.ServerInfo.Name, ShouldEqual, "mlwh-mcp-server")
			So(init.ServerInfo.Version, ShouldEqual, "0.1.0")
		})

		Convey("C2.2: Instructions expose the server, MLWH API, workflow, and version channels", func() {
			So(init.Instructions, ShouldContainSubstring, "0.1.0")
			So(init.Instructions, ShouldContainSubstring, wa.APIVersion)
			So(init.Instructions, ShouldContainSubstring, "mlwh://workflow")
			So(init.Instructions, ShouldContainSubstring, "mcp-server://version")
		})

		Convey("C2.3: mcp-server://version returns the server and targeted MLWH API versions", func() {
			content := readResource(t, session, "mcp-server://version")
			So(content.MIMEType, ShouldEqual, "application/json")

			var version core.VersionInfo
			err := json.Unmarshal([]byte(content.Text), &version)
			So(err, ShouldBeNil)
			So(version.ServerVersion, ShouldEqual, "0.1.0")
			So(version.APIVersions["mlwh"], ShouldEqual, wa.APIVersion)
		})
	})
}

func TestMLWHHTTPConcurrentClientsUseStubWarehouse(t *testing.T) {
	Convey("Given two HTTP clients connected to one MLWH HTTP server and stub warehouse", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSONWithHeaders("/studies", http.StatusOK, []wa.Study{
			{IDStudyTmp: 1, IDStudyLims: "S1", Name: "Alpha"},
		}, http.Header{
			"X-Total-Count": {"1"},
			"X-Next-Offset": {"-1"},
		})

		firstSession, endpoint, firstCleanup := runMLWHHTTPServerWithClient(t, stub)
		defer firstCleanup()

		secondSession, secondCleanup := connectMLWHHTTPClient(t, endpoint)
		defer secondCleanup()

		Convey("Implementation Order: concurrent tool calls both complete and reach the stub", func() {
			results := callAllStudiesConcurrently(firstSession, secondSession)

			So(results, ShouldHaveLength, 2)
			So(results[0].err, ShouldBeNil)
			So(results[1].err, ShouldBeNil)
			So(results[0].total, ShouldEqual, float64(1))
			So(results[1].total, ShouldEqual, float64(1))
			So(results[0].nextOffset, ShouldEqual, float64(-1))
			So(results[1].nextOffset, ShouldEqual, float64(-1))
			So(results[0].resultLength, ShouldEqual, 1)
			So(results[1].resultLength, ShouldEqual, 1)
			So(stub.requestCount(), ShouldEqual, 2)
		})
	})
}

func runMLWHHTTPServerWithClient(t *testing.T, stub *stubMLWH) (*mcp.ClientSession, string, func()) {
	t.Helper()

	endpoint, serverCleanup := runMLWHHTTPServer(t, stub)
	session, sessionCleanup := connectMLWHHTTPClient(t, endpoint)

	cleanup := func() {
		sessionCleanup()
		serverCleanup()
	}

	return session, endpoint, cleanup
}

func runMLWHHTTPServer(t *testing.T, stub *stubMLWH) (string, func()) {
	t.Helper()

	provider, err := New(wa.RemoteConfig{BaseURL: stub.server.URL})
	if err != nil {
		t.Fatalf("mlwh.New() returned error: %v", err)
	}

	srv, err := core.New(core.Options{
		ServerVersion: "0.1.0",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Providers:     []core.Provider{provider},
	})
	if err != nil {
		t.Fatalf("core.New() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	addr := reserveMLWHHTTPAddr(t)
	runErr := make(chan error, 1)

	go func() {
		runErr <- srv.RunHTTP(ctx, core.HTTPOptions{
			Addr:      addr,
			LogWriter: io.Discard,
		})
	}()

	waitForMLWHHTTPStatus(t, "http://"+addr+"/health", http.StatusOK, runErr)

	cleanup := func() {
		cancel()

		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("RunHTTP returned error after cancellation: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("RunHTTP did not return after context cancellation")
		}
	}

	return "http://" + addr + "/mcp", cleanup
}

func reserveMLWHHTTPAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving local HTTP addr: %v", err)
	}

	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("closing reserved listener: %v", err)
	}

	return addr
}

func waitForMLWHHTTPStatus(t *testing.T, url string, status int, runErr <-chan error) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}

	for time.Now().Before(deadline) {
		select {
		case err := <-runErr:
			t.Fatalf("RunHTTP returned before %s was ready: %v", url, err)
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == status {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s to return status %d", url, status)
}

func connectMLWHHTTPClient(t *testing.T, endpoint string) (*mcp.ClientSession, func()) {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	session, err := client.Connect(ctx, transport, nil)
	cancel()
	if err != nil {
		t.Fatalf("client Connect() returned error: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
	}

	return session, cleanup
}

func sortedToolMetadata(t *testing.T, cs *mcp.ClientSession) []*mcp.Tool {
	t.Helper()

	res, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() returned error: %v", err)
	}

	tools := slices.Clone(res.Tools)
	slices.SortFunc(tools, func(a, b *mcp.Tool) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return tools
}

func toolMetadataNameDiff(expected, actual []*mcp.Tool) ([]string, []string) {
	expectedNames := make([]string, 0, len(expected))
	actualNames := make([]string, 0, len(actual))

	for _, tool := range expected {
		expectedNames = append(expectedNames, tool.Name)
	}

	for _, tool := range actual {
		actualNames = append(actualNames, tool.Name)
	}

	return stringSetDiff(expectedNames, actualNames)
}

func sortedResourceMetadata(t *testing.T, cs *mcp.ClientSession) []*mcp.Resource {
	t.Helper()

	res, err := cs.ListResources(context.Background(), &mcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources() returned error: %v", err)
	}

	resources := slices.Clone(res.Resources)
	slices.SortFunc(resources, func(a, b *mcp.Resource) int {
		return cmp.Compare(a.URI, b.URI)
	})

	return resources
}

func resourceMetadataURIDiff(expected, actual []*mcp.Resource) ([]string, []string) {
	expectedURIs := make([]string, 0, len(expected))
	actualURIs := make([]string, 0, len(actual))

	for _, resource := range expected {
		expectedURIs = append(expectedURIs, resource.URI)
	}

	for _, resource := range actual {
		actualURIs = append(actualURIs, resource.URI)
	}

	return stringSetDiff(expectedURIs, actualURIs)
}

func stringSetDiff(expected, actual []string) ([]string, []string) {
	actualSet := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		actualSet[value] = struct{}{}
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, value := range expected {
		expectedSet[value] = struct{}{}
	}

	missing := []string{}
	for _, value := range expected {
		if _, ok := actualSet[value]; !ok {
			missing = append(missing, value)
		}
	}

	extra := []string{}
	for _, value := range actual {
		if _, ok := expectedSet[value]; !ok {
			extra = append(extra, value)
		}
	}

	slices.Sort(missing)
	slices.Sort(extra)

	return missing, extra
}
