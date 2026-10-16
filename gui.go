//go:build !nogui

package main

import (
	"context"
	"encoding/json"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/aitakemyjob/codediver/internal/api"
	"github.com/aitakemyjob/codediver/internal/core"
)

// Desktop is bound into the webview as window.go.main.Desktop.
type Desktop struct {
	ctx  context.Context
	api  *api.API
	core *core.Service
}

// Invoke calls an API method; args and result are JSON strings.
func (d *Desktop) Invoke(method, args string) (string, error) {
	out, err := d.api.Invoke(d.ctx, method, json.RawMessage(args))
	return string(out), err
}

// startup runs when the window is ready: it wires the native folder
// picker and forwards bus events to the webview.
func (d *Desktop) startup(ctx context.Context) {
	d.ctx = ctx
	d.api.PickDirectory = func() (string, error) {
		return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{Title: "Add a repository to CodeDiver", CanCreateDirectories: false})
	}
	ch, cancel := d.core.Bus.Subscribe()
	go func() {
		defer cancel()
		for ev := range ch {
			wruntime.EventsEmit(ctx, "gd:event", ev)
		}
	}()
}

// runGUI starts the MCP server and opens the desktop window. A port
// conflict is shown in the UI rather than preventing startup.
func runGUI(port int, add string) error {
	a, err := bootstrap(port, add, false)
	if err != nil {
		return err
	}
	// The MCP server failing to bind (e.g. port in use) must not stop the UI;
	// the status bar surfaces the error.
	_ = a.server.Start()
	desk := &Desktop{api: a.api, core: a.core}
	bg := &options.RGBA{R: 11, G: 17, B: 32, A: 255} // Deep Dive --bg
	return wails.Run(&options.App{
		Title:            "CodeDiver",
		Width:            1480,
		Height:           920,
		MinWidth:         980,
		MinHeight:        620,
		BackgroundColour: bg,
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        desk.startup,
		OnShutdown: func(ctx context.Context) {
			a.server.Shutdown(ctx)
		},
		Bind: []interface{}{desk},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			About:                &mac.AboutInfo{Title: "CodeDiver", Message: "Visual code exploration for humans and agents. " + version},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			Theme:                windows.SystemDefault,
		},
		Linux: &linux.Options{
			ProgramName:         "CodeDiver",
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			WindowIsTranslucent: false,
		},
	})
}
