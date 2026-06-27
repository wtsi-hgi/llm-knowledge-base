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
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// aToETools is the full set of tool names the MLWH provider registers for
// stories A-E: the sample/study search and count tools (A), the
// resolve/classify, unified find-samples, and expand tools (B), the detail and
// fan-out enumeration tools (C), the freshness tool (D), and the generic
// escape-hatch tool (E). I1.1 requires at least the eight headline tools listed
// in the spec; this fuller set strengthens the assertion so a dropped tool in
// any group fails the test, not just one of the eight.
var aToETools = []string{
	// A: search and count (5).
	"mlwh_search_samples",
	"mlwh_count_samples",
	"mlwh_search_studies",
	"mlwh_count_studies_search",
	"mlwh_count_studies",

	// B: resolve/classify (7), unified find-samples (1), expand (3).
	"mlwh_classify_identifier",
	"mlwh_resolve_sample",
	"mlwh_resolve_sample_name",
	"mlwh_resolve_study",
	"mlwh_resolve_run",
	"mlwh_resolve_library",
	"mlwh_resolve_library_identifier",
	"mlwh_find_samples",
	"mlwh_expand_identifier",
	"mlwh_expand_search_values",
	"mlwh_expand_sample_search_values",

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
	"mlwh_count_samples_for_study",

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

		Convey("I1.1: a tools listing includes the MLWH tools from stories A-E", func() {
			registered := listToolNames(t, cs)

			for _, name := range i1HeadlineTools {
				So(registered, ShouldContainKey, name)
			}

			// Strengthen: the full A-E surface registers, so a dropped tool in
			// any group is caught, not only one of the eight headline tools.
			var missing []string

			for _, name := range aToETools {
				if _, ok := registered[name]; !ok {
					missing = append(missing, name)
				}
			}

			So(missing, ShouldBeEmpty)
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

	res, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() returned error: %v", err)
	}

	names := make(map[string]struct{}, len(res.Tools))
	for _, tool := range res.Tools {
		names[tool.Name] = struct{}{}
	}

	return names
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
	})
}

func TestProviderBindFlags(t *testing.T) {
	Convey("BindFlags registers the three --mlwh-* flags so cmd/mlwh-mcp-server can wire them", t, func() {
		clearMLWHEnv(t)

		var cfg Config
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.BindFlags(fs)

		err := fs.Parse([]string{
			"--mlwh-base-url", "http://flagged.example",
			"--mlwh-ca-cert", "/tmp/ca.pem",
			"--mlwh-timeout", "9s",
		})
		So(err, ShouldBeNil)

		So(cfg.BaseURL, ShouldEqual, "http://flagged.example")
		So(cfg.CACert, ShouldEqual, "/tmp/ca.pem")
		So(cfg.Timeout, ShouldEqual, "9s")

		resolved, err := cfg.Resolve(nil)
		So(err, ShouldBeNil)
		So(resolved.BaseURL, ShouldEqual, "http://flagged.example")
		So(resolved.CACert, ShouldEqual, "/tmp/ca.pem")
		So(resolved.Timeout, ShouldEqual, 9*time.Second)
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
}
