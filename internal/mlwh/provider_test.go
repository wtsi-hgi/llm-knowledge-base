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
	"flag"
	"strings"
	"testing"
	"time"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProviderNew(t *testing.T) {
	Convey("New builds an MLWH provider from a resolved RemoteConfig", t, func() {
		Convey("H2.2: a missing base URL is a clear startup error mentioning it is required", func() {
			provider, err := New(wa.RemoteConfig{})
			So(err, ShouldNotBeNil)
			So(provider, ShouldBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "base url")
			So(msg, ShouldContainSubstring, "required")
		})

		Convey("a valid base URL yields a non-nil core.Provider and no error", func() {
			provider, err := New(wa.RemoteConfig{BaseURL: "http://stub.example"})
			So(err, ShouldBeNil)
			So(provider, ShouldNotBeNil)

			Convey("I1.3: Name() returns \"mlwh\"", func() {
				So(provider.Name(), ShouldEqual, "mlwh")
			})

			Convey("APIVersion() returns the targeted wa.APIVersion", func() {
				So(provider.APIVersion(), ShouldEqual, wa.APIVersion)
			})

			Convey("Register wires the search/count tools through the Registrar", func() {
				stub := newStubMLWH(t)
				cs, cleanup := runMLWHServerWithClient(t, stub)
				defer cleanup()

				_, ok := toolByName(t, cs, "mlwh_search_samples")
				So(ok, ShouldBeTrue)
			})
		})
	})
}

func TestProviderConfig(t *testing.T) {
	Convey("Config.Resolve reads the MLWH provider settings from the environment", t, func() {
		Convey("H2.1: MLWH_BASE_URL from env populates RemoteConfig.BaseURL", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.BaseURL, ShouldEqual, "http://stub.example")
		})

		Convey("H2.3: MLWH_TIMEOUT=5s yields a 5s timeout and leaves CacheTTL zero", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_TIMEOUT", "5s")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.Timeout, ShouldEqual, 5*time.Second)
			So(cfg.CacheTTL, ShouldEqual, time.Duration(0))
		})

		Convey("the optional CA cert path flows through to RemoteConfig.CACert", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_CA_CERT", "/etc/ssl/mlwh-ca.pem")

			cfg, err := (Config{}).Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.CACert, ShouldEqual, "/etc/ssl/mlwh-ca.pem")
		})

		Convey("an unparseable MLWH_TIMEOUT is a clear error naming the timeout", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://stub.example")
			t.Setenv("MLWH_TIMEOUT", "soon")

			_, err := (Config{}).Resolve(nil)
			So(err, ShouldNotBeNil)
			So(strings.ToLower(err.Error()), ShouldContainSubstring, "timeout")
		})

		Convey("flag-sourced values take precedence over the environment", func() {
			clearMLWHEnv(t)
			t.Setenv("MLWH_BASE_URL", "http://from-env.example")

			cfg, err := Config{BaseURL: "http://from-flag.example"}.Resolve(nil)
			So(err, ShouldBeNil)
			So(cfg.BaseURL, ShouldEqual, "http://from-flag.example")
		})
	})
}

func TestProviderBindFlags(t *testing.T) {
	Convey("BindFlags registers the three --mlwh-* flags so cmd/mcp-server can wire them", t, func() {
		clearMLWHEnv(t)

		var cfg Config
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.BindFlags(fs)

		err := fs.Parse([]string{
			"--mlwh-base-url", "http://flagged.example",
			"--mlwh-ca-cert", "/tmp/ca.pem",
			"--mlwh-timeout", "9s",
		})
		So(err, ShouldBeNil)

		So(cfg.BaseURL, ShouldEqual, "http://flagged.example")
		So(cfg.CACert, ShouldEqual, "/tmp/ca.pem")
		So(cfg.Timeout, ShouldEqual, "9s")

		resolved, err := cfg.Resolve(nil)
		So(err, ShouldBeNil)
		So(resolved.BaseURL, ShouldEqual, "http://flagged.example")
		So(resolved.CACert, ShouldEqual, "/tmp/ca.pem")
		So(resolved.Timeout, ShouldEqual, 9*time.Second)
	})
}

// clearMLWHEnv blanks every MLWH_* setting for the duration of the test (via
// t.Setenv, which restores them afterwards), so a value already present in the
// developer's or CI environment cannot leak into a "nothing set" assertion.
func clearMLWHEnv(t *testing.T) {
	t.Helper()

	t.Setenv("MLWH_BASE_URL", "")
	t.Setenv("MLWH_CA_CERT", "")
	t.Setenv("MLWH_TIMEOUT", "")
}
