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
	"errors"
	"fmt"
	"strings"

	wa "github.com/wtsi-hgi/wa/mlwh"
)

// multiSampleAmbiguityHint replaces the ErrAmbiguous hint for an mlwh_export
// call that already set multi_sample=true: an API 1.11.0+ server still reports
// ambiguity when the value is also a must-be-unique sample identifier, so
// suggesting multi_sample=true again would loop the agent.
const multiSampleAmbiguityHint = "this value also identifies a single sample by name, id, or accession, " +
	"so multi_sample cannot span it; pick one Sanger name from mlwh_search_samples term=<the value> and retry with it."

// errorHint pairs a wa/mlwh sentinel with the short actionable hint appended to
// its mapped tool error. The hints turn an opaque upstream failure into advice
// the calling agent can act on (disambiguate, retry later, or fix the input).
type errorHint struct {
	sentinel error
	hint     string
}

// mapErrorWithHints implements mapToolError over the given precedence-ordered
// sentinel hints.
func mapErrorWithHints(err error, mappings []errorHint) error {
	if err == nil {
		return nil
	}

	for _, mapping := range mappings {
		if !errors.Is(err, mapping.sentinel) {
			continue
		}

		base := mapping.sentinel.Error()
		if context := upstreamContext(err.Error(), mappings); context != "" {
			return fmt.Errorf("%s: %s (%s)", context, base, mapping.hint)
		}

		return fmt.Errorf("%s (%s)", base, mapping.hint)
	}

	return err
}

// multiSampleHints returns sentinelHints with the ErrAmbiguous hint replaced by
// multiSampleAmbiguityHint, keeping the mapping precedence unchanged.
func multiSampleHints() []errorHint {
	hints := sentinelHints()
	for i := range hints {
		if hints[i].sentinel == wa.ErrAmbiguous {
			hints[i].hint = multiSampleAmbiguityHint
		}
	}

	return hints
}

// sentinelHints lists the wa/mlwh sentinels in mapping-precedence order. Order
// matters: ErrCacheNeverSynced is checked BEFORE ErrNotFound because the slice
// endpoints report a never-synced cache as errors.Join(ErrCacheNeverSynced,
// ErrNotFound), and a never-synced cache (retry later) is a different remedy
// from a genuinely absent identifier (fix the input). ErrMultiSampleUnsupported
// is checked BEFORE ErrAmbiguous because wa's MultiSampleUnsupportedError
// unwraps to both, and an old server needs the retry-without-multi_sample
// remedy rather than the advice to set multi_sample. ErrUpstreamImpaired
// covers both the 502 (impaired cache) and the 400 (bad request) cases, whose
// preserved upstream message (e.g. "term too short") carries the detail.
func sentinelHints() []errorHint {
	return []errorHint{
		{
			sentinel: wa.ErrCacheNeverSynced,
			hint:     "the warehouse cache has never been synced yet, so no data is available; retry once a sync has run.",
		},
		{
			sentinel: wa.ErrNotFound,
			hint:     "the identifier was not found; check the value and the kind of identifier you supplied.",
		},
		{
			sentinel: wa.ErrMultiSampleUnsupported,
			hint: "retry without multi_sample, once per sample, using each Sanger sample name; " +
				"list the samples with mlwh_search_samples term=<the name> and keep rows whose " +
				"supplier_name or donor_id equals it.",
		},
		{
			sentinel: wa.ErrAmbiguous,
			hint: "the identifier matches multiple records; disambiguate it (e.g. use a more specific identifier or kind). " +
				"A sample supplier name or donor id can span several samples: list them with " +
				"mlwh_search_samples term=<the name> and keep rows whose supplier_name or donor_id " +
				"equals it, or get rows for all of them with mlwh_export parent_kind=sample multi_sample=true.",
		},
		{
			sentinel: wa.ErrUnsupportedIdentifier,
			hint:     "the identifier form is not supported for this query; use a supported identifier kind.",
		},
		{
			sentinel: wa.ErrUpstreamImpaired,
			hint:     "the request could not be served (bad request or impaired upstream); fix the input if the upstream message reports a bad request, otherwise retry later after cache or upstream recovery.",
		},
	}
}

// upstreamContext extracts any contextual text the upstream error wrapped around
// its sentinels, by removing every known sentinel message (so a joined companion
// sentinel does not leak through) with its following ": " separator, so a
// sentinel in the middle leaves no doubled separator, then trimming the residual
// separators. It returns "" when nothing but sentinel text remains.
func upstreamContext(message string, mappings []errorHint) string {
	for _, mapping := range mappings {
		sentinel := mapping.sentinel.Error()
		message = strings.ReplaceAll(message, sentinel+": ", "")
		message = strings.ReplaceAll(message, sentinel, "")
	}

	return strings.Trim(message, " :\n\t")
}

// mapExportError maps an mlwh_export error like mapToolError, except that a
// call made with multi_sample=true gets multiSampleAmbiguityHint for an
// ambiguous identifier instead of advice to set multi_sample.
func mapExportError(err error, multiSample bool) error {
	if multiSample {
		return mapErrorWithHints(err, multiSampleHints())
	}

	return mapToolError(err)
}

// mapToolError converts a wa/mlwh client error into the error a typed tool
// handler returns (nil for no error). It prefers errors.Is against the wa/mlwh
// sentinels over parsing HTTP status, preserves the upstream message, and
// appends a short actionable hint per sentinel so the SDK can pack a clear
// IsError result. An unrecognised error is returned with its message intact.
//
// The matched sentinel's own message forms the base, so a slice endpoint's
// errors.Join(ErrCacheNeverSynced, ErrNotFound) reports the cache-not-synced
// remedy and never leaks the misleading "not found" half of the join. Any
// genuine contextual text the upstream wrapped around the sentinel (e.g. a 400
// "term too short" carried as ErrUpstreamImpaired) is preserved as a prefix.
func mapToolError(err error) error {
	return mapErrorWithHints(err, sentinelHints())
}
