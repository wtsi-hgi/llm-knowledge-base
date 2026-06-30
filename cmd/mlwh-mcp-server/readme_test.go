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

package main

import (
	"os"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestREADMEHTTPDocs(t *testing.T) {
	Convey("Given the README contents for admin-run HTTP mode", t, func() {
		readmeBytes, err := os.ReadFile("../../README.md")
		So(err, ShouldBeNil)

		readme := string(readmeBytes)
		httpDocs := readmeSection(
			readme,
			"## Run a shared HTTP service",
			"## Use the shared HTTP service from agent CLIs",
		)
		httpClientDocs := readmeSection(
			readme,
			"## Use the shared HTTP service from agent CLIs",
			"## Use it with Claude Code",
		)

		Convey("D1.1: it gives the upstream URL and explicit HTTP startup command", func() {
			So(httpDocs, ShouldContainSubstring, "MLWH_BASE_URL=http://mlwh.internal:8080")
			So(httpDocs, ShouldContainSubstring, "mlwh-mcp-server --http 127.0.0.1:8081")
		})

		Convey("D1.2: it states the MCP and health paths and internal plain HTTP deployment model", func() {
			So(httpDocs, ShouldContainSubstring, "/mcp")
			So(httpDocs, ShouldContainSubstring, "/health")
			So(httpDocs, ShouldContainSubstring, "unauthenticated plain HTTP")
			So(httpDocs, ShouldContainSubstring, "internal-network deployment")
		})

		Convey("D1.3: it includes a minimal service example setting both required env vars", func() {
			So(httpDocs, ShouldContainSubstring, "[Service]")
			So(httpDocs, ShouldContainSubstring, "Environment=MLWH_BASE_URL=http://mlwh.internal:8080")
			So(httpDocs, ShouldContainSubstring, "Environment=MLWH_HTTP_ADDR=")
			So(httpDocs, ShouldContainSubstring, "ExecStart=/usr/local/bin/mlwh-mcp-server")
		})

		Convey("D1.4: this acceptance test is a GoConvey test in the command package", func() {
			So(t.Name(), ShouldEqual, "TestREADMEHTTPDocs")
		})

		Convey("D2.1: Claude Code HTTP docs include the URL-based CLI command", func() {
			So(httpClientDocs, ShouldContainSubstring, "claude mcp add --transport http mlwh http://mlwh-mcp.internal:8080/mcp")
		})

		Convey("D2.2: Claude Code JSON docs include an HTTP mcpServers.mlwh entry", func() {
			So(httpClientDocs, ShouldContainSubstring, `"mcpServers"`)
			So(httpClientDocs, ShouldContainSubstring, `"mlwh"`)
			So(httpClientDocs, ShouldContainSubstring, `"type": "http"`)
			So(httpClientDocs, ShouldContainSubstring, `"url": "http://mlwh-mcp.internal:8080/mcp"`)
		})

		Convey("D2.3: Codex HTTP docs include the URL-based CLI command", func() {
			So(httpClientDocs, ShouldContainSubstring, "codex mcp add mlwh --url http://mlwh-mcp.internal:8080/mcp")
		})

		Convey("D2.4: Codex TOML docs include the URL-based server entry", func() {
			So(httpClientDocs, ShouldContainSubstring, "[mcp_servers.mlwh]")
			So(httpClientDocs, ShouldContainSubstring, `url = "http://mlwh-mcp.internal:8080/mcp"`)
		})

		Convey("D2.5: shared HTTP client docs say users do not install or run a local binary", func() {
			So(singleSpaced(httpClientDocs), ShouldContainSubstring, "do not install or run a local `mlwh-mcp-server` binary")
		})

		Convey("D2.6: shared HTTP client examples do not include local command forms", func() {
			So(httpClientDocs, ShouldNotContainSubstring, `"command": "mlwh-mcp-server"`)
			So(httpClientDocs, ShouldNotContainSubstring, `command = "mlwh-mcp-server"`)
			So(httpClientDocs, ShouldNotContainSubstring, "-- mlwh-mcp-server")
		})
	})
}

func readmeSection(readme, startHeading, endHeading string) string {
	start := strings.Index(readme, startHeading)
	if start == -1 {
		return ""
	}

	section := readme[start:]
	if end := strings.Index(section, endHeading); end != -1 {
		return section[:end]
	}

	return section
}

func singleSpaced(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
