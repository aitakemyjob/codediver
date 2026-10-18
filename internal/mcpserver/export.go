package mcpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/core"
)

type exportArgs struct {
	Repo   string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Demo   string `json:"demo,omitempty" jsonschema:"demo id from show_demo; defaults to the newest demo"`
	Step   int    `json:"step,omitempty" jsonschema:"1-based step to render; defaults to the last step (the whole scene built up)"`
	Format string `json:"format,omitempty" jsonschema:"png (default; returned as an image you can see) or svg (returned as SVG text you can save into docs)"`
	Theme  string `json:"theme,omitempty" jsonschema:"light (best for docs and print), dark, or current (the developer's theme, default)"`
}

// addExportTool registers export_demo, which returns a demo step as an
// image. Unlike most tools its result is not plain text, so it records
// activity itself.
func addExportTool(s *Server) {
	t := &mcp.Tool{
		Name:        "export_demo",
		Description: "Render a step of a demo you showed with show_demo as a PNG image (to look at it yourself, e.g. to check the layout reads well) or as SVG text (to save into documentation, a README or a design doc). The whole diagram is framed. Rendering happens in the CodeDiver window, so the app must be open.",
		Annotations: readOnly("Export demo"),
	}
	mcp.AddTool(s.srv, t, func(ctx context.Context, _ *mcp.CallToolRequest, in exportArgs) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		res, err := exportDemo(ctx, s, in)
		a := core.Activity{Tool: t.Name, Summary: fmt.Sprintf("%s %s", in.Demo, in.Format), DurationMS: time.Since(start).Milliseconds()}
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
		return res, nil, nil
	})
}

// exportDemo renders the requested step and wraps it as tool content.
func exportDemo(ctx context.Context, s *Server, in exportArgs) (*mcp.CallToolResult, error) {
	rt, err := s.core.AgentRepo(in.Repo)
	if err != nil {
		return nil, err
	}
	r, d, err := s.core.RenderDemo(ctx, rt.ID(), in.Demo, in.Step, in.Format, in.Theme)
	if err != nil {
		return nil, err
	}
	step := in.Step
	if step == 0 {
		step = len(d.Steps)
	}
	what := fmt.Sprintf("%s, step %d of %d (%q)", d.Title, step, len(d.Steps), d.Steps[step-1].Title)
	if in.Format == "svg" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("SVG of %s:\n\n%s", what, r.Data)}}}, nil
	}
	png, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		return nil, fmt.Errorf("bad image from CodeDiver: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: fmt.Sprintf("PNG of %s, %d×%d px:", what, r.Width, r.Height)},
		&mcp.ImageContent{Data: png, MIMEType: "image/png"},
	}}, nil
}
