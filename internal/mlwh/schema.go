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
	"fmt"
	"strings"

	wa "github.com/wtsi-hgi/wa/mlwh"
)

// openAPISchemaRefPrefix is the JSON-pointer prefix every component schema is
// referenced by ($ref) in wa.OpenAPIDocument().
const openAPISchemaRefPrefix = "#/components/schemas/"

// findSamplesMethodPrefix is the Registry Method prefix shared by the five
// exact-field sample finders the mlwh_find_samples tool unifies.
const findSamplesMethodPrefix = "FindSamplesBy"

// Shared typed-tool pagination bounds. A missing or non-positive limit becomes
// one bounded page; values above pagedMaxLimit are rejected before any HTTP
// request reaches MLWH.
const (
	pagedDefaultLimit  = 100
	pagedDefaultOffset = 0
	pagedMaxLimit      = 1000
)

// slice wrapper structs give each list-returning tool an object-typed Out, as
// MCP requires (output schemas and StructuredContent must be JSON objects, not
// bare arrays). Each wraps exactly one slice under a JSON field name that is
// consistent per element type. The output schema for each (see
// outputSchemaForSlice) describes the same one-property object.

// samplesResult wraps a []wa.Sample as {"samples":[...]}.
type samplesResult struct {
	Samples []wa.Sample `json:"samples"`
}

// studiesResult wraps a []wa.Study as {"studies":[...]}.
type studiesResult struct {
	Studies []wa.Study `json:"studies"`
}

// runsResult wraps a []wa.Run as {"runs":[...]}.
type runsResult struct {
	Runs []wa.Run `json:"runs"`
}

// lanesResult wraps a []wa.Lane as {"lanes":[...]}.
type lanesResult struct {
	Lanes []wa.Lane `json:"lanes"`
}

// irodsPathsResult wraps a []wa.IRODSPath as {"irods_paths":[...]}.
type irodsPathsResult struct {
	IRODSPaths []wa.IRODSPath `json:"irods_paths"`
}

// librariesResult wraps a []wa.Library as {"libraries":[...]}.
type librariesResult struct {
	Libraries []wa.Library `json:"libraries"`
}

// taggedIDsResult wraps a []wa.TaggedID as {"tagged_ids":[...]}.
type taggedIDsResult struct {
	TaggedIDs []wa.TaggedID `json:"tagged_ids"`
}

// valuesResult wraps a []string as {"values":[...]}.
type valuesResult struct {
	Values []string `json:"values"`
}

// pagedSamplesResult wraps a header-aware sample page as
// {"samples":[...],"total":N,"next_offset":M}.
type pagedSamplesResult struct {
	Samples    []wa.Sample `json:"samples"`
	Total      int         `json:"total"`
	NextOffset int         `json:"next_offset"`
}

// pagedStudiesResult wraps a header-aware study page as
// {"studies":[...],"total":N,"next_offset":M}.
type pagedStudiesResult struct {
	Studies    []wa.Study `json:"studies"`
	Total      int        `json:"total"`
	NextOffset int        `json:"next_offset"`
}

// pagedRunsResult wraps a header-aware run page as
// {"runs":[...],"total":N,"next_offset":M}.
type pagedRunsResult struct {
	Runs       []wa.Run `json:"runs"`
	Total      int      `json:"total"`
	NextOffset int      `json:"next_offset"`
}

// pagedLanesResult wraps a header-aware lane page as
// {"lanes":[...],"total":N,"next_offset":M}.
type pagedLanesResult struct {
	Lanes      []wa.Lane `json:"lanes"`
	Total      int       `json:"total"`
	NextOffset int       `json:"next_offset"`
}

// pagedIRODSPathsResult wraps a header-aware iRODS path page as
// {"irods_paths":[...],"total":N,"next_offset":M}.
type pagedIRODSPathsResult struct {
	IRODSPaths []wa.IRODSPath `json:"irods_paths"`
	Total      int            `json:"total"`
	NextOffset int            `json:"next_offset"`
}

// pagedLibrariesResult wraps a header-aware library page as
// {"libraries":[...],"total":N,"next_offset":M}.
type pagedLibrariesResult struct {
	Libraries  []wa.Library `json:"libraries"`
	Total      int          `json:"total"`
	NextOffset int          `json:"next_offset"`
}

// outputSchemaFor returns an MCP-ready output schema (a map[string]any of type
// "object") for the named OpenAPI component schema, sourced from
// wa.OpenAPIDocument(). The component schemas carry the per-field descriptions
// from the result types' doc: tags (which the SDK's own jsonschema reflection
// would drop), so the returned schema is pre-set on Tool.OutputSchema and used
// verbatim. Every $ref within the component is inlined so the schema is
// self-contained, as mcp.AddTool rejects unresolved $refs. An unknown component
// name is an error.
func outputSchemaFor(componentName string) (map[string]any, error) {
	schemas, err := componentSchemas()
	if err != nil {
		return nil, err
	}

	component, ok := schemas[componentName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mlwh: no OpenAPI component schema named %q", componentName)
	}

	resolved, ok := resolveRefs(component, schemas, map[string]bool{}).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mlwh: component schema %q did not resolve to an object", componentName)
	}

	return resolved, nil
}

// outputSchemaForSlice returns the object-typed output schema for a list tool
// that returns a slice of the named component, wrapping the element schema in a
// one-property object whose single property is an array of that schema (the
// shape the slice wrapper structs serialise to). The wrapper's top-level type
// is "object", as MCP requires.
func outputSchemaForSlice(propertyName, componentName string) (map[string]any, error) {
	itemSchema, err := outputSchemaFor(componentName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			propertyName: map[string]any{
				"type":  "array",
				"items": itemSchema,
			},
		},
		"required": []any{propertyName},
	}, nil
}

// outputSchemaForPagedSlice returns the object-typed output schema for a paged
// list tool: the semantic slice field plus the required pagination metadata
// sourced from upstream response headers.
func outputSchemaForPagedSlice(propertyName, componentName string) (map[string]any, error) {
	itemSchema, err := outputSchemaFor(componentName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			propertyName: map[string]any{
				"type":  "array",
				"items": itemSchema,
			},
			"total": map[string]any{
				"type":        "integer",
				"description": "total number of matching rows from X-Total-Count",
			},
			"next_offset": map[string]any{
				"type":        "integer",
				"description": "offset of the next page from X-Next-Offset, or -1 when absent or on the last page",
			},
		},
		"required": []any{propertyName, "total", "next_offset"},
	}, nil
}

// boundedPagination resolves a typed paged tool's effective limit and offset.
// It rejects over-large limits before HTTP, defaults omitted/non-positive limits
// to the bounded page size, and otherwise preserves the caller's offset.
func boundedPagination(limit, offset int) (int, int, error) {
	if limit > pagedMaxLimit {
		return 0, 0, fmt.Errorf("limit %d exceeds the maximum of %d (a larger limit is rejected, not clamped); request a smaller page", limit, pagedMaxLimit)
	}

	if limit <= 0 {
		limit = pagedDefaultLimit
	}

	if offset == 0 {
		offset = pagedDefaultOffset
	}

	return limit, offset, nil
}

// componentSchemas returns the components.schemas map from the freshly built
// OpenAPI document.
func componentSchemas() (map[string]any, error) {
	components, ok := wa.OpenAPIDocument()["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mlwh: OpenAPI document has no components object")
	}

	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mlwh: OpenAPI document has no components.schemas object")
	}

	return schemas, nil
}

// resolveRefs returns a deep copy of node with every "$ref" into
// components.schemas replaced by the (recursively resolved) target schema, so
// the result contains no $ref. visiting tracks the component names currently on
// the resolution stack; a $ref back to an ancestor (a cyclic schema) terminates
// at a bare object schema rather than recursing forever.
func resolveRefs(node any, schemas map[string]any, visiting map[string]bool) any {
	switch typed := node.(type) {
	case map[string]any:
		if name, ok := refComponentName(typed); ok {
			return resolveComponent(name, schemas, visiting)
		}

		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = resolveRefs(value, schemas, visiting)
		}

		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = resolveRefs(value, schemas, visiting)
		}

		return out
	default:
		return node
	}
}

// refComponentName reports whether node is a {"$ref": "#/components/schemas/X"}
// reference and, if so, returns the referenced component name X.
func refComponentName(node map[string]any) (string, bool) {
	ref, ok := node["$ref"].(string)
	if !ok {
		return "", false
	}

	name, found := strings.CutPrefix(ref, openAPISchemaRefPrefix)
	if !found {
		return "", false
	}

	return name, true
}

// resolveComponent resolves a single component reference by name, guarding
// against cycles via the visiting set.
func resolveComponent(name string, schemas map[string]any, visiting map[string]bool) any {
	if visiting[name] {
		return map[string]any{"type": "object"}
	}

	target, ok := schemas[name].(map[string]any)
	if !ok {
		// A dangling reference: degrade to a permissive object rather than
		// leaking an unresolved $ref that mcp.AddTool would reject.
		return map[string]any{"type": "object"}
	}

	visiting[name] = true
	resolved := resolveRefs(target, schemas, visiting)
	delete(visiting, name)

	return resolved
}

// findSamplesFieldCorrespondence is the curated map from a FindSamplesBy*
// Registry method to the clean field name the mlwh_find_samples tool exposes in
// its enum. The SET and ORDER of fields come from filtering the live Registry
// (findSamplesEntries); this table only supplies the human-facing names. A
// FindSamplesBy* method with no entry here is a programming error surfaced by
// findSamplesMethods.
var findSamplesFieldCorrespondence = map[string]string{
	"FindSamplesBySangerID":        "sanger_id",
	"FindSamplesByIDSampleLims":    "lims_id",
	"FindSamplesByAccessionNumber": "accession",
	"FindSamplesBySupplierName":    "supplier_name",
	"FindSamplesByLibraryType":     "library_type",
}

// findSamplesEntries returns the FindSamplesBy* Registry method names in
// Registry declaration order. It is the single source of the find_samples field
// set and order; the clean names are layered on by findSamplesFieldEnum.
func findSamplesEntries() []string {
	var methods []string
	for _, entry := range wa.Registry {
		if strings.HasPrefix(entry.Method, findSamplesMethodPrefix) {
			methods = append(methods, entry.Method)
		}
	}

	return methods
}

// findSamplesFieldEnum returns the clean field names for the mlwh_find_samples
// field enum, in Registry order (sanger_id, lims_id, accession, supplier_name,
// library_type). A FindSamplesBy* method missing from the curated
// correspondence is skipped here but caught by findSamplesMethods.
func findSamplesFieldEnum() []string {
	methods := findSamplesEntries()

	fields := make([]string, 0, len(methods))
	for _, method := range methods {
		if field, ok := findSamplesFieldCorrespondence[method]; ok {
			fields = append(fields, field)
		}
	}

	return fields
}

// findSamplesMethods returns the lookup from a clean find_samples field name to
// the FindSamplesBy* Registry method the handler dispatches to. It panics if a
// FindSamplesBy* Registry method has no curated clean name, so a newly added
// upstream finder cannot silently fall out of the enum.
func findSamplesMethods() map[string]string {
	methods := findSamplesEntries()

	lookup := make(map[string]string, len(methods))
	for _, method := range methods {
		field, ok := findSamplesFieldCorrespondence[method]
		if !ok {
			panic(fmt.Sprintf("mlwh: %s Registry method %q has no curated field name", findSamplesMethodPrefix, method))
		}

		lookup[field] = method
	}

	return lookup
}

// identifierKindEnum returns the identifier kind enum for the expand tools: the
// string values of wa.IdentifierKinds() in their stable order (15 values, first
// sample_uuid, last id_library_lims). It is sourced from code, never
// hand-maintained.
func identifierKindEnum() []string {
	kinds := wa.IdentifierKinds()

	enum := make([]string, len(kinds))
	for i, kind := range kinds {
		enum[i] = string(kind)
	}

	return enum
}
