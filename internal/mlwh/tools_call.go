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
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// callEndpointDescription is the LLM-facing description for mlwh_call_endpoint. It
// names the escape-hatch role (prefer the curated tools), points at the
// mlwh://workflow resource as the source of valid Method names and their path and
// query parameters, and explains the three inputs so an agent can dispatch any
// Registry endpoint that lacks a curated tool.
const callEndpointDescription = "Escape hatch: call any MLWH endpoint by its Registry Method name. " +
	"Prefer the curated tools (search, resolve, detail, fan-out, freshness) where one exists; use this " +
	"only to reach an endpoint that has no curated tool. Set method to the Registry Method name (e.g. " +
	"\"ResolveStudy\", \"AllStudies\"); the mlwh://workflow resource lists every Method with its path and " +
	"query parameters. Supply path_params in the endpoint's declared order and query_params (including " +
	"limit/offset for paginated endpoints). Unknown methods and the wrong number of path params are " +
	"rejected. The decoded result is returned untyped (no per-endpoint output schema)."

// CallInput is the input for mlwh_call_endpoint: a Registry Method name, the
// endpoint's path parameters in declaration order, and its query parameters. The
// Method and the path-param arity are validated by (*RemoteClient).Call itself,
// so the handler passes them through without a pre-check against the Registry.
type CallInput struct {
	Method      string            `json:"method" jsonschema:"the Registry Method name to dispatch, e.g. ResolveStudy or AllStudies (see the mlwh://workflow resource)"`
	PathParams  []string          `json:"path_params,omitempty" jsonschema:"the endpoint's path parameters, in the order the Registry declares them"`
	QueryParams map[string]string `json:"query_params,omitempty" jsonschema:"the endpoint's query parameters (including limit/offset for paginated endpoints)"`
}

// registerCallTool adds the generic mlwh_call_endpoint escape-hatch tool (Story
// E1) to the server through the Registrar. Its Out is any (an UNTYPED JSON
// passthrough), so it deliberately leaves Tool.OutputSchema nil: the SDK then
// omits the output schema and places whatever the handler returns in
// StructuredContent (and the JSON text in Content). This registrar never fails;
// it returns an error only to share the registrar signature of the other tool
// groups.
func (p *provider) registerCallTool(r core.Registrar) error {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        "mlwh_call_endpoint",
		Description: callEndpointDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CallInput) (*mcp.CallToolResult, any, error) {
		return callEndpoint(ctx, client, in)
	})

	return nil
}

// callEndpoint dispatches the chosen Registry Method through
// (*RemoteClient).Call, which validates the Method (rejecting unknown methods)
// and the path-param arity itself, so no pre-check against the Registry is
// needed. It converts the input's query parameters to url.Values and, on a Call
// error, maps it to a clear tool error whose message still names the offending
// method or arity (mapToolError preserves the upstream text); on success it
// returns the decoded value as the untyped Out for the SDK to place in
// StructuredContent.
func callEndpoint(ctx context.Context, client caller, in CallInput) (*mcp.CallToolResult, any, error) {
	decoded, err := client.Call(ctx, in.Method, in.PathParams, queryValues(in.QueryParams))
	if err != nil {
		return core.ToolError[any](mapToolError(err))
	}

	return nil, decoded, nil
}

// queryValues converts the input's query parameters (a flat string map) to the
// url.Values the remote client's Call expects, giving each key a single value.
// A nil or empty map yields empty url.Values, so an endpoint with no query
// parameters is called with no query string.
func queryValues(params map[string]string) url.Values {
	values := make(url.Values, len(params))
	for key, value := range params {
		values.Set(key, value)
	}

	return values
}

// caller is the (*RemoteClient).Call surface callEndpoint needs: the generic
// dispatcher keyed by Registry Method name. Depending on this method set rather
// than the concrete client keeps callEndpoint's contract explicit.
type caller interface {
	Call(ctx context.Context, method string, pathParams []string, query url.Values) (any, error)
}
