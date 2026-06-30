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
	"bytes"
	"encoding/json"
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSliceWrappers(t *testing.T) {
	Convey("slice wrapper structs serialise to a one-property object per element type", t, func() {
		Convey("F2: samplesResult marshals to {\"samples\":[...]}", func() {
			b, err := json.Marshal(samplesResult{Samples: []wa.Sample{{Name: "s1"}, {Name: "s2"}}})
			So(err, ShouldBeNil)

			var obj map[string]any
			So(json.Unmarshal(b, &obj), ShouldBeNil)

			arr, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(arr), ShouldEqual, 2)
		})

		Convey("every wrapper exposes its slice under the agreed JSON field name", func() {
			cases := []struct {
				field string
				value any
			}{
				{"samples", samplesResult{Samples: []wa.Sample{{}}}},
				{"studies", studiesResult{Studies: []wa.Study{{}}}},
				{"runs", runsResult{Runs: []wa.Run{{}}}},
				{"lanes", lanesResult{Lanes: []wa.Lane{{}}}},
				{"irods_paths", irodsPathsResult{IRODSPaths: []wa.IRODSPath{{}}}},
				{"libraries", librariesResult{Libraries: []wa.Library{{}}}},
				{"tagged_ids", taggedIDsResult{TaggedIDs: []wa.TaggedID{{}}}},
				{"values", valuesResult{Values: []string{"v"}}},
			}

			missing := 0
			for _, c := range cases {
				b, err := json.Marshal(c.value)
				if err != nil {
					missing++

					continue
				}

				var obj map[string]any
				if json.Unmarshal(b, &obj) != nil {
					missing++

					continue
				}
				if _, ok := obj[c.field]; !ok {
					missing++
				}
			}

			So(missing, ShouldEqual, 0)
		})
	})
}

func TestFindSamplesFields(t *testing.T) {
	Convey("the find_samples field table derives from the FindSamplesBy Registry prefix", t, func() {
		Convey("B2.4 (foundation): the ordered enum is the 5 clean field names in Registry order", func() {
			So(findSamplesFieldEnum(), ShouldResemble, []string{
				"sanger_id", "lims_id", "accession", "supplier_name", "library_type",
			})
		})

		Convey("each clean field name maps to its FindSamplesBy* Registry method", func() {
			lookup := findSamplesMethods()
			So(lookup["sanger_id"], ShouldEqual, "FindSamplesBySangerID")
			So(lookup["lims_id"], ShouldEqual, "FindSamplesByIDSampleLims")
			So(lookup["accession"], ShouldEqual, "FindSamplesByAccessionNumber")
			So(lookup["supplier_name"], ShouldEqual, "FindSamplesBySupplierName")
			So(lookup["library_type"], ShouldEqual, "FindSamplesByLibraryType")
		})

		Convey("the enum order matches the FindSamplesBy* order in the live Registry", func() {
			var registryOrder []string
			for _, entry := range wa.Registry {
				if len(entry.Method) >= len("FindSamplesBy") && entry.Method[:len("FindSamplesBy")] == "FindSamplesBy" {
					registryOrder = append(registryOrder, entry.Method)
				}
			}

			enum := findSamplesFieldEnum()
			lookup := findSamplesMethods()
			So(len(enum), ShouldEqual, len(registryOrder))

			mapped := make([]string, len(enum))
			for i, field := range enum {
				mapped[i] = lookup[field]
			}
			So(mapped, ShouldResemble, registryOrder)
		})
	})
}

func TestIdentifierKindEnum(t *testing.T) {
	Convey("the kind enum is wa.IdentifierKinds() mapped to strings, in order", t, func() {
		Convey("B3.3 (foundation): 15 values, first sample_uuid, last id_library_lims", func() {
			enum := identifierKindEnum()
			So(len(enum), ShouldEqual, 15)
			So(enum[0], ShouldEqual, "sample_uuid")
			So(enum[len(enum)-1], ShouldEqual, "id_library_lims")
		})

		Convey("the enum equals the live IdentifierKinds() string values in the same order", func() {
			want := make([]string, len(wa.IdentifierKinds()))
			for i, k := range wa.IdentifierKinds() {
				want[i] = string(k)
			}
			So(identifierKindEnum(), ShouldResemble, want)
		})
	})
}

func TestOutputSchemaFor(t *testing.T) {
	Convey("outputSchemaFor sources MCP output schemas from wa.OpenAPIDocument()", t, func() {
		Convey("F1.1: the Sample schema preserves the supplier_name doc-tag description", func() {
			schema, err := outputSchemaFor("Sample")
			So(err, ShouldBeNil)
			So(schema["type"], ShouldEqual, "object")

			props, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)

			supplier, ok := props["supplier_name"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(supplier["description"], ShouldEqual, "name the sample supplier gave the sample")
		})

		Convey("F1.2 (schema source): the Match schema preserves the kind doc-tag description", func() {
			schema, err := outputSchemaFor("Match")
			So(err, ShouldBeNil)
			So(schema["type"], ShouldEqual, "object")

			props, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)

			kind, ok := props["kind"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(kind["description"], ShouldEqual, "kind of the resolved identifier")
		})

		Convey("F1.3: the resolved Sample schema marshals to a valid JSON object with no unresolved $ref", func() {
			schema, err := outputSchemaFor("Sample")
			So(err, ShouldBeNil)

			b, err := json.Marshal(schema)
			So(err, ShouldBeNil)

			var roundTrip map[string]any
			So(json.Unmarshal(b, &roundTrip), ShouldBeNil)
			So(bytes.Contains(b, []byte(`"$ref"`)), ShouldBeFalse)
		})

		Convey("nested object references are inlined, not left as $ref", func() {
			// Sample.libraries is an array of Library; Sample.studies an array of
			// Study. Both arrive as $ref in the component schema and must resolve.
			schema, err := outputSchemaFor("Sample")
			So(err, ShouldBeNil)
			So(containsRef(schema), ShouldBeFalse)

			props := schema["properties"].(map[string]any)
			libraries := props["libraries"].(map[string]any)
			So(libraries["type"], ShouldEqual, "array")

			items, ok := libraries["items"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(items["type"], ShouldEqual, "object")
			So(containsRef(items), ShouldBeFalse)
		})

		Convey("Match's description-wrapped references resolve while keeping the sibling description", func() {
			// Match.sample is {description, allOf:[{$ref Sample}]}; resolving must
			// drop the $ref but keep the human description.
			schema, err := outputSchemaFor("Match")
			So(err, ShouldBeNil)
			So(containsRef(schema), ShouldBeFalse)

			props := schema["properties"].(map[string]any)
			sample, ok := props["sample"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(sample["description"], ShouldEqual, "matched sample, when the identifier resolves to one")
		})

		Convey("an unknown component name returns an error", func() {
			_, err := outputSchemaFor("NoSuchComponent")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestOutputSchemaForSlice(t *testing.T) {
	Convey("outputSchemaForSlice builds the object wrapper schema list tools need", t, func() {
		Convey("F2.2: the samples wrapper schema is an object with a samples array property", func() {
			schema, err := outputSchemaForSlice("samples", "Sample")
			So(err, ShouldBeNil)
			So(schema["type"], ShouldEqual, "object")

			props, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)

			samples, ok := props["samples"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(samples["type"], ShouldEqual, "array")

			items, ok := samples["items"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(items["type"], ShouldEqual, "object")

			// The wrapped element schema retains its doc-tag descriptions and has
			// no unresolved $ref.
			itemProps := items["properties"].(map[string]any)
			supplier := itemProps["supplier_name"].(map[string]any)
			So(supplier["description"], ShouldEqual, "name the sample supplier gave the sample")
			So(containsRef(schema), ShouldBeFalse)
		})
	})
}

func TestOutputSchemaForPagedSlice(t *testing.T) {
	Convey("outputSchemaForPagedSlice builds a semantic list wrapper with required page metadata", t, func() {
		schema, err := outputSchemaForPagedSlice("samples", "Sample")
		So(err, ShouldBeNil)
		So(schema["type"], ShouldEqual, "object")

		props, ok := schema["properties"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(props["samples"].(map[string]any)["type"], ShouldEqual, "array")
		So(props["total"].(map[string]any)["type"], ShouldEqual, "integer")
		So(props["next_offset"].(map[string]any)["type"], ShouldEqual, "integer")
		So(schema["required"], ShouldResemble, []any{"samples", "total", "next_offset"})
		So(containsRef(schema), ShouldBeFalse)
	})
}

// containsRef reports whether the marshaled JSON of v contains an unresolved
// "$ref" key, which mcp.AddTool would reject.
func containsRef(v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return true
	}

	return bytes.Contains(b, []byte(`"$ref"`))
}
