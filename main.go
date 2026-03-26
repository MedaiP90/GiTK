// Package main is the entry point for GiTK, a GTK4/libadwaita Git GUI client.
//
// GiTK follows the GNOME Human Interface Guidelines and uses libadwaita
// widgets for a native GNOME desktop experience. The application is built
// with Go using the gotk4 bindings for GTK4 and libadwaita.
//
// Architecture overview:
//   - main.go:       Creates the AdwApplication and starts the event loop.
//   - app/:          Application setup, window layout, keyboard shortcuts.
//   - ui/:           All UI components (sidebar, commit log, staging, etc.).
//   - git/:          Pure Go git backend — zero GTK imports.
//   - config/:       XDG-compliant configuration management.
//   - ai/:           Optional Claude API integration for commit messages.
package main

import (
	"os"

	"github.com/MedaiP90/GiTK/app"
)

func main() {
	// Create the GiTK application. The app package handles all GTK/Adwaita
	// initialization, window creation, and action registration.
	application := app.New()

	// Run the GTK main loop. os.Args is passed so GTK can process any
	// command-line flags it recognizes (e.g., --display for X11).
	// The return value is the exit code: 0 for success, >0 for errors.
	if code := application.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
