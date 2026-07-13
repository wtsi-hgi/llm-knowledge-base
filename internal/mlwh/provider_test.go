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
	"flag"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// fullSurfaceTools is the full set of typed MLWH tools the provider registers:
// the original A-E tools plus the phase 5-8 overview/status, availability,
// availability, people, and count surfaces that F2 guards. I1.1 requires at least
// the eight headline tools listed in the spec; this fuller set strengthens the
// assertion so a dropped tool in any group fails the test, not just one of the
// named headline tools.
var fullSurfaceTools = []string{
	// A: search and count (5).
	"mlwh_search_samples",
	"mlwh_count_samples",
	"mlwh_search_studies",
	"mlwh_count_studies_search",
	"mlwh_count_studies",

	// B/D4: resolve/classify (7), unified find-samples/count (2), expand (3).
	"mlwh_classify_identifier",
	"mlwh_resolve_sample",
	"mlwh_resolve_sample_name",
	"mlwh_resolve_study",
	"mlwh_resolve_run",
	"mlwh_resolve_library",
	"mlwh_resolve_library_identifier",
	"mlwh_find_samples",
	"mlwh_count_find_samples",
	"mlwh_expand_identifier",
	"mlwh_expand_search_values",
	"mlwh_expand_sample_search_values",

	// B: overview and status (5).
	"mlwh_study_overview",
	"mlwh_study_status_breakdown",
	"mlwh_run_overview",
	"mlwh_run_status",
	"mlwh_sample_progress",

	// C: detail (4) and fan-out enumeration (10).
	"mlwh_sample_detail",
	"mlwh_study_detail",
	"mlwh_run_detail",
	"mlwh_library_detail",
	"mlwh_all_studies",
	"mlwh_samples_for_study",
	"mlwh_samples_for_run",
	"mlwh_libraries_for_study",
	"mlwh_runs_for_study",
	"mlwh_lanes_for_sample",
	"mlwh_irods_paths_for_sample",
	"mlwh_irods_paths_for_study",
	"mlwh_studies_for_sample",
	"mlwh_count_samples_with_data_for_study",
	"mlwh_samples_with_data_for_study",
	"mlwh_samples_without_data_for_study",
	"mlwh_irods_paths_for_run",
	"mlwh_count_irods_paths_for_sample",
	"mlwh_count_irods_paths_for_study",
	"mlwh_count_irods_paths_for_run",
	"mlwh_count_samples_for_study",
	"mlwh_count_samples_for_run",
	"mlwh_count_runs_for_study",
	"mlwh_count_libraries_for_study",
	"mlwh_count_lanes_for_sample",
	"mlwh_count_samples_for_library",
	"mlwh_count_samples_for_library_id",
	"mlwh_count_samples_for_library_lims_id",
	"mlwh_count_samples_for_library_type",

	// D2: people and sponsor/user counts (6).
	"mlwh_studies_for_faculty_sponsor",
	"mlwh_count_studies_for_faculty_sponsor",
	"mlwh_studies_for_user",
	"mlwh_count_studies_for_user",
	"mlwh_resolve_person",
	"mlwh_count_resolve_person",

	// D: freshness (1).
	"mlwh_freshness",

	// E: generic escape hatch (1).
	"mlwh_call_endpoint",
}

// i1HeadlineTools are the eight tools spec story I1.1 explicitly names. They are
// a subset of aToETools; asserting them individually keeps the acceptance test
// pinned to the spec text while the fuller aToETools set guards the rest.
var i1HeadlineTools = []string{
	"mlwh_search_samples",
	"mlwh_count_samples",
	"mlwh_search_studies",
	"mlwh_resolve_sample",
	"mlwh_find_samples",
	"mlwh_sample_detail",
	"mlwh_freshness",
	"mlwh_call_endpoint",
}

// f2HeadlineTools are the Phase 9 F2 tools explicitly named by the acceptance
// test. The fullSurfaceTools set above guards every registered surface, while
// this list keeps the test pinned to F2's named examples.
var f2HeadlineTools = []string{
	"mlwh_study_overview",
	"mlwh_study_status_breakdown",
	"mlwh_run_overview",
	"mlwh_run_status",
	"mlwh_sample_progress",
	"mlwh_samples_with_data_for_study",
	"mlwh_samples_without_data_for_study",
	"mlwh_irods_paths_for_run",
	"mlwh_studies_for_user",
	"mlwh_resolve_person",
	"mlwh_count_find_samples",
}

// bareListTools are list-like tools whose successful responses do not carry
// cache_synced_at, so their descriptions must route agents to mlwh_freshness
// for the cache as-of caveat.
var bareListTools = []string{
	"mlwh_search_samples",
	"mlwh_search_studies",
	"mlwh_find_samples",
	"mlwh_expand_identifier",
	"mlwh_expand_search_values",
	"mlwh_expand_sample_search_values",
	"mlwh_all_studies",
	"mlwh_samples_for_study",
	"mlwh_samples_for_run",
	"mlwh_libraries_for_study",
	"mlwh_runs_for_study",
	"mlwh_lanes_for_sample",
	"mlwh_irods_paths_for_sample",
	"mlwh_irods_paths_for_study",
	"mlwh_studies_for_sample",
	"mlwh_samples_with_data_for_study",
	"mlwh_samples_without_data_for_study",
	"mlwh_irods_paths_for_run",
	"mlwh_studies_for_faculty_sponsor",
	"mlwh_studies_for_user",
	"mlwh_resolve_person",
}

func TestProviderNew(t *testing.T) {
	Convey("New builds an MLWH provider from a resolved RemoteConfig", t, func() {
		Convey("H2.2: a missing base URL is a clear startup error mentioning it is required", func() {
			provider, err := New(wa.RemoteConfig{})
			So(err, ShouldNotBeNil)
			So(provider, ShouldBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "base url")
			So(msg, ShouldContainSubstring, "required")
		})

		Convey("a valid base URL yields a non-nil core.Provider and no error", func() {
			provider, err := New(wa.RemoteConfig{BaseURL: "http://stub.example"})
			So(err, ShouldBeNil)
			So(provider, ShouldNotBeNil)

			Convey("I1.3: Name() returns \"mlwh\"", func() {
				So(provider.Name(), ShouldEqual, "mlwh")
			})

			Convey("APIVersion() returns the targeted wa.APIVersion", func() {
				So(provider.APIVersion(), ShouldEqual, wa.APIVersion)
			})

			Convey("Register wires the search/count tools through the Registrar", func() {
				stub := newStubMLWH(t)
				cs, cleanup := runMLWHServerWithClient(t, stub)
				defer cleanup()

				_, ok := toolByName(t, cs, "mlwh_search_samples")
				So(ok, ShouldBeTrue)
			})
		})
	})
}

func TestWAAPI18Contract(t *testing.T) {
	Convey("A1.1: Given the updated module, provider construction targets wa API 1.8.0", t, func() {
		So(wa.APIVersion, ShouldEqual, "1.8.0")

		provider, err := New(wa.RemoteConfig{BaseURL: "http://stub.example"})
		So(err, ShouldBeNil)
		So(provider.APIVersion(), ShouldEqual, wa.APIVersion)
	})

	Convey("A1.4: Given the v0.8.0 Registry, removed manifest APIs are not part of the compiled contract", t, func() {
		_, hasStudyManifest := registryEntryByMethod("StudyManifest")
		So(hasStudyManifest, ShouldBeFalse)

		_, hasCountStudyManifest := registryEntryByMethod("CountStudyManifest")
		So(hasCountStudyManifest, ShouldBeFalse)

		So(wa.EndpointReference(), ShouldNotContainSubstring, "/manifest")
	})

	Convey("Given wa.Registry, the A1 methods expose upstream endpoint documentation", t, func() {
		studyOverview, ok := registryEntryByMethod("StudyOverview")
		So(ok, ShouldBeTrue)
		So(studyOverview.Path, ShouldEqual, "/study/:id/overview")
		So(strings.TrimSpace(studyOverview.Summary), ShouldNotBeBlank)
		So(strings.TrimSpace(studyOverview.Description), ShouldNotBeBlank)

		resolvePerson, ok := registryEntryByMethod("ResolvePerson")
		So(ok, ShouldBeTrue)
		So(resolvePerson.Path, ShouldEqual, "/resolve-person/:term")
		So(strings.TrimSpace(resolvePerson.Summary), ShouldNotBeBlank)
		So(strings.TrimSpace(resolvePerson.Description), ShouldNotBeBlank)
	})

	Convey("Given wa.OpenAPIDocument, outputSchemaFor keeps upstream json names and field descriptions", t, func() {
		studyOverview, err := outputSchemaFor("StudyOverview")
		So(err, ShouldBeNil)

		studyProperties, ok := studyOverview["properties"].(map[string]any)
		So(ok, ShouldBeTrue)
		_, hasGoName := studyProperties["SamplesTotal"]
		So(hasGoName, ShouldBeFalse)

		samplesTotal, ok := studyProperties["samples_total"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(samplesTotal["description"], ShouldEqual, "distinct samples linked via library_samples")

		cacheSyncedAt, ok := studyProperties["cache_synced_at"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(cacheSyncedAt["description"], ShouldEqual, "oldest last_run across feeding tables (UTC RFC3339)")

		personCandidate, err := outputSchemaFor("PersonCandidate")
		So(err, ShouldBeNil)

		personProperties, ok := personCandidate["properties"].(map[string]any)
		So(ok, ShouldBeTrue)
		_, hasPersonGoName := personProperties["StudyCount"]
		So(hasPersonGoName, ShouldBeFalse)

		source, ok := personProperties["source"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(source["description"], ShouldEqual, "faculty_sponsor or study_users")

		studyCount, ok := personProperties["study_count"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(studyCount["description"], ShouldEqual, "distinct studies for this candidate")
	})
}

func TestF3ProviderGuardGuidanceAndHermeticHarness(t *testing.T) {
	Convey("F3.4: provider size guidance names every actual continuation mechanism", t, func() {
		guidance := strings.ToLower(ToolResultSizeGuidance)
		So(guidance, ShouldContainSubstring, "smaller")
		So(guidance, ShouldContainSubstring, "nextcursor")
		So(guidance, ShouldContainSubstring, "offset")
		So(guidance, ShouldContainSubstring, "next_offset")
		So(guidance, ShouldContainSubstring, "last row")
		So(guidance, ShouldContainSubstring, "cursor")
	})

	Convey("F3.6: the provider runs end-to-end against only its loopback httptest server", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/programmes", http.StatusOK, []wa.Programme{})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := callTool(t, cs, "mlwh_programmes", map[string]any{})
		So(result.IsError, ShouldBeFalse)
		So(stub.server.URL, ShouldStartWith, "http://127.0.0.1:")
		So(stub.requestCount(), ShouldEqual, 1)
	})
}

type f3PagedToolCase struct {
	name           string
	path           string
	arguments      map[string]any
	body           any
	defaultLimit   string
	supportsOffset bool
	headerPage     bool
}

func f3PagedToolCases() []f3PagedToolCase {
	return []f3PagedToolCase{
		{
			name: "mlwh_samples_with_data_for_study", path: "/study/S1/samples-with-data",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.SampleWithData{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_samples_without_data_for_study", path: "/study/S1/samples-without-data",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.SampleWithData{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_irods_paths_for_sample", path: "/sample/S1/irods",
			arguments: map[string]any{"sanger_name": "S1"}, body: []wa.IRODSPath{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_irods_paths_for_study", path: "/study/S1/irods",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.IRODSPath{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_irods_paths_for_run", path: "/run/52553/irods",
			arguments: map[string]any{"id_run": "52553"}, body: []wa.IRODSPath{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_latest_data_for_study", path: "/study/S1/latest-data",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.RecentDataRow{},
			defaultLimit: "10", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_latest_data_for_faculty_sponsor", path: "/latest-data/faculty-sponsor/Ada",
			arguments: map[string]any{"faculty_sponsor": "Ada"}, body: []wa.RecentDataRow{},
			defaultLimit: "10", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_sample_crams_for_study", path: "/study/S1/sample-crams",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.SampleCRAM{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_runs_for_sample", path: "/sample/S1/runs",
			arguments: map[string]any{"sanger_name": "S1"}, body: []wa.Run{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_runs", path: "/runs", arguments: map[string]any{}, body: []wa.RunListingRow{},
			defaultLimit: "100",
		},
		{
			name: "mlwh_studies_for_programme", path: "/studies/programme/Cancer",
			arguments: map[string]any{"programme": "Cancer"}, body: []wa.Study{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
		{
			name: "mlwh_study_users", path: "/study/S1/users",
			arguments: map[string]any{"study_lims_id": "S1"}, body: []wa.StudyUser{},
			defaultLimit: "100", supportsOffset: true, headerPage: true,
		},
	}
}

// TestProviderFullSurface exercises Story I1: with only the MLWH provider
// configured (pointed at a hermetic stub), a connected in-memory MCP client sees
// every tool from stories A-E and both the mlwh://workflow and
// mcp-server://version resources, and the provider identifies itself as "mlwh".
// Driving it through a real in-memory client also realises the end-to-end form
// of Story H1.1 (Run accepts any mcp.Transport; the client lists the MLWH tools
// over mcp.NewInMemoryTransports()).
func TestProviderFullSurface(t *testing.T) {
	Convey("Given a core server with only the MLWH provider, over an in-memory client", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A1.2/I1.1/F2.1: a tools listing excludes manifests and includes every other existing tool", func() {
			registered := listToolNames(t, cs)
			So(registered, ShouldNotContainKey, "mlwh_study_manifest")
			So(registered, ShouldNotContainKey, "mlwh_count_study_manifest")

			for _, name := range i1HeadlineTools {
				So(registered, ShouldContainKey, name)
			}

			for _, name := range f2HeadlineTools {
				So(registered, ShouldContainKey, name)
			}

			// Strengthen: the full typed surface registers, so a dropped tool in
			// any group is caught, not only one of the headline tools.
			var missing []string

			for _, name := range fullSurfaceTools {
				if _, ok := registered[name]; !ok {
					missing = append(missing, name)
				}
			}

			So(missing, ShouldBeEmpty)
		})

		Convey("A1.2: every surviving existing count tool remains Registry-backed", func() {
			tools := listToolsByName(t, cs)
			registered := toolNameSet(tools)
			expected := registryCountToolNames()

			failures := existingCountToolRegistryFailures(registered, expected)
			So(failures, ShouldBeEmpty)
		})

		Convey("F2.3: paged availability schemas keep semantic fields plus required page metadata", func() {
			tools := listToolsByName(t, cs)

			failures := pagedToolSchemaFailures(tools, "mlwh_samples_with_data_for_study", "samples")

			So(failures, ShouldBeEmpty)
		})

		Convey("F2.4: all count tool descriptions point freshness caveats at mlwh_freshness", func() {
			tools := listToolsByName(t, cs)
			failures := countToolDescriptionFailures(tools)

			So(failures, ShouldBeEmpty)
		})

		Convey("F2: all bare list tool descriptions point freshness caveats at mlwh_freshness", func() {
			tools := listToolsByName(t, cs)
			failures := bareListToolDescriptionFailures(tools)

			So(failures, ShouldBeEmpty)
		})

		Convey("I1.2: a resources listing includes mlwh://workflow and mcp-server://version", func() {
			resources := listResourceURIs(t, cs)

			So(resources, ShouldContainKey, "mlwh://workflow")
			So(resources, ShouldContainKey, "mcp-server://version")
		})

		Convey("I1.3: the MLWH provider's Name() returns \"mlwh\"", func() {
			provider, err := New(wa.RemoteConfig{BaseURL: stub.server.URL})
			So(err, ShouldBeNil)
			So(provider.Name(), ShouldEqual, "mlwh")
		})
	})
}

// listToolNames lists the server's tools over the MCP client and returns their
// names as a set, so an acceptance test can assert membership without putting a
// So() inside the listing loop (collect, then assert).
func listToolNames(t *testing.T, cs *mcp.ClientSession) map[string]struct{} {
	t.Helper()

	return toolNameSet(listToolsByName(t, cs))
}

func toolNameSet(tools map[string]*mcp.Tool) map[string]struct{} {
	names := make(map[string]struct{}, len(tools))
	for name := range tools {
		names[name] = struct{}{}
	}

	return names
}

// listToolsByName lists the server's tools over the MCP client and returns the
// full tool metadata by name, so F2 can assert descriptions and schemas as a
// client observes them.
func listToolsByName(t *testing.T, cs *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()

	res, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() returned error: %v", err)
	}

	tools := make(map[string]*mcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}

	return tools
}

func registryCountToolNames() map[string]string {
	names := map[string]string{}

	for _, entry := range wa.Registry {
		if strings.Contains(entry.Path, "/count") {
			names[entry.Method] = countToolNameForRegistryMethod(entry.Method)
		}
	}

	return names
}

func countToolNameForRegistryMethod(method string) string {
	if strings.HasPrefix(method, "CountFindSamplesBy") {
		return "mlwh_count_find_samples"
	}

	switch method {
	case "CountSampleSearch":
		return "mlwh_count_samples"
	case "CountStudySearch":
		return "mlwh_count_studies_search"
	case "CountSamplesWithData":
		return "mlwh_count_samples_with_data_for_study"
	default:
		return "mlwh_count_" + camelToSnake(strings.TrimPrefix(method, "Count"))
	}
}

func camelToSnake(value string) string {
	var builder strings.Builder

	for i, char := range value {
		if i > 0 && char >= 'A' && char <= 'Z' {
			builder.WriteByte('_')
		}

		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}

		builder.WriteRune(char)
	}

	snaked := builder.String()
	snaked = strings.ReplaceAll(snaked, "i_r_o_d_s", "irods")
	snaked = strings.ReplaceAll(snaked, "i_d", "id")

	return snaked
}

func existingCountToolRegistryFailures(registered map[string]struct{}, expected map[string]string) []string {
	expectedNames := map[string]struct{}{}
	for _, name := range expected {
		expectedNames[name] = struct{}{}
	}

	var failures []string
	for _, name := range fullSurfaceTools {
		if !strings.HasPrefix(name, "mlwh_count_") {
			continue
		}
		if _, ok := registered[name]; !ok {
			failures = append(failures, name+" is not registered")
		}
		if _, ok := expectedNames[name]; !ok {
			failures = append(failures, name+" has no Registry count method")
		}
	}

	return failures
}

func pagedToolSchemaFailures(tools map[string]*mcp.Tool, toolName, semanticField string) []string {
	tool, ok := tools[toolName]
	if !ok {
		return []string{toolName + " is not registered"}
	}

	schema, ok := tool.OutputSchema.(map[string]any)
	if !ok {
		return []string{toolName + " output schema is not an object map"}
	}

	var failures []string

	if schema["type"] != "object" {
		failures = append(failures, toolName+" output schema type is not object")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return append(failures, toolName+" output schema has no properties object")
	}

	for _, field := range []string{semanticField, "total", "next_offset"} {
		if _, ok := properties[field]; !ok {
			failures = append(failures, toolName+" output schema is missing property "+field)
		}
	}

	required := requiredFieldSet(schema)
	for _, field := range []string{"total", "next_offset"} {
		if _, ok := required[field]; !ok {
			failures = append(failures, toolName+" output schema does not require "+field)
		}
	}

	return failures
}

func requiredFieldSet(schema map[string]any) map[string]struct{} {
	required := map[string]struct{}{}

	fields, _ := schema["required"].([]any)
	for _, field := range fields {
		name, ok := field.(string)
		if ok {
			required[name] = struct{}{}
		}
	}

	return required
}

func countToolDescriptionFailures(tools map[string]*mcp.Tool) []string {
	var failures []string

	for name, tool := range tools {
		if !strings.HasPrefix(name, "mlwh_count_") {
			continue
		}
		if !strings.Contains(tool.Description, "Count responses have no cache_synced_at") ||
			!strings.Contains(tool.Description, "mlwh_freshness") {
			failures = append(failures, name)
		}
	}

	return failures
}

func bareListToolDescriptionFailures(tools map[string]*mcp.Tool) []string {
	var failures []string

	for _, name := range bareListTools {
		tool, ok := tools[name]
		if !ok {
			failures = append(failures, name+" is not registered")

			continue
		}

		if !strings.Contains(tool.Description, "Bare list responses have no cache_synced_at") ||
			!strings.Contains(tool.Description, "mlwh_freshness") {
			failures = append(failures, name)
		}
	}

	return failures
}

// listResourceURIs lists the server's resources over the MCP client and returns
// their URIs as a set, so an acceptance test can assert membership without a
// So() inside the listing loop.
func listResourceURIs(t *testing.T, cs *mcp.ClientSession) map[string]struct{} {
	t.Helper()

	res, err := cs.ListResources(context.Background(), &mcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources() returned error: %v", err)
	}

	uris := make(map[string]struct{}, len(res.Resources))
	for _, resource := range res.Resources {
		uris[resource.URI] = struct{}{}
	}

	return uris
}

func TestProviderConfig(t *testing.T) {
	Convey("Config.Resolve reads the MLWH provider settings from the environment", t, func() {
		Convey("H2.1: MLWH_BASE_URL from env populates RemoteConfig.BaseURL", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.BaseURL, ShouldEqual, "http://stub.example")
		})

		Convey("H2.3: MLWH_TIMEOUT=5s yields a 5s timeout and leaves CacheTTL zero", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_TIMEOUT", "5s")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.Timeout, ShouldEqual, 5*time.Second)
			So(cfg.CacheTTL, ShouldEqual, time.Duration(0))
		})

		Convey("the optional CA cert path flows through to RemoteConfig.CACert", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_CA_CERT", "/etc/ssl/mlwh-ca.pem")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.CACert, ShouldEqual, "/etc/ssl/mlwh-ca.pem")
		})

		Convey("an unparseable MLWH_TIMEOUT is a clear error naming the timeout", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_TIMEOUT", "soon")

			_, err := (Config{}).Resolve(nil)
			So(err, ShouldNotBeNil)
			So(strings.ToLower(err.Error()), ShouldContainSubstring, "timeout")
		})

		Convey("flag-sourced values take precedence over the environment", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://from-env.example")

			cfg, err := Config{BaseURL: "http://from-flag.example"}.Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.BaseURL, ShouldEqual, "http://from-flag.example")
		})

		Convey("MaxToolResultBytes defaults to the MLWH guard budget", func() {
			clearMLWHEnv(t)

			maxBytes, err := (Config{}).ResolveMaxToolResultBytes(nil)
			So(err, ShouldBeNil)
			So(maxBytes, ShouldEqual, DefaultMaxToolResultBytes)
		})

		Convey("MLWH_MAX_TOOL_RESULT_BYTES resolves to an integer byte budget", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_MAX_TOOL_RESULT_BYTES", "4096")

			maxBytes, err := (Config{}).ResolveMaxToolResultBytes(nil)
			So(err, ShouldBeNil)
			So(maxBytes, ShouldEqual, 4096)
		})
	})
}

func TestProviderBindFlags(t *testing.T) {
	Convey("BindFlags registers the four --mlwh-* flags so cmd/mlwh-mcp-server can wire them", t, func() {
		clearMLWHEnv(t)

		var cfg Config
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.BindFlags(fs)

		err := fs.Parse([]string{
			"--mlwh-base-url", "http://flagged.example",
			"--mlwh-ca-cert", "/tmp/ca.pem",
			"--mlwh-timeout", "9s",
			"--mlwh-max-tool-result-bytes", "2048",
		})
		So(err, ShouldBeNil)

		So(cfg.BaseURL, ShouldEqual, "http://flagged.example")
		So(cfg.CACert, ShouldEqual, "/tmp/ca.pem")
		So(cfg.Timeout, ShouldEqual, "9s")
		So(cfg.MaxToolResultBytes, ShouldEqual, "2048")

		resolved, err := cfg.Resolve(nil)
		So(err, ShouldBeNil)
		So(resolved.BaseURL, ShouldEqual, "http://flagged.example")
		So(resolved.CACert, ShouldEqual, "/tmp/ca.pem")
		So(resolved.Timeout, ShouldEqual, 9*time.Second)

		maxBytes, err := cfg.ResolveMaxToolResultBytes(nil)
		So(err, ShouldBeNil)
		So(maxBytes, ShouldEqual, 2048)
	})
}

// clearMLWHEnv blanks every MLWH_* setting for the duration of the test (via
// t.Setenv, which restores them afterwards), so a value already present in the
// developer's or CI environment cannot leak into a "nothing set" assertion.
func clearMLWHEnv(t *testing.T) {
	t.Helper()

	t.Setenv("MLWH_BASE_URL", "")
	t.Setenv("MLWH_CA_CERT", "")
	t.Setenv("MLWH_TIMEOUT", "")
	t.Setenv("MLWH_MAX_TOOL_RESULT_BYTES", "")
}

func TestF3PaginationBoundaries(t *testing.T) {
	Convey("F3.1: every new paged list applies its default and maximum and rejects invalid MCP-owned pagination before HTTP", t, func() {
		for _, testCase := range f3PagedToolCases() {
			Convey(testCase.name, func() {
				stub := newStubMLWH(t)
				if testCase.headerPage {
					stub.respondJSONWithHeaders(testCase.path, http.StatusOK, testCase.body, http.Header{
						"X-Total-Count": {"0"}, "X-Next-Offset": {"-1"},
					})
				} else {
					stub.respondJSON(testCase.path, http.StatusOK, testCase.body)
				}

				cs, cleanup := runMLWHServerWithClient(t, stub)
				defer cleanup()

				result := callTool(t, cs, testCase.name, maps.Clone(testCase.arguments))
				So(result.IsError, ShouldBeFalse)
				request, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(request.Path, ShouldEqual, testCase.path)
				So(request.Query.Get("limit"), ShouldEqual, testCase.defaultLimit)
				if testCase.supportsOffset {
					So(request.Query.Get("offset"), ShouldEqual, "0")
				}

				beforeNegativeLimit := stub.requestCount()
				negativeLimit := maps.Clone(testCase.arguments)
				negativeLimit["limit"] = -1
				result = callTool(t, cs, testCase.name, negativeLimit)
				So(result.IsError, ShouldBeTrue)
				So(firstTextContent(result), ShouldContainSubstring, "non-negative")
				So(stub.requestCount(), ShouldEqual, beforeNegativeLimit)

				if testCase.supportsOffset {
					beforeNegativeOffset := stub.requestCount()
					negativeOffset := maps.Clone(testCase.arguments)
					negativeOffset["offset"] = -1
					result = callTool(t, cs, testCase.name, negativeOffset)
					So(result.IsError, ShouldBeTrue)
					So(firstTextContent(result), ShouldContainSubstring, "non-negative")
					So(stub.requestCount(), ShouldEqual, beforeNegativeOffset)
				}

				maximum := maps.Clone(testCase.arguments)
				maximum["limit"] = 1000
				result = callTool(t, cs, testCase.name, maximum)
				So(result.IsError, ShouldBeFalse)
				request, ok = stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(request.Query.Get("limit"), ShouldEqual, "1000")

				beforeOverMaximum := stub.requestCount()
				overMaximum := maps.Clone(testCase.arguments)
				overMaximum["limit"] = 1001
				result = callTool(t, cs, testCase.name, overMaximum)
				So(result.IsError, ShouldBeTrue)
				So(firstTextContent(result), ShouldContainSubstring, "maximum of 1000")
				So(stub.requestCount(), ShouldEqual, beforeOverMaximum)
			})
		}
	})
}
