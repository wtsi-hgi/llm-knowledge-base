# Phase 6: Availability, IRODS, And Manifest

Ref: [spec.md](spec.md) sections C1, C2, C3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Batch 1 (parallel)

#### Item 6.1: C1 - Samples with and without sequencing data

Parallel with C2, C3.

spec.md section: C1

Register samples-with-data count and list tools plus
`mlwh_samples_without_data_for_study` in
`internal/mlwh/tools_availability.go`, using the `wa` availability
methods and RFC3339 windows, covering all 6 acceptance tests from C1.
Depends on phase 4.

- [ ] implemented
- [ ] reviewed

#### Item 6.2: C2 - IRODS path tools with file-type counts

Parallel with C1, C3.

spec.md section: C2

Update sample and study iRODS tools and add run iRODS plus count tools
in `internal/mlwh/tools_availability.go`, using file-type-aware paged
and count `wa` methods, covering all 9 acceptance tests from C2.
Depends on phase 4.

- [ ] implemented
- [ ] reviewed

#### Item 6.3: C3 - Study manifest and count [parallel with C1, C2]

spec.md section: C3

Register `mlwh_study_manifest` and `mlwh_count_study_manifest` in
`internal/mlwh/tools_availability.go`, call `StudyManifestPage` and
`CountStudyManifest`, and flatten manifest output, covering all 5
acceptance tests from C3. Depends on phase 4.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
