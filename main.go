// Command codediver is a desktop app for exploring and reviewing code
// visually, with a built-in MCP server so coding agents can drive it.
//
//	codediver                 launch the desktop app
//	codediver serve           run headless and open the UI in a browser
//	codediver mcp             stdio MCP bridge to the running app (for stdio-only agents)
//	codediver version
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/aitakemyjob/codediver/internal/api"
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/server"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `CodeDiver %s — visual code exploration with an MCP server for agents.

Usage:
  codediver [flags]          launch the desktop app
  codediver serve [flags]    run without a window and use the UI in your browser
  codediver mcp [flags]      stdio MCP bridge to the running app (Claude Desktop & other stdio-only clients)
  codediver version          print the version

Flags:
  -port N      MCP/HTTP port (default from settings, %d)
  -add PATH    add a repository before starting
  -open        (serve) open the browser automatically (default true)

The MCP endpoint is http://127.0.0.1:<port>/mcp (streamable HTTP).
`

type app struct {
	core   *core.Service
	api    *api.API
	server *server.Server
}

// bootstrap loads the config, optionally adds a repository, and wires the
// service, API and HTTP server. withUI also serves the embedded frontend.
func bootstrap(port int, addPath string, withUI bool) (*app, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	store, err := config.Open(cfgPath)
	if err != nil {
		return nil, err
	}
	svc := core.New(store, version)
	if addPath != "" {
		r, err := svc.AddRepo(addPath, "")
		if err != nil {
			return nil, fmt.Errorf("add %s: %w", addPath, err)
		}
		svc.SetActiveRepo(r.ID)
	}
	if port == 0 {
		port = store.Get().Settings.MCPPort
	}
	a := api.New(svc)
	opts := server.Options{Port: port}
	if withUI {
		ui, err := fs.Sub(assets, "frontend/dist")
		if err != nil {
			return nil, err
		}
		opts.UI = ui
	}
	return &app{core: svc, api: a, server: server.New(svc, a, opts)}, nil
}

// main dispatches the subcommands: the desktop app (default), serve, mcp
// and version.
func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fl := flag.NewFlagSet("codediver", flag.ExitOnError)
	port := fl.Int("port", 0, "MCP/HTTP port")
	add := fl.String("add", "", "repository to add")
	open := fl.Bool("open", true, "open the browser (serve)")
	standalone := fl.Bool("standalone", false, "(mcp) serve stdio directly instead of bridging to the app")
	fl.Usage = func() { fmt.Fprintf(os.Stderr, usage, version, config.DefaultMCPPort) }
	fl.Parse(args)

	var err error
	switch cmd {
	case "", "gui":
		err = runGUI(*port, *add)
	case "serve":
		err = runServe(*port, *add, *open)
	case "mcp":
		err = runMCP(*port, *standalone)
	case "version", "--version":
		fmt.Println("codediver", version)
	case "help":
		fl.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		fl.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codediver:", err)
		os.Exit(1)
	}
}

// runServe runs headless, serving the UI and MCP over HTTP until
// interrupted, and optionally opens the browser.
func runServe(port int, add string, open bool) error {
	a, err := bootstrap(port, add, true)
	if err != nil {
		return err
	}
	if err := a.server.Start(); err != nil {
		return err
	}
	url := a.server.URL()
	fmt.Printf("CodeDiver %s\n  UI:  %s\n  MCP: %s/mcp\n", version, url, url)
	if open {
		openBrowser(url)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Shutdown(shutdown)
}

// openBrowser opens url in the default browser (best effort).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("could not open browser", "err", err)
	}
}
