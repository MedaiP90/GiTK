// Package app sets up the AdwApplication, registers global actions, and
// creates the main application window when GTK emits the "activate" signal.
//
// In GTK/libadwaita, an Application object owns the main event loop and
// manages the lifecycle of all windows. We use adw.Application (which wraps
// gtk.Application) to get automatic libadwaita styling and dark/light theme
// support that follows the system setting.
package app

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/ui/prefs"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// AppID is the reverse-DNS application identifier used for D-Bus activation,
// desktop file naming, and Flatpak packaging. It uses the GitHub URI format
// (io.github.<owner>.<repo>) so that Flathub can automatically verify
// ownership of the application.
const AppID = "io.github.MedaiP90.GiTK"

// GiTKApp holds the top-level application state. It wraps an adw.Application
// and owns the configuration and the main window reference.
//
// Design note: We avoid global variables. Instead, GiTKApp is created in
// main.go and passed around. This makes dependencies explicit and testing
// easier.
type GiTKApp struct {
	// app is the underlying libadwaita application object.
	app *adw.Application

	// cfg holds the user's persisted configuration (recent repos, prefs, etc.).
	cfg *config.Config

	// version is the application version string, injected at build time.
	version string

	// win is the main application window. It is created on the first
	// "activate" signal and reused on subsequent activations.
	win *Window
}

// New creates and configures a new GiTKApp. It does NOT start the main loop —
// call Run() for that.
//
// What happens here:
//  1. Load (or create) the user configuration from XDG directories.
//  2. Create an adw.Application with our app ID.
//  3. Connect the "activate" signal so we create the window on launch.
//  4. Register global application actions (keyboard shortcuts, menu items).
func New(version string) *GiTKApp {
	// Load configuration from XDG config directory. If the config file does
	// not exist yet, this returns sensible defaults.
	cfg, err := config.Load()
	if err != nil {
		// Non-fatal: we can run with defaults. Log the error so the user
		// can investigate if something is wrong with their config file.
		slog.Warn("failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// Create the libadwaita application. The flags are set to
	// "FlagsNone" because we handle file arguments ourselves (via the
	// "Open Repository" action) rather than letting GIO handle them.
	gtkApp := adw.NewApplication(AppID, gio.ApplicationFlagsNone)

	gitkApp := &GiTKApp{
		app:     gtkApp,
		cfg:     cfg,
		version: version,
	}

	// "activate" is emitted when the application is launched (or when the
	// user tries to launch a second instance — GTK ensures only one
	// instance runs and re-activates the existing one).
	gtkApp.ConnectActivate(func() {
		// Apply theme on first activation — StyleManager requires GTK
		// to be initialized, which only happens after the app starts.
		gitkApp.applyTheme()
		gitkApp.onActivate()
	})

	// Register global actions that are available from any point in the UI.
	// Actions are the GTK way of connecting keyboard shortcuts, menu items,
	// and buttons to behavior.
	gitkApp.registerActions()

	return gitkApp
}

// Run starts the GTK main event loop. It blocks until the application exits.
// The return value is the process exit code (0 = success).
func (a *GiTKApp) Run(args []string) int {
	return a.app.Run(args)
}

// onActivate is called when GTK emits the "activate" signal. On first call,
// it creates the main window. On subsequent calls (e.g., when the user
// clicks the app icon again), it just presents the existing window.
func (a *GiTKApp) onActivate() {
	if a.win == nil {
		// First activation: build the main window.
		a.win = NewWindow(a, a.app, a.cfg)
	}

	// Present brings the window to the front. If it was minimized or on
	// another workspace, GTK/the window manager will make it visible.
	a.win.Present()
}

// registerActions sets up application-level GActions. Each action can be
// triggered by a keyboard shortcut, a menu item, or programmatically.
//
// GTK actions follow a naming convention:
//   - "app.quit"   → application-scope action
//   - "win.close"  → window-scope action (registered on the window)
//
// We register app-scope actions here. Window-scope actions are registered
// in window.go.
func (a *GiTKApp) registerActions() {
	// --- Quit action ---
	// Triggered by Ctrl+Q. Closes all windows and exits the application.
	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(param *glib.Variant) {
		a.app.Quit()
	})
	a.app.AddAction(quitAction)
	a.app.SetAccelsForAction("app.quit", []string{"<Control>q"})

	// --- About action ---
	// Shows the About dialog with application metadata.
	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(param *glib.Variant) {
		a.showAbout()
	})
	a.app.AddAction(aboutAction)

	// --- Keyboard shortcuts help ---
	// Ctrl+? opens the shortcuts window (standard GNOME convention).
	shortcutsAction := gio.NewSimpleAction("shortcuts", nil)
	shortcutsAction.ConnectActivate(func(param *glib.Variant) {
		if a.win != nil {
			ShowShortcutsWindow(a.win.window)
		}
	})
	a.app.AddAction(shortcutsAction)
	a.app.SetAccelsForAction("app.shortcuts", []string{"<Control>question"})

	// --- Preferences ---
	// Ctrl+, opens the preferences window (standard GNOME convention).
	prefsAction := gio.NewSimpleAction("preferences", nil)
	prefsAction.ConnectActivate(func(param *glib.Variant) {
		if a.win != nil {
			prefs.Show(a.win.window, a.cfg)
		}
	})
	a.app.AddAction(prefsAction)
	a.app.SetAccelsForAction("app.preferences", []string{"<Control>comma"})
}

// applyTheme forces the application to use the dark color scheme.
func (a *GiTKApp) applyTheme() {
	sm := adw.StyleManagerGetDefault()
	sm.SetColorScheme(adw.ColorSchemeForceDark)
}

// showAbout creates and presents the GNOME-style About dialog using
// adw.AboutDialog (if available) or adw.AboutWindow.
func (a *GiTKApp) showAbout() {
	// adw.AboutDialog is the GNOME HIG-compliant way to show app info.
	about := adw.NewAboutDialog()
	about.SetApplicationName("GiTK")
	about.SetApplicationIcon(AppID)
	about.SetVersion(a.version)
	about.SetDeveloperName("MedaiP90")
	about.SetWebsite("https://github.com/MedaiP90/GiTK")
	about.SetIssueURL("https://github.com/MedaiP90/GiTK/issues")
	about.SetLicense("GPL-3.0-or-later")
	about.SetLicenseType(4) // GTK_LICENSE_GPL_3_0 = 4
	about.SetComments("A comprehensive GTK4 Git GUI client for GNOME")
	about.SetDevelopers([]string{"MedaiP90 https://github.com/MedaiP90"})

	if a.win != nil {
		about.Present(a.win.window)
	}
}
