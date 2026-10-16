// Package mcpserver exposes CodeDiver to coding agents over the Model Context
// Protocol. Every tool is scoped to repositories the developer has shared in
// the CodeDiver UI, and the only way an agent can change code is by filing a
// proposal that the developer reviews and accepts.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/core"
)

const instructions = `CodeDiver is a visual code-exploration desktop app the developer is watching right now.
Use it to help the developer understand and change code VISUALLY:

- Start with list_repos / get_developer_context to see what repo and code the developer is looking at.
- Explore with repo_overview, list_files, read_file, search_code, search_symbols, get_symbol.
- Analyze with get_call_graph, trace_to (how execution reaches a function), get_function_flow (control flow),
  get_dependencies, git_diff and get_heatmap. Pass show=true to put the result on the developer's screen.
- Teach with start_walkthrough (step-by-step code tour), explain_function (inputs, outputs and logic of a
  function, rendered next to its control-flow graph), show_diagram (mermaid or nodes/edges) and annotate_code.
- Change code ONLY with propose_changes. The developer reviews a visual diff, heatmap and blast-radius
  graph, then accepts or rejects; poll get_proposal for the decision and feedback. Never assume a proposal
  was applied until get_proposal says so.

Paths are repo-relative. Symbols can be referenced by id, qualified name (Type.Method), bare name or file:line.
The repo argument is optional and defaults to the repo the developer has open.`

type Server struct {
	core *core.Service
	srv  *mcp.Server
}

// New creates the MCP server with every tool registered and logs each
// newly connected client to the activity feed.
func New(c *core.Service) *Server {
	s := &Server{core: c}
	s.srv = mcp.NewServer(&mcp.Implementation{
		Name:    "codediver",
		Title:   "CodeDiver",
		Version: c.Version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			name := "agent"
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
				name = p.ClientInfo.Name
			}
			c.RecordActivity(core.Activity{Tool: "connect", Summary: name + " connected"})
		},
	})
	s.register()
	return s
}

// MCP returns the underlying SDK server (used by tests and the stdio mode).
func (s *Server) MCP() *mcp.Server { return s.srv }

// Handler serves the streamable HTTP transport.
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.srv }, &mcp.StreamableHTTPOptions{
		SessionTimeout: 30 * time.Minute,
	})
}

// Run serves MCP over stdio (used by `codediver mcp --standalone`).
func (s *Server) Run(ctx context.Context) error {
	return s.srv.Run(ctx, &mcp.StdioTransport{})
}

type textResult = *mcp.CallToolResult

// ok wraps text in a successful tool result.
func ok(text string) textResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// addTool registers a tool whose handler returns text, recording every call
// in the activity feed so the developer can see what the agent is doing.
func addTool[In any](s *Server, t *mcp.Tool, summary func(In) string, h func(context.Context, In) (string, error)) {
	mcp.AddTool(s.srv, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		out, err := h(ctx, in)
		a := core.Activity{Tool: t.Name, Summary: summary(in), DurationMS: time.Since(start).Milliseconds()}
		if err != nil {
			a.Error = err.Error()
		}
		s.core.RecordActivity(a)
		if err != nil {
			if errors.Is(err, core.ErrAgentDisabled) {
				return nil, nil, fmt.Errorf("%w (the developer can enable it under Settings → Agent access)", err)
			}
			return nil, nil, err
		}
		return ok(out), nil, nil
	})
}

// readOnly annotates a tool that only reads repository state.
func readOnly(title string) *mcp.ToolAnnotations {
	f := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &f}
}

// visual annotates a tool that changes what the UI shows but never
// repository files.
func visual(title string) *mcp.ToolAnnotations {
	f := false
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &f, OpenWorldHint: &f}
}
