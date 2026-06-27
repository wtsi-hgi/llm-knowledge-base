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
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMapToolError(t *testing.T) {
	Convey("mapToolError turns wa/mlwh sentinels into clear tool errors", t, func() {
		Convey("J1.1: a joined never-synced+not-found maps to cache-not-synced, not not-found", func() {
			err := mapToolError(errors.Join(wa.ErrCacheNeverSynced, wa.ErrNotFound))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "synced")
			So(msg, ShouldNotContainSubstring, "not found")
		})

		Convey("J1.2: a not-found error mentions the identifier was not found", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrNotFound))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "not found")
		})

		Convey("J1.3: an ambiguous error mentions multiple records and disambiguating", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrAmbiguous))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "multiple")
			So(msg, ShouldContainSubstring, "disambiguat")
		})

		Convey("J1.4: an unsupported-identifier error mentions the form is unsupported", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUnsupportedIdentifier))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "identifier")
			So(msg, ShouldContainSubstring, "not supported")
		})

		Convey("J1.5: a 400 carried as ErrUpstreamImpaired preserves its upstream message", func() {
			err := mapToolError(fmt.Errorf("term too short: %w", wa.ErrUpstreamImpaired))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "term too short")
		})

		Convey("a bare ErrUpstreamImpaired (502) still maps to a non-nil actionable error", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUpstreamImpaired))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, wa.ErrUpstreamImpaired.Error())
		})

		Convey("J1.6: a nil error maps to nil", func() {
			So(mapToolError(nil), ShouldBeNil)
		})

		Convey("an unrecognised (non-sentinel) error is returned with its message preserved", func() {
			err := mapToolError(errors.New("some random failure"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "some random failure")
		})
	})
}
