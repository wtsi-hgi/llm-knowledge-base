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
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

const exportContinuationGuidance = "For bounded products and unsorted iRODS pages, continue with the exact opaque NextCursor. " +
	"Created-desc iRODS and all other relationships are offset-backed: add the number of returned Rows to the request offset " +
	"(for example, offset 100 plus 40 returned Rows means offset 140). Do not use a cursor for offset-backed relationships."

// materializeExportRows converts wa's all=true iterator into the structured
// matrix required by MCP. Rows are cloned because iterator implementations may
// reuse their backing storage. Any cancellation or iteration error returns a
// zero result so callers never receive a successful partial matrix.
func materializeExportRows(ctx context.Context, result wa.ExportResult) (wa.ExportResult, error) {
	if err := ctx.Err(); err != nil {
		return wa.ExportResult{}, err
	}

	rows := make([][]string, 0)
	_, err := result.ForEachRow(ctx, func(row []string) error {
		rows = append(rows, slices.Clone(row))

		return nil
	})
	if err != nil {
		return wa.ExportResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return wa.ExportResult{}, err
	}

	result.Rows = rows

	return result, nil
}

// exportInput is the exact public input for mlwh_export. The pointer boolean
// preserves omission separately from explicit false and true.
type exportInput struct {
	Children         string   `json:"children"`
	ParentKind       string   `json:"parent_kind"`
	ParentID         string   `json:"parent_id"`
	Columns          []string `json:"columns,omitempty"`
	FileType         string   `json:"file_type,omitempty"`
	DeliverablesOnly *bool    `json:"deliverables_only,omitempty"`
	Role             string   `json:"role,omitempty"`
	QC               string   `json:"qc,omitempty"`
	LibraryType      string   `json:"library_type,omitempty"`
	Organism         string   `json:"organism,omitempty"`
	Sort             string   `json:"sort,omitempty"`
	Since            string   `json:"since,omitempty"`
	Until            string   `json:"until,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
	All              bool     `json:"all,omitempty"`
	Cursor           string   `json:"cursor,omitempty"`
	Format           string   `json:"format,omitempty"`
}

// registerExportTool adds the generic relationship projection tool. The
// adapter deliberately performs no relationship, option, or parent validation:
// RemoteClient.Export remains the authority for the complete export grammar.
func (p *provider) registerExportTool(r core.Registrar) error {
	description, err := resolveDescription("Export")
	if err != nil {
		return err
	}
	description += " " + exportContinuationGuidance

	outputSchema, err := outputSchemaFor("ExportResult")
	if err != nil {
		return fmt.Errorf("mlwh: build export output schema: %w", err)
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_export",
		Description:  description,
		InputSchema:  exportInputSchema(),
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in exportInput) (*mcp.CallToolResult, wa.ExportResult, error) {
		result, err := client.Export(ctx, wa.ExportRelationship{
			Children: in.Children, ParentKind: in.ParentKind,
		}, in.ParentID, wa.ExportOptions{
			Columns:          in.Columns,
			FileType:         in.FileType,
			DeliverablesOnly: in.DeliverablesOnly,
			Role:             in.Role,
			QC:               in.QC,
			LibraryType:      in.LibraryType,
			Organism:         in.Organism,
			Sort:             in.Sort,
			Since:            in.Since,
			Until:            in.Until,
			Limit:            in.Limit,
			Offset:           in.Offset,
			All:              in.All,
			Cursor:           in.Cursor,
			Format:           in.Format,
		})
		if err != nil {
			return core.ToolError[wa.ExportResult](mapToolError(err))
		}
		if in.All {
			result, err = materializeExportRows(ctx, result)
			if err != nil {
				return core.ToolError[wa.ExportResult](mapToolError(err))
			}
		}

		return nil, result, nil
	})

	return nil
}
