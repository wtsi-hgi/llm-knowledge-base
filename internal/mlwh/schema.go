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
	latestDataDefaultLimit = 10
	pagedDefaultLimit      = 100
	pagedDefaultOffset     = 0
	pagedMaxLimit          = 1000
)

// exportInputSchema describes the generic relationship export input. Its three
// vocabulary-bearing properties are generated from wa's runtime sources of
// truth so aliases, parent kinds, defaults, and selectable columns move with
// the upstream API instead of being copied into this provider.
func exportInputSchema() map[string]any {
	childrenEnum, parentEnum, childrenDescription, parentDescription := exportRelationshipSchema()
	columnEnum, columnsDescription := exportColumnSchema()

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"children": map[string]any{
				"type": "string", "description": childrenDescription, "enum": childrenEnum,
			},
			"parent_kind": map[string]any{
				"type": "string", "description": parentDescription, "enum": parentEnum,
			},
			"parent_id": map[string]any{
				"type": "string", "description": "identifier of the selected parent; resolution is performed upstream",
			},
			"columns": map[string]any{
				"type": "array", "description": columnsDescription,
				"items": map[string]any{"type": "string", "enum": columnEnum},
			},
			"file_type": map[string]any{
				"type": "string", "description": "optional file suffix for file exports or product iRODS attachments",
			},
			"deliverables_only": map[string]any{
				"type": "boolean", "description": "optional tri-state deliverability filter; omit to use the relationship default",
			},
			"role": map[string]any{
				"type": "string", "description": "optional comma-separated study_users role filter",
			},
			"qc": map[string]any{
				"type": "string", "description": "optional product-backed QC filter", "enum": []any{"pass", "fail", "pending"},
			},
			"library_type": map[string]any{
				"type": "string", "description": "optional exact library type filter for sample-backed exports",
			},
			"organism": map[string]any{
				"type": "string", "description": "optional organism or common-name filter for sample-backed exports",
			},
			"sort": map[string]any{
				"type": "string", "description": "optional iRODS created-time sort mode", "enum": []any{"created-desc", "created_desc"},
			},
			"since": map[string]any{
				"type": "string", "description": "optional RFC3339 inclusive lower bound for iRODS created time",
			},
			"until": map[string]any{
				"type": "string", "description": "optional RFC3339 exclusive upper bound for iRODS created time; requires since",
			},
			"limit": map[string]any{
				"type": "integer", "description": "optional bounded page size; upstream defaults to 1000",
			},
			"offset": map[string]any{
				"type": "integer", "description": "optional offset for offset-backed relationships",
			},
			"all": map[string]any{
				"type": "boolean", "description": "return the complete matching set through the upstream iterator",
			},
			"cursor": map[string]any{
				"type": "string", "description": "opaque continuation cursor returned by iRODS or products exports",
			},
			"format": map[string]any{
				"type": "string", "description": "output format metadata; defaults upstream to tsv", "enum": []any{"tsv", "csv", "json"},
			},
		},
		"required": []any{"children", "parent_kind", "parent_id"},
	}
}

func exportRelationshipSchema() (childrenEnum, parentEnum []any, childrenDescription, parentDescription string) {
	relationships := wa.ExportRelationshipDescriptions()
	seenParents := map[string]bool{}
	childrenParts := make([]string, len(relationships))
	parentParts := make([]string, len(relationships))

	for index, relationship := range relationships {
		label := exportVocabularyLabel(relationship.Children, relationship.Aliases)
		childrenParts[index] = label + " (" + relationship.Description + ")"
		parentParts[index] = label + ": " + strings.Join(relationship.ParentKinds, ",")
		childrenEnum = append(childrenEnum, relationship.Children)
		for _, alias := range relationship.Aliases {
			childrenEnum = append(childrenEnum, alias)
		}
		for _, parent := range relationship.ParentKinds {
			if !seenParents[parent] {
				parentEnum = append(parentEnum, parent)
				seenParents[parent] = true
			}
		}
	}

	childrenDescription = "child relationship to export. Supported children and aliases: " + strings.Join(childrenParts, "; ")
	parentDescription = "kind of parent identified by parent_id. Allowed parent kinds by child: " + strings.Join(parentParts, "; ")

	return childrenEnum, parentEnum, childrenDescription, parentDescription
}

func exportColumnSchema() ([]any, string) {
	vocabularies := wa.ExportColumnVocabularies()
	seen := map[string]bool{}
	enum := []any{}
	descriptionParts := make([]string, len(vocabularies))

	for index, vocabulary := range vocabularies {
		columns := make([]string, len(vocabulary.Columns))
		for columnIndex, column := range vocabulary.Columns {
			columns[columnIndex] = exportColumnLabel(column)
			for _, value := range append([]string{column.Name}, column.Aliases...) {
				if !seen[value] {
					enum = append(enum, value)
					seen[value] = true
				}
			}
		}

		descriptionParts[index] = exportVocabularyLabel(vocabulary.Children, vocabulary.Aliases) +
			" default " + strings.Join(vocabulary.Default, ",") +
			"; available " + strings.Join(columns, ",")
	}

	description := "ordered columns to return; omit to use the relationship default. Columns by child: " +
		strings.Join(descriptionParts, "; ")

	return enum, description
}

func exportVocabularyLabel(children string, aliases []string) string {
	if len(aliases) == 0 {
		return children
	}

	return children + "/" + strings.Join(aliases, "/")
}

func exportColumnLabel(column wa.ExportColumnDescription) string {
	if len(column.Aliases) == 0 {
		return column.Name
	}

	return column.Name + " (aliases: " + strings.Join(column.Aliases, ",") + ")"
}

// callEndpointInputSchema describes the generic call tool input and advertises
// every Registry Method as the method enum. The enum is rebuilt from the live
// Registry when the provider registers, so adding an upstream endpoint makes
// it discoverable without adding another curated tool or maintaining a second
// method list.
func callEndpointInputSchema() map[string]any {
	methods := make([]any, len(wa.Registry))
	for i, entry := range wa.Registry {
		methods[i] = entry.Method
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"description": "the Registry Method name to dispatch (see mlwh://workflow for its endpoint)",
				"enum":        methods,
			},
			"path_params": map[string]any{
				"type":        "array",
				"description": "the endpoint's path parameters, in Registry declaration order",
				"items":       map[string]any{"type": "string"},
			},
			"query_params": map[string]any{
				"type":                 "object",
				"description":          "the endpoint's query parameters, including pagination controls",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required": []any{"method"},
	}
}

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

// pagedSamplesWithDataResult wraps a header-aware sample availability page as
// {"samples":[...],"total":N,"next_offset":M}. Each row carries the sample and
// its platform qualifiers.
type pagedSamplesWithDataResult struct {
	Samples    []wa.SampleWithData `json:"samples"`
	Total      int                 `json:"total"`
	NextOffset int                 `json:"next_offset"`
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

// pagedLatestDataResult wraps a header-aware latest-data page as
// {"latest_data":[...],"total":N,"next_offset":M}.
type pagedLatestDataResult struct {
	LatestData []wa.RecentDataRow `json:"latest_data"`
	Total      int                `json:"total"`
	NextOffset int                `json:"next_offset"`
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
	if componentName == "IRODSPath" {
		allowNullProperty(resolved, "deliverable")
	}

	return resolved, nil
}

// allowNullProperty amends an OpenAPI-derived property for a Go pointer field.
// The upstream IRODSPath schema currently describes *bool Deliverable as only a
// boolean even though its exact JSON contract is tri-state true/false/null.
func allowNullProperty(schema map[string]any, propertyName string) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		return
	}

	property["type"] = []any{"boolean", "null"}
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

// outputSchemaForPagedObject returns the OpenAPI-sourced object schema for an
// upstream envelope with the required pagination metadata added at the top
// level. It is used for paged typed tools whose semantic response is already an
// object, so the MCP result stays flattened instead of wrapping the object
// under another property.
func outputSchemaForPagedObject(componentName string) (map[string]any, error) {
	schema, err := outputSchemaFor(componentName)
	if err != nil {
		return nil, err
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mlwh: component schema %q has no properties object", componentName)
	}

	properties["total"] = map[string]any{
		"type":        "integer",
		"description": "total number of matching rows from X-Total-Count",
	}
	properties["next_offset"] = map[string]any{
		"type":        "integer",
		"description": "offset of the next page from X-Next-Offset, or -1 when absent or on the last page",
	}

	required, _ := schema["required"].([]any)
	schema["required"] = append(required, "total", "next_offset")

	return schema, nil
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
