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

package core

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolError returns the (*mcp.CallToolResult, zero Out, error) triple a typed
// tool handler can return directly to signal a tool error. The MCP SDK packs the
// returned error into an IsError result (CallToolResult.IsError=true with the
// message in Content), rather than a protocol-level error.
//
// It is generic over Out so a handler can return the zero value of its own output
// type. Providers map their own domain errors to a clear message before calling
// it. The message is taken verbatim from err.
//
// The helper is service-agnostic: it carries no provider-specific behaviour, so
// every provider's handlers can use it without the core knowing their domain.
func ToolError[Out any](err error) (*mcp.CallToolResult, Out, error) {
	var zero Out

	if err == nil {
		err = errors.New("tool error")
	}

	return nil, zero, err
}
