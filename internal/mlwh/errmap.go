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

// errorHint pairs a wa/mlwh sentinel with the short actionable hint appended to
// its mapped tool error. The hints turn an opaque upstream failure into advice
// the calling agent can act on (disambiguate, retry later, or fix the input).
type errorHint struct {
	sentinel error
	hint     string
}

// sentinelHints lists the wa/mlwh sentinels in mapping-precedence order. Order
// matters: ErrCacheNeverSynced is checked BEFORE ErrNotFound because the slice
// endpoints report a never-synced cache as errors.Join(ErrCacheNeverSynced,
// ErrNotFound), and a never-synced cache (retry later) is a different remedy
// from a genuinely absent identifier (fix the input). ErrUpstreamImpaired
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
			sentinel: wa.ErrAmbiguous,
			hint:     "the identifier matches multiple records; disambiguate it (e.g. use a more specific identifier or kind).",
		},
		{
			sentinel: wa.ErrUnsupportedIdentifier,
			hint:     "the identifier form is not supported for this query; use a supported identifier kind.",
		},
		{
			sentinel: wa.ErrUpstreamImpaired,
			hint:     "the request could not be served (bad request or impaired upstream); fix the input or retry later.",
		},
	}
}

// upstreamContext extracts any contextual text the upstream error wrapped around
// its sentinels, by removing every known sentinel message (so a joined companion
// sentinel does not leak through) and trimming the residual separators. It
// returns "" when nothing but sentinel text remains.
func upstreamContext(message string, mappings []errorHint) string {
	for _, mapping := range mappings {
		message = strings.ReplaceAll(message, mapping.sentinel.Error(), "")
	}

	return strings.Trim(message, " :\n\t")
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
	if err == nil {
		return nil
	}

	mappings := sentinelHints()
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
