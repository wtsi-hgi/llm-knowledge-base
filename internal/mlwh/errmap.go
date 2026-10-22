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
	"regexp"
	"strconv"
	"strings"

	wa "github.com/wtsi-hgi/wa/mlwh"
)

// multiSampleAmbiguityHint replaces the ErrAmbiguous hint for an mlwh_export
// call that already set multi_sample=true: an API 1.11.0+ server still reports
// ambiguity when the value is also a must-be-unique sample identifier, so
// suggesting multi_sample=true again would loop the agent.
const multiSampleAmbiguityHint = "this value also identifies a single sample by name, id, or accession, " +
	"so multi_sample cannot span it; pick one of the Sanger samples the message names, or one Sanger name " +
	"from mlwh_search_samples term=<the value>, and retry with it."

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

		base := sentinelBase(err, mapping.sentinel)
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
// remedy rather than the advice to set multi_sample. ErrBadRequest is checked
// BEFORE ErrRequestRejected because wa's RemoteRequestError unwraps to both for
// a 400, and a bad request has the more specific fix-the-input remedy. A
// rejected request (any other unrecognised 4xx) is the caller's to fix, while
// ErrUpstreamImpaired now only covers a server that failed to serve an accepted
// request, so its remedy is to retry later.
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
			hint: "the identifier matches multiple records; disambiguate it. The message names candidate " +
				"Sanger samples (LIMS ids for studies) you can use directly; otherwise use a more specific " +
				"identifier or kind. A sample supplier name or donor id can span several samples: list every " +
				"sample sharing a supplier name with mlwh_find_samples field=supplier_name value=<the name>, " +
				"or search with mlwh_search_samples term=<the name> and keep rows whose supplier_name or " +
				"donor_id equals it, or get rows for all of them with mlwh_export parent_kind=sample multi_sample=true.",
		},
		{
			sentinel: wa.ErrUnsupportedIdentifier,
			hint:     "the identifier form is not supported for this query; use a supported identifier kind.",
		},
		{
			sentinel: wa.ErrBadRequest,
			hint:     "the server rejected the input as invalid; fix the input as the message describes, then retry.",
		},
		{
			sentinel: wa.ErrRequestRejected,
			hint:     "the server rejected the request; fix the request as the status and message describe rather than retrying it unchanged.",
		},
		{
			sentinel: wa.ErrUpstreamImpaired,
			hint:     "the warehouse or its cache could not serve the request; retry later, after cache or upstream recovery.",
		},
	}
}

// requestRejectedDetail matches the "(<status> <code>)" detail wa's
// RemoteRequestError adds after the ErrRequestRejected text.
var requestRejectedDetail = regexp.MustCompile(regexp.QuoteMeta(wa.ErrRequestRejected.Error()) + ` \([^)]*\)`)

// upstreamContext extracts any contextual text the upstream error wrapped around
// its sentinels, by removing every known sentinel message (so a joined companion
// sentinel does not leak through) with its following ": " separator, so a
// sentinel in the middle leaves no doubled separator, then trimming the residual
// separators. A rejected request's "(<status> <code>)" detail goes with its
// sentinel text, as mapErrorWithHints restates it in the base. It returns ""
// when nothing but sentinel text remains.
func upstreamContext(message string, mappings []errorHint) string {
	message = requestRejectedDetail.ReplaceAllString(message, wa.ErrRequestRejected.Error())

	for _, mapping := range mappings {
		sentinel := mapping.sentinel.Error()
		message = strings.ReplaceAll(message, sentinel+": ", "")
		message = strings.ReplaceAll(message, sentinel, "")
	}

	return strings.Trim(message, " :\n\t")
}

// sentinelBase returns the matched sentinel's message as the mapped error's
// base. For a rejected request it adds the upstream status and code, e.g.
// "mlwh: request rejected (403 forbidden)", since they tell the caller why.
func sentinelBase(err, sentinel error) string {
	base := sentinel.Error()

	var rejected *wa.RemoteRequestError
	if sentinel != wa.ErrRequestRejected || !errors.As(err, &rejected) {
		return base
	}

	detail := strconv.Itoa(rejected.StatusCode)
	if rejected.Code != "" {
		detail += " " + rejected.Code
	}

	return base + " (" + detail + ")"
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
// "term too short" carried as ErrBadRequest) is preserved as a prefix.
func mapToolError(err error) error {
	return mapErrorWithHints(err, sentinelHints())
}
