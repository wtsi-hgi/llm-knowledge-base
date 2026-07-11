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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// callEndpointDescription is the LLM-facing description for mlwh_call_endpoint. It
// names the escape-hatch role (prefer the curated tools), points at the
// mlwh://workflow resource as the source of valid Method names and their path and
// query parameters, and explains the inputs, pagination wrapper, and freshness
// caveat so an agent can dispatch any Registry endpoint that lacks a curated
// tool.
const callEndpointDescription = "Escape hatch: call any MLWH endpoint by its Registry Method name. " +
	"Prefer the curated tools (search, resolve, detail, fan-out, freshness) where one exists; use this " +
	"only to reach an endpoint that has no curated tool. Choose method from the complete Registry-derived " +
	"input enum (e.g. \"ResolveStudy\", \"AllStudies\", \"Export\"); the mlwh://workflow resource contains " +
	"the EndpointReference catalogue with every endpoint's path and " +
	"query parameters. Supply path_params in the endpoint's declared order and each query_params value as " +
	"a string or ordered array of strings (including limit/offset for paginated endpoints). Unknown methods " +
	"and the wrong number of path params are " +
	"rejected. The decoded result is returned untyped (no per-endpoint output schema); dynamic calls with " +
	"X-Total-Count or X-Next-Offset are wrapped as result, total, and next_offset. Responses with no " +
	"cache_synced_at need mlwh_freshness for the cache as-of caveat."

// QueryParameterValues holds the one or more ordered values for a query key.
// It accepts both the original scalar string input and an array of strings.
type QueryParameterValues []string

// UnmarshalJSON accepts one query parameter as either a scalar string or an
// ordered array of strings.
func (values *QueryParameterValues) UnmarshalJSON(data []byte) error {
	var scalar string
	if err := json.Unmarshal(data, &scalar); err == nil {
		*values = []string{scalar}

		return nil
	}

	var repeated []string
	if err := json.Unmarshal(data, &repeated); err != nil {
		return fmt.Errorf("query parameter must be a string or array of strings: %w", err)
	}
	if repeated == nil {
		return errors.New("query parameter must be a string or array of strings")
	}

	*values = repeated

	return nil
}

// CallInput is the input for mlwh_call_endpoint: a Registry Method name, the
// endpoint's path parameters in declaration order, and its query parameters. The
// Method and the path-param arity are validated by (*RemoteClient).Call itself,
// so the handler passes them through without a pre-check against the Registry.
type CallInput struct {
	Method      string                          `json:"method" jsonschema:"the Registry Method name to dispatch, e.g. ResolveStudy or AllStudies (see the mlwh://workflow resource)"`
	PathParams  []string                        `json:"path_params,omitempty" jsonschema:"the endpoint's path parameters, in the order the Registry declares them"`
	QueryParams map[string]QueryParameterValues `json:"query_params,omitempty" jsonschema:"the endpoint's query parameters; each value is a string or ordered array of strings"`
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
		InputSchema: callEndpointInputSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CallInput) (*mcp.CallToolResult, any, error) {
		return callEndpoint(ctx, client, in)
	})

	return nil
}

// callEndpoint dispatches the chosen Registry Method through
// (*RemoteClient).CallWithHeaders, which validates the Method (rejecting unknown
// methods) and the path-param arity itself, so no pre-check against the Registry
// is needed. It converts the input's query parameters to url.Values and, on a
// call error, maps it to a clear tool error whose message still names the
// offending method or arity (mapToolError preserves the upstream text); on
// success it returns the decoded value as the untyped Out for the SDK to place in
// StructuredContent, adding generic pagination metadata when upstream sent it.
func callEndpoint(ctx context.Context, client caller, in CallInput) (*mcp.CallToolResult, any, error) {
	decoded, headers, err := client.CallWithHeaders(ctx, in.Method, in.PathParams, queryValues(in.QueryParams))
	if err != nil {
		return core.ToolError[any](mapToolError(err))
	}

	return nil, dynamicResult(decoded, headers), nil
}

// queryValues converts the input's scalar or repeated query parameters to the
// url.Values the remote client's Call expects, preserving value order. A nil or
// empty map yields empty url.Values, so an endpoint with no query parameters is
// called with no query string.
func queryValues(params map[string]QueryParameterValues) url.Values {
	values := make(url.Values, len(params))
	for key, parameterValues := range params {
		values[key] = append([]string(nil), parameterValues...)
	}

	return values
}

func dynamicResult(decoded any, headers http.Header) any {
	if headers.Get("X-Total-Count") == "" && headers.Get("X-Next-Offset") == "" {
		return decoded
	}

	return map[string]any{
		"result":      decoded,
		"total":       headerInt(headers, "X-Total-Count", 0),
		"next_offset": headerInt(headers, "X-Next-Offset", -1),
	}
}

func headerInt(headers http.Header, name string, fallback int) int {
	raw := headers.Get(name)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return value
}

// caller is the (*RemoteClient).CallWithHeaders surface callEndpoint needs: the
// generic dispatcher keyed by Registry Method name. Depending on this method set
// rather than the concrete client keeps callEndpoint's contract explicit.
type caller interface {
	CallWithHeaders(ctx context.Context, method string, pathParams []string, query url.Values) (any, http.Header, error)
}
