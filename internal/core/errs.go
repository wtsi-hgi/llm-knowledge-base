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
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolResultTooLargeCode = "tool_result_too_large"

// ToolResultSizeError is returned as structured content when a tool result
// exceeds the configured byte budget. It gives callers the measured size,
// configured limit, and provider-specific guidance for choosing a cheaper or
// smaller workflow.
type ToolResultSizeError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	LimitBytes  int    `json:"limit_bytes"`
	ActualBytes int    `json:"actual_bytes"`
	Guidance    string `json:"guidance"`
}

func oversizedToolResult(limitBytes, actualBytes int, guidance string) *mcp.CallToolResult {
	body := ToolResultSizeError{
		Code:        toolResultTooLargeCode,
		Message:     "tool result exceeds configured byte limit",
		LimitBytes:  limitBytes,
		ActualBytes: actualBytes,
		Guidance:    guidance,
	}

	text, err := json.Marshal(body)
	if err != nil {
		text = []byte(`{"code":"tool_result_too_large"}`)
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: body,
		IsError:           true,
	}
}

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

// ResultSizeGuard returns middleware that replaces oversized tool results with
// a structured MCP tool error. A maxBytes value <= 0 disables the guard.
func ResultSizeGuard(maxBytes int, guidance string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || maxBytes <= 0 || method != "tools/call" {
				return result, err
			}

			toolResult, ok := result.(*mcp.CallToolResult)
			if !ok || toolResult == nil {
				return result, err
			}

			actualBytes := callToolResultBytes(toolResult)
			if actualBytes <= maxBytes {
				return result, err
			}

			return oversizedToolResult(maxBytes, actualBytes, guidance), nil
		}
	}
}

func callToolResultBytes(result *mcp.CallToolResult) int {
	if body, err := json.Marshal(*result); err == nil {
		return len(body)
	}

	return max(
		marshaledByteLen(result.StructuredContent),
		marshaledByteLen(result.Content),
	)
}

func marshaledByteLen(value any) int {
	body, err := json.Marshal(value)
	if err != nil {
		return 0
	}

	return len(body)
}
