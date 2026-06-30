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
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// findSamplesDescription is the LLM-facing description of the unified
// mlwh_find_samples tool. It explains the exact-match semantics and lists the
// supported field values so the agent picks one tool with a field enum instead
// of choosing among five flat finders.
const findSamplesDescription = "Find samples by an exact field value. Choose the field to match on " +
	"(sanger_id, lims_id, accession, supplier_name, or library_type) and give the exact value; " +
	"returns every sample whose chosen field equals that value (an exact match, not a search). " +
	"This unifies the five per-field sample finders behind one field enum."

const countFindSamplesDescription = "Count samples by an exact field value. Choose the field to match on " +
	"(sanger_id, lims_id, accession, supplier_name, or library_type) and give the exact value; " +
	"returns the number of samples whose chosen field equals that value (an exact match, not a search). " +
	"This unifies the five per-field sample-finder counts behind the same field enum as mlwh_find_samples." +
	countFreshnessNote

// resolver pairs one resolve/classify tool name with the Registry Method it
// derives its description from and the typed client call it dispatches to. The
// seven resolvers all share the resolveInput/wa.Match shape, so registering them
// is a single loop over this table.
type resolver struct {
	name   string
	method string
	call   func(client *wa.RemoteClient, ctx context.Context, identifier string) (wa.Match, error)
}

// resolvers returns the seven resolve/classify tools (Story B1) in the order
// their underlying methods appear in the Queryer surface. Each call is the
// matching typed RemoteClient method, so the tool inherits its exact wire
// contract; the description is derived from the same Registry entry (by method).
func resolvers() []resolver {
	return []resolver{
		{"mlwh_classify_identifier", "ClassifyIdentifier", (*wa.RemoteClient).ClassifyIdentifier},
		{"mlwh_resolve_sample", "ResolveSample", (*wa.RemoteClient).ResolveSample},
		{"mlwh_resolve_sample_name", "ResolveSampleName", (*wa.RemoteClient).ResolveSampleName},
		{"mlwh_resolve_study", "ResolveStudy", (*wa.RemoteClient).ResolveStudy},
		{"mlwh_resolve_run", "ResolveRun", (*wa.RemoteClient).ResolveRun},
		{"mlwh_resolve_library", "ResolveLibrary", (*wa.RemoteClient).ResolveLibrary},
		{"mlwh_resolve_library_identifier", "ResolveLibraryIdentifier", (*wa.RemoteClient).ResolveLibraryIdentifier},
	}
}

// resolveDescription derives a resolve/classify tool's LLM-facing description
// from its Registry entry, combining the entry's short Summary and longer
// Description so the tool's documentation tracks the upstream Registry rather
// than being hand-maintained. A missing Registry method is an error.
func resolveDescription(method string) (string, error) {
	entry, ok := registryEntryByMethod(method)
	if !ok {
		return "", fmt.Errorf("mlwh: no Registry entry for method %q", method)
	}

	return entry.Summary + ": " + entry.Description, nil
}

// registryEntryByMethod returns the Registry endpoint whose Method equals
// method, or false if none does. Tool descriptions and the find_samples
// field<->method mapping are keyed off the live Registry through it, so a
// renamed or removed upstream method is caught rather than silently mis-wired.
func registryEntryByMethod(method string) (wa.Endpoint, bool) {
	for _, entry := range wa.Registry {
		if entry.Method == method {
			return entry, true
		}
	}

	return wa.Endpoint{}, false
}

// findSamplesInputSchema builds the pre-set input schema for mlwh_find_samples,
// constraining the field property to the code-sourced enum (findSamplesFieldEnum,
// derived from the FindSamplesBy* Registry methods in Registry order). The SDK
// validates input against this schema before the handler runs, so an out-of-enum
// field is rejected without an HTTP call (Story B2.3).
func findSamplesInputSchema() map[string]any {
	return enumStringInputSchema(
		"field", findSamplesFieldEnum(), "the exact sample field to match on",
		"value", "the exact value the chosen field must equal",
	)
}

// expandInputSchema builds the pre-set input schema for the expand tools,
// constraining the kind property to the code-sourced enum (identifierKindEnum,
// the string values of wa.IdentifierKinds()). The SDK validates input against it
// before the handler runs, so an out-of-enum kind is rejected without an HTTP
// call (Story B3.2).
func expandInputSchema() map[string]any {
	return enumStringInputSchema(
		"kind", identifierKindEnum(), "the kind of the canonical identifier to expand",
		"canonical", "the canonical identifier value to expand",
	)
}

// enumStringInputSchema builds an object input schema with two required string
// properties: an enum-constrained property (enumName, with the given enum values
// and description) and a free-text property (freeName, with its description). It
// is the shared shape behind the enum-driven find/expand tools, so the
// code-sourced enum is the only schema input that differs between them.
func enumStringInputSchema(enumName string, enumValues []string, enumDescription, freeName, freeDescription string) map[string]any {
	enum := make([]any, len(enumValues))
	for i, value := range enumValues {
		enum[i] = value
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			enumName: map[string]any{
				"type":        "string",
				"description": enumDescription,
				"enum":        enum,
			},
			freeName: map[string]any{
				"type":        "string",
				"description": freeDescription,
			},
		},
		"required": []any{enumName, freeName},
	}
}

// valuesOutputSchema is the output schema for mlwh_expand_sample_search_values:
// an object with a single values property that is an array of strings, matching
// the valuesResult wrapper. The element is a plain string (not an OpenAPI
// component), so the schema is built directly rather than via outputSchemaForSlice.
func valuesOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"values": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []any{"values"},
	}
}

func unsupportedFindSamplesFieldError(field string) error {
	return fmt.Errorf(
		"unknown field %q; supported fields are: %s",
		field,
		strings.Join(findSamplesFieldEnum(), ", "),
	)
}

// registerResolveTools adds the resolve/classify (Story B1), unified find-samples
// (Story B2), and expand (Story B3) tools to the server through the Registrar.
// The typed tools pre-set their OpenAPI-sourced output schema so the upstream
// doc: field descriptions survive (the SDK's own reflection would drop them),
// and the enum-driven tools pre-set an input schema carrying the code-sourced
// enum so an out-of-enum value is rejected by the SDK before any HTTP call.
// Building a schema fails only on a programming error (the schemas and enums are
// compiled in), so such a failure is surfaced as a registration error.
func (p *provider) registerResolveTools(r core.Registrar) error {
	matchSchema, err := outputSchemaFor("Match")
	if err != nil {
		return fmt.Errorf("mlwh: build match output schema: %w", err)
	}

	if err := p.registerResolvers(r, matchSchema); err != nil {
		return err
	}

	if err := p.addFindSamples(r); err != nil {
		return fmt.Errorf("mlwh: register find_samples tool: %w", err)
	}

	if err := p.addCountFindSamples(r); err != nil {
		return fmt.Errorf("mlwh: register count_find_samples tool: %w", err)
	}

	if err := p.registerExpandTools(r); err != nil {
		return err
	}

	return nil
}

// resolveInput is the shared input for the seven resolve/classify tools (Story
// B1): a single raw identifier the corresponding resolver turns into a canonical
// wa.Match.
type resolveInput struct {
	Identifier string `json:"identifier" jsonschema:"the raw identifier to resolve or classify"`
}

// registerResolvers adds the seven resolve/classify tools, each sharing the
// resolveInput/wa.Match shape and the pre-set Match output schema. A tool whose
// Registry method is missing (a contract drift) is a registration error rather
// than a silently mis-described tool.
func (p *provider) registerResolvers(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	for _, res := range resolvers() {
		description, err := resolveDescription(res.method)
		if err != nil {
			return err
		}

		call := res.call

		mcp.AddTool(r.Server(), &mcp.Tool{
			Name:         res.name,
			Description:  description,
			OutputSchema: outputSchema,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in resolveInput) (*mcp.CallToolResult, wa.Match, error) {
			match, err := call(client, ctx, in.Identifier)
			if err != nil {
				return core.ToolError[wa.Match](mapToolError(err))
			}

			return nil, match, nil
		})
	}

	return nil
}

// findSamplesInput is the input for the unified mlwh_find_samples tool (Story
// B2): the exact field to match on (a constrained enum, validated against the
// pre-set input schema before the handler runs) and the value to match. The
// jsonschema tag supplies the property descriptions; the enum constraint is
// injected via the pre-set input schema (findSamplesInputSchema), not this tag.
type findSamplesInput struct {
	Field string `json:"field" jsonschema:"the exact sample field to match on"`
	Value string `json:"value" jsonschema:"the exact value the chosen field must equal"`
}

// addFindSamples registers mlwh_find_samples (Story B2). Its input schema is
// pre-set with the code-sourced field enum (findSamplesFieldEnum), so the SDK
// rejects an out-of-enum field before the handler runs (no HTTP call). The
// handler maps the chosen field to its FindSamplesBy* Registry method and
// dispatches through the generic Call, so the hyphenated upstream path (e.g.
// /find/sample/sanger-id/:id) is produced by the Registry, not hand-built; the
// decoded *[]wa.Sample is wrapped under {"samples":[...]} for the object-typed
// result MCP requires.
func (p *provider) addFindSamples(r core.Registrar) error {
	outputSchema, err := outputSchemaForSlice("samples", "Sample")
	if err != nil {
		return fmt.Errorf("build samples output schema: %w", err)
	}

	client := p.client
	methods := findSamplesMethods()

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_find_samples",
		Description:  findSamplesDescription,
		InputSchema:  findSamplesInputSchema(),
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in findSamplesInput) (*mcp.CallToolResult, samplesResult, error) {
		method, ok := methods[in.Field]
		if !ok {
			return core.ToolError[samplesResult](fmt.Errorf("unknown field %q; choose one of the supported sample fields", in.Field))
		}

		result, err := client.Call(ctx, method, []string{in.Value}, nil)
		if err != nil {
			return core.ToolError[samplesResult](mapToolError(err))
		}

		samples, ok := result.(*[]wa.Sample)
		if !ok {
			return core.ToolError[samplesResult](fmt.Errorf("mlwh: %s returned %T, want *[]Sample", method, result))
		}

		return nil, samplesResult{Samples: *samples}, nil
	})

	return nil
}

// addCountFindSamples registers mlwh_count_find_samples (D4). It shares
// mlwh_find_samples' input schema and clean field enum, then dispatches to the
// matching CountFindSamplesBy* Registry method by prefixing the selected
// FindSamplesBy* method. The count method's Registry path supplies the
// hyphenated /find/sample/*/:id/count endpoint and the Count result shape.
func (p *provider) addCountFindSamples(r core.Registrar) error {
	outputSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("build count output schema: %w", err)
	}

	client := p.client
	methods := findSamplesMethods()

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_find_samples",
		Description:  countFindSamplesDescription,
		InputSchema:  findSamplesInputSchema(),
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in findSamplesInput) (*mcp.CallToolResult, wa.Count, error) {
		method, ok := methods[in.Field]
		if !ok {
			return core.ToolError[wa.Count](unsupportedFindSamplesFieldError(in.Field))
		}

		countMethod := "Count" + method
		result, err := client.Call(ctx, countMethod, []string{in.Value}, nil)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		count, ok := result.(*wa.Count)
		if !ok {
			return core.ToolError[wa.Count](fmt.Errorf("mlwh: %s returned %T, want *Count", countMethod, result))
		}

		return nil, *count, nil
	})

	return nil
}

// expandTool registers one expand tool's name, the Registry method it derives
// its description from, and the closure that performs the typed call and returns
// the typed output the SDK packs into StructuredContent. Each expand tool shares
// the expandInput shape (kind enum + canonical) but differs in output type, so
// the closure also carries the pre-built output schema.
type expandTool struct {
	name         string
	method       string
	outputSchema map[string]any
	add          func(p *provider, r core.Registrar, t *mcp.Tool)
}

// registerExpandTools adds the three expand tools (Story B3), each with the
// shared kind-enum input schema and its own output schema and typed handler. A
// missing Registry method or a schema-build failure is a registration error.
func (p *provider) registerExpandTools(r core.Registrar) error {
	taggedIDsSchema, err := outputSchemaForSlice("tagged_ids", "TaggedID")
	if err != nil {
		return fmt.Errorf("mlwh: build tagged_ids output schema: %w", err)
	}

	searchValuesSchema, err := outputSchemaFor("SearchValues")
	if err != nil {
		return fmt.Errorf("mlwh: build search_values output schema: %w", err)
	}

	tools := []expandTool{
		{"mlwh_expand_identifier", "ExpandIdentifier", taggedIDsSchema, (*provider).addExpandIdentifier},
		{"mlwh_expand_search_values", "ExpandSearchValues", searchValuesSchema, (*provider).addExpandSearchValues},
		{"mlwh_expand_sample_search_values", "ExpandSampleSearchValues", valuesOutputSchema(), (*provider).addExpandSampleSearchValues},
	}

	for _, tool := range tools {
		description, err := resolveDescription(tool.method)
		if err != nil {
			return err
		}

		tool.add(p, r, &mcp.Tool{
			Name:         tool.name,
			Description:  description,
			InputSchema:  expandInputSchema(),
			OutputSchema: tool.outputSchema,
		})
	}

	return nil
}

// expandInput is the shared input for the three expand tools (Story B3): the
// kind of the canonical identifier (a constrained enum sourced from
// wa.IdentifierKinds(), validated against the pre-set input schema) and its
// canonical value.
type expandInput struct {
	Kind      string `json:"kind" jsonschema:"the kind of the canonical identifier to expand"`
	Canonical string `json:"canonical" jsonschema:"the canonical identifier value to expand"`
}

// addExpandIdentifier registers mlwh_expand_identifier (Story B3): it expands the
// canonical identifier of the given kind into its related canonical identifiers,
// wrapping the []wa.TaggedID under {"tagged_ids":[...]}.
func (p *provider) addExpandIdentifier(r core.Registrar, t *mcp.Tool) {
	client := p.client

	mcp.AddTool(r.Server(), t, func(ctx context.Context, _ *mcp.CallToolRequest, in expandInput) (*mcp.CallToolResult, taggedIDsResult, error) {
		tagged, err := client.ExpandIdentifier(ctx, wa.IdentifierKind(in.Kind), in.Canonical)
		if err != nil {
			return core.ToolError[taggedIDsResult](mapToolError(err))
		}

		return nil, taggedIDsResult{TaggedIDs: tagged}, nil
	})
}

// addExpandSearchValues registers mlwh_expand_search_values (Story B3): it
// expands the identifier into the sample/run/lane values used to search
// downstream results, returning the wa.SearchValues object unchanged.
func (p *provider) addExpandSearchValues(r core.Registrar, t *mcp.Tool) {
	client := p.client

	mcp.AddTool(r.Server(), t, func(ctx context.Context, _ *mcp.CallToolRequest, in expandInput) (*mcp.CallToolResult, wa.SearchValues, error) {
		values, err := client.ExpandSearchValues(ctx, wa.IdentifierKind(in.Kind), in.Canonical)
		if err != nil {
			return core.ToolError[wa.SearchValues](mapToolError(err))
		}

		return nil, values, nil
	})
}

// addExpandSampleSearchValues registers mlwh_expand_sample_search_values (Story
// B3): it expands the identifier into the list of sample values used to search
// downstream results, wrapping the []string under {"values":[...]} so the
// structured result is the object MCP requires.
func (p *provider) addExpandSampleSearchValues(r core.Registrar, t *mcp.Tool) {
	client := p.client

	mcp.AddTool(r.Server(), t, func(ctx context.Context, _ *mcp.CallToolRequest, in expandInput) (*mcp.CallToolResult, valuesResult, error) {
		values, err := client.ExpandSampleSearchValues(ctx, wa.IdentifierKind(in.Kind), in.Canonical)
		if err != nil {
			return core.ToolError[valuesResult](mapToolError(err))
		}

		return nil, valuesResult{Values: values}, nil
	})
}
