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
	"os"
	"os/user"

	wa "github.com/wtsi-hgi/wa/mlwh"
)

// identityRemoteConfig returns cfg with UserAgent set to
// wa.UserAgentProductMCP+"/"+serverVersion and Identity replaced by
// wa.ClientIdentity{MCPTransport: transport}, plus Username from
// localUsername(user.Current, os.Getenv) when transport is "stdio". Any
// UserAgent or Identity already in cfg is discarded. Other fields are kept.
func identityRemoteConfig(cfg wa.RemoteConfig, serverVersion, transport string) wa.RemoteConfig {
	cfg.UserAgent = wa.UserAgentProductMCP + "/" + serverVersion
	cfg.Identity = wa.ClientIdentity{MCPTransport: transport}

	if transport == "stdio" {
		cfg.Identity.Username = localUsername(user.Current, os.Getenv)
	}

	return cfg
}

// localUsername returns current().Username when current returns no error
// and a non-empty name, else getenv("USER"), else "".
func localUsername(current func() (*user.User, error), getenv func(string) string) string {
	if u, err := current(); err == nil && u.Username != "" {
		return u.Username
	}

	return getenv("USER")
}
