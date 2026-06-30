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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// registerPeopleTools adds D2's faculty_sponsor, study_users, and
// resolve-person tools. Descriptions are derived from the upstream Registry, so
// the sponsor-versus-role-membership routing guidance stays aligned with MLWH.
func (p *provider) registerPeopleTools(r core.Registrar) error {
	studiesSchema, err := outputSchemaForPagedSlice("studies", "PersonStudy")
	if err != nil {
		return fmt.Errorf("mlwh: build person studies output schema: %w", err)
	}

	peopleSchema, err := outputSchemaForPagedSlice("people", "PersonCandidate")
	if err != nil {
		return fmt.Errorf("mlwh: build people output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build count output schema: %w", err)
	}

	if err := p.addStudiesForFacultySponsor(r, studiesSchema); err != nil {
		return err
	}
	if err := p.addCountStudiesForFacultySponsor(r, countSchema); err != nil {
		return err
	}
	if err := p.addStudiesForUser(r, studiesSchema); err != nil {
		return err
	}
	if err := p.addCountStudiesForUser(r, countSchema); err != nil {
		return err
	}
	if err := p.addResolvePerson(r, peopleSchema); err != nil {
		return err
	}
	if err := p.addCountResolvePerson(r, countSchema); err != nil {
		return err
	}

	return nil
}

// pagedPersonStudiesResult wraps a header-aware PersonStudy page as
// {"studies":[...],"total":N,"next_offset":M}.
type pagedPersonStudiesResult struct {
	Studies    []wa.PersonStudy `json:"studies"`
	Total      int              `json:"total"`
	NextOffset int              `json:"next_offset"`
}

// facultySponsorInput is the input for mlwh_studies_for_faculty_sponsor: a
// faculty_sponsor substring plus optional bounded pagination.
type facultySponsorInput struct {
	Name   string `json:"name" jsonschema:"the faculty_sponsor name or substring to match"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addStudiesForFacultySponsor(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudiesForFacultySponsor")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_studies_for_faculty_sponsor",
		Description:  description + pagedFanOutPaginationNote + bareListFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in facultySponsorInput) (*mcp.CallToolResult, pagedPersonStudiesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedPersonStudiesResult](err)
		}

		page, err := client.StudiesForFacultySponsorPage(ctx, in.Name, limit, offset)
		if err != nil {
			return core.ToolError[pagedPersonStudiesResult](mapToolError(err))
		}

		return nil, pagedPersonStudiesResult{
			Studies:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// facultySponsorCountInput is the input for
// mlwh_count_studies_for_faculty_sponsor.
type facultySponsorCountInput struct {
	Name string `json:"name" jsonschema:"the faculty_sponsor name or substring to match"`
}

func (p *provider) addCountStudiesForFacultySponsor(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountStudiesForFacultySponsor")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies_for_faculty_sponsor",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in facultySponsorCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudiesForFacultySponsor(ctx, in.Name)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// userStudiesInput is the input for mlwh_studies_for_user: a study_users person
// term, an optional raw role override, and optional bounded pagination.
type userStudiesInput struct {
	Person string `json:"person" jsonschema:"the study_users person name, login, email, or substring to match"`
	Role   string `json:"role,omitempty" jsonschema:"optional raw role override; omit to use upstream defaults owner, manager, and data_access_contact"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addStudiesForUser(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudiesForUser")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_studies_for_user",
		Description:  description + pagedFanOutPaginationNote + bareListFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in userStudiesInput) (*mcp.CallToolResult, pagedPersonStudiesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedPersonStudiesResult](err)
		}

		page, err := client.StudiesForUserPage(ctx, in.Person, in.Role, limit, offset)
		if err != nil {
			return core.ToolError[pagedPersonStudiesResult](mapToolError(err))
		}

		return nil, pagedPersonStudiesResult{
			Studies:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// userCountInput is the input for mlwh_count_studies_for_user. Role is passed
// through exactly when non-empty; an omitted role remains omitted upstream.
type userCountInput struct {
	Person string `json:"person" jsonschema:"the study_users person name, login, email, or substring to match"`
	Role   string `json:"role,omitempty" jsonschema:"optional raw role override; omit to use upstream defaults owner, manager, and data_access_contact"`
}

func (p *provider) addCountStudiesForUser(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountStudiesForUser")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies_for_user",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in userCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudiesForUser(ctx, in.Person, in.Role)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// pagedPeopleResult wraps a header-aware PersonCandidate page as
// {"people":[...],"total":N,"next_offset":M}.
type pagedPeopleResult struct {
	People     []wa.PersonCandidate `json:"people"`
	Total      int                  `json:"total"`
	NextOffset int                  `json:"next_offset"`
}

// resolvePersonInput is the input for mlwh_resolve_person: a person term plus
// optional bounded pagination.
type resolvePersonInput struct {
	Term   string `json:"term" jsonschema:"the person term to resolve across faculty_sponsor and study_users"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addResolvePerson(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("ResolvePerson")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_resolve_person",
		Description:  description + pagedFanOutPaginationNote + bareListFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in resolvePersonInput) (*mcp.CallToolResult, pagedPeopleResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedPeopleResult](err)
		}

		page, err := client.ResolvePersonPage(ctx, in.Term, limit, offset)
		if err != nil {
			return core.ToolError[pagedPeopleResult](mapToolError(err))
		}

		return nil, pagedPeopleResult{
			People:     page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// personTermInput is the input for mlwh_count_resolve_person.
type personTermInput struct {
	Term string `json:"term" jsonschema:"the person term to resolve across faculty_sponsor and study_users"`
}

func (p *provider) addCountResolvePerson(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountResolvePerson")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_resolve_person",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in personTermInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountResolvePerson(ctx, in.Term)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}
