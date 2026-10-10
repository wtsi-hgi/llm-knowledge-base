//go:build ignore

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

// mcpclient is the verify-mlwh-mcp skill's MCP client: a real go-sdk client
// that talks to mlwh-mcp-server the way an agent host does.
//
// Build it from the repository root so it uses this repo's go.mod (and so the
// same go-sdk version as the server):
//
//	go build -o <out>/mcpclient .agents/skills/verify-mlwh-mcp/scripts/mcpclient.go
//
// Usage:
//
//	mcpclient -stdio <server-binary> [-stderr <file>] [-name N -version V] <op>
//	mcpclient -http <url>/mcp [-ua <User-Agent>] [-name N -version V] <op>
//
// Ops: "list" prints server info, tool names, and every resource's text;
// "call <tool> <json-args>" prints the tool name, arguments, and the full
// CallToolResult. Both print one JSON object on stdout and exit 0 when the MCP
// exchange completed (check isError yourself); transport failures exit 1.
// The stdio server inherits this process's environment (MLWH_BASE_URL).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := client(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mcpclient:", err)
		os.Exit(1)
	}
}

type uaTransport struct {
	ua string
}

func (t uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", t.ua)

	return http.DefaultTransport.RoundTrip(r)
}

func client(args []string) error {
	fs := flag.NewFlagSet("mcpclient", flag.ContinueOnError)
	stdio := fs.String("stdio", "", "server binary to spawn over stdio")
	stderrPath := fs.String("stderr", "", "file for the stdio server's stderr")
	endpoint := fs.String("http", "", "streamable HTTP endpoint, e.g. http://127.0.0.1:8081/mcp")
	ua := fs.String("ua", "", "User-Agent for HTTP requests")
	name := fs.String("name", "verify-mlwh-mcp", "client name sent in initialize")
	version := fs.String("version", "1.0.0", "client version sent in initialize")
	timeout := fs.Duration("timeout", 60*time.Second, "overall timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if (*stdio == "") == (*endpoint == "") {
		return errors.New("give exactly one of -stdio or -http")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var transport mcp.Transport

	if *stdio != "" {
		cmd := exec.Command(*stdio)
		cmd.Env = os.Environ()

		if *stderrPath != "" {
			f, err := os.Create(*stderrPath)
			if err != nil {
				return err
			}
			defer f.Close()

			cmd.Stderr = f
		}

		transport = &mcp.CommandTransport{Command: cmd}
	} else {
		st := &mcp.StreamableClientTransport{Endpoint: *endpoint}
		if *ua != "" {
			st.HTTPClient = &http.Client{Transport: uaTransport{ua: *ua}}
		}

		transport = st
	}

	c := mcp.NewClient(&mcp.Implementation{Name: *name, Version: *version}, nil)

	session, err := c.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer session.Close()

	var out any

	switch op := fs.Arg(0); op {
	case "list":
		out, err = list(ctx, session)
	case "call":
		out, err = call(ctx, session, fs.Arg(1), fs.Arg(2))
	default:
		return fmt.Errorf("unknown op %q (want list or call)", op)
	}

	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	return enc.Encode(out)
}

func list(ctx context.Context, s *mcp.ClientSession) (any, error) {
	tools, err := s.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	names := make([]string, 0, len(tools.Tools))
	for _, t := range tools.Tools {
		names = append(names, t.Name)
	}

	resources, err := s.ListResources(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("resources/list: %w", err)
	}

	texts := map[string]string{}

	for _, r := range resources.Resources {
		read, err := s.ReadResource(ctx, &mcp.ReadResourceParams{URI: r.URI})
		if err != nil {
			return nil, fmt.Errorf("resources/read %s: %w", r.URI, err)
		}

		for _, c := range read.Contents {
			texts[r.URI] += c.Text
		}
	}

	return map[string]any{
		"server":    s.InitializeResult().ServerInfo,
		"tools":     names,
		"resources": texts,
	}, nil
}

func call(ctx context.Context, s *mcp.ClientSession, tool, rawArgs string) (any, error) {
	if tool == "" {
		return nil, errors.New("call needs a tool name")
	}

	var arguments map[string]any

	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &arguments); err != nil {
			return nil, fmt.Errorf("arguments: %w", err)
		}
	}

	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", tool, err)
	}

	return map[string]any{"tool": tool, "arguments": arguments, "result": res}, nil
}
