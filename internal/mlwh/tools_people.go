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

// programmeDiscoveryNote connects the exact-match membership tools to their
// upstream vocabulary. It is also present on the vocabulary tool itself so all
// programme-facing descriptions expose the same agent routing guidance.
const programmeDiscoveryNote = " Call mlwh_programmes to discover exact programme values."

const studyUserDirectionNote = " This study-to-users lookup differs from mlwh_studies_for_user: omitting role there " +
	"defaults to owner, manager, and data_access_contact membership; faculty_sponsor is a Study field, not a study_users role."

// registerPeopleTools adds E1's programme tools, E2's inverse study-user
// tools, and D2's faculty_sponsor, user-study, and resolve-person tools.
// Descriptions are derived from the upstream Registry, so routing guidance
// stays aligned with MLWH.
func (p *provider) registerPeopleTools(r core.Registrar) error {
	if err := p.registerProgrammeTools(r); err != nil {
		return err
	}
	if err := p.registerStudyUserTools(r); err != nil {
		return err
	}

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

func (p *provider) registerStudyUserTools(r core.Registrar) error {
	usersSchema, err := outputSchemaForPagedSlice("users", "StudyUser")
	if err != nil {
		return fmt.Errorf("mlwh: build study users output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build study users count output schema: %w", err)
	}

	if err := p.addStudyUsers(r, usersSchema); err != nil {
		return err
	}

	return p.addCountStudyUsers(r, countSchema)
}

// studyUsersPageInput is the input for mlwh_study_users: a study identifier,
// optional raw role set, and optional bounded pagination.
type studyUsersPageInput struct {
	StudyLimsID string `json:"study_lims_id"`
	Role        string `json:"role,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

// pagedStudyUsersResult wraps an upstream StudyUser page under its semantic
// users property with exact header-derived pagination metadata.
type pagedStudyUsersResult struct {
	Users      []wa.StudyUser `json:"users"`
	Total      int            `json:"total"`
	NextOffset int            `json:"next_offset"`
}

func (p *provider) addStudyUsers(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudyUsers")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_study_users",
		Description:  description + pagedFanOutPaginationNote + bareListFreshnessNote + studyUserDirectionNote,
		InputSchema:  studyUsersInputSchema(true),
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyUsersPageInput) (*mcp.CallToolResult, pagedStudyUsersResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedStudyUsersResult](err)
		}

		page, err := client.StudyUsersPage(ctx, in.StudyLimsID, in.Role, limit, offset)
		if err != nil {
			return core.ToolError[pagedStudyUsersResult](mapToolError(err))
		}

		return nil, pagedStudyUsersResult{
			Users: page.Items, Total: page.Total, NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// studyUsersInput is the identifier and optional raw role set accepted by
// mlwh_count_study_users.
type studyUsersInput struct {
	StudyLimsID string `json:"study_lims_id"`
	Role        string `json:"role,omitempty"`
}

func (p *provider) addCountStudyUsers(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountStudyUsers")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_study_users",
		Description:  description + countFreshnessNote + studyUserDirectionNote,
		InputSchema:  studyUsersInputSchema(false),
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyUsersInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudyUsers(ctx, in.StudyLimsID, in.Role)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

func (p *provider) registerProgrammeTools(r core.Registrar) error {
	studiesSchema, err := outputSchemaForPagedSlice("studies", "Study")
	if err != nil {
		return fmt.Errorf("mlwh: build programme studies output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build programme count output schema: %w", err)
	}

	programmesSchema, err := outputSchemaForSlice("programmes", "Programme")
	if err != nil {
		return fmt.Errorf("mlwh: build programmes output schema: %w", err)
	}

	if err := p.addStudiesForProgramme(r, studiesSchema); err != nil {
		return err
	}
	if err := p.addCountStudiesForProgramme(r, countSchema); err != nil {
		return err
	}

	return p.addProgrammes(r, programmesSchema)
}

// programmePageInput is the input for mlwh_studies_for_programme: an exact
// programme value plus optional bounded pagination.
type programmePageInput struct {
	Programme string `json:"programme" jsonschema:"exact programme value; call mlwh_programmes to discover valid values"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addStudiesForProgramme(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudiesForProgramme")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_studies_for_programme",
		Description:  description + pagedFanOutPaginationNote + bareListFreshnessNote + programmeDiscoveryNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in programmePageInput) (*mcp.CallToolResult, pagedStudiesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedStudiesResult](err)
		}

		page, err := client.StudiesForProgrammePage(ctx, in.Programme, limit, offset)
		if err != nil {
			return core.ToolError[pagedStudiesResult](mapToolError(err))
		}

		return nil, pagedStudiesResult{
			Studies:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// programmeInput is the exact programme value accepted by
// mlwh_count_studies_for_programme.
type programmeInput struct {
	Programme string `json:"programme" jsonschema:"exact programme value; call mlwh_programmes to discover valid values"`
}

func (p *provider) addCountStudiesForProgramme(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountStudiesForProgramme")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies_for_programme",
		Description:  description + countFreshnessNote + programmeDiscoveryNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in programmeInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudiesForProgramme(ctx, in.Programme)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// programmesInput keeps mlwh_programmes' public input as an empty object.
type programmesInput struct{}

// programmesResult wraps the exact upstream vocabulary rows under the semantic
// programmes property required by MCP object results.
type programmesResult struct {
	Programmes []wa.Programme `json:"programmes"`
}

func (p *provider) addProgrammes(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("Programmes")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_programmes",
		Description:  description + bareListFreshnessNote + programmeDiscoveryNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ programmesInput) (*mcp.CallToolResult, programmesResult, error) {
		programmes, err := client.Programmes(ctx)
		if err != nil {
			return core.ToolError[programmesResult](mapToolError(err))
		}

		return nil, programmesResult{Programmes: programmes}, nil
	})

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
