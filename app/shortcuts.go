// Package app — shortcuts.go defines the keyboard shortcuts help window.
//
// GNOME HIG requires that applications provide a shortcuts window accessible
// via Ctrl+? (question mark). This window lists all available keyboard
// shortcuts grouped by category, using GtkShortcutsWindow.
//
// GtkShortcutsWindow is a special GTK widget designed specifically for
// displaying keyboard shortcuts. It automatically handles search and
// filtering, and it renders accelerator strings (like "<Control>o") as
// nice visual key caps.
package app

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowShortcutsWindow creates and presents the keyboard shortcuts help window.
// It is called when the user presses Ctrl+? or selects "Keyboard Shortcuts"
// from the application menu.
//
// Parameters:
//   - parent: the parent window to attach the shortcuts window to.
func ShowShortcutsWindow(parent *adw.ApplicationWindow) {
	// GtkShortcutsWindow is built from a hierarchy of:
	//   ShortcutsWindow → ShortcutsSection → ShortcutsGroup → ShortcutsShortcut
	//
	// Each ShortcutsShortcut displays one keyboard shortcut with its
	// description and a visual representation of the key combination.

	// --- General section ---
	generalGroup := buildShortcutGroup("General", []shortcutDef{
		{accel: "<Control>o", title: "Open repository"},
		{accel: "<Control>comma", title: "Preferences"},
		{accel: "<Control>question", title: "Keyboard shortcuts"},
		{accel: "<Control>q", title: "Quit"},
	})

	// --- Repository section ---
	repoGroup := buildShortcutGroup("Repository", []shortcutDef{
		{accel: "<Control>r", title: "Refresh"},
		{accel: "<Control><Shift>p", title: "Push"},
		{accel: "<Control><Shift>f", title: "Fetch all"},
		{accel: "<Control><Shift>l", title: "Pull"},
	})

	// --- Staging section ---
	stagingGroup := buildShortcutGroup("Staging", []shortcutDef{
		{accel: "<Control>Return", title: "Commit"},
		{accel: "<Control><Shift>a", title: "Stage all"},
		{accel: "<Control><Shift>u", title: "Unstage all"},
	})

	// --- Navigation section ---
	navGroup := buildShortcutGroup("Navigation", []shortcutDef{
		{accel: "<Control>1", title: "Show commit log"},
		{accel: "<Control>2", title: "Show graph view"},
		{accel: "<Control>3", title: "Show staging area"},
		{accel: "<Control>f", title: "Search / filter"},
	})

	// Build the section that contains all groups.
	section := gtk.NewShortcutsSection()
	section.SetProperty("section-name", "shortcuts")
	section.SetProperty("title", "Shortcuts")
	section.AddGroup(generalGroup)
	section.AddGroup(repoGroup)
	section.AddGroup(stagingGroup)
	section.AddGroup(navGroup)

	// Build the shortcuts window.
	win := gtk.NewShortcutsWindow()
	win.AddSection(section)

	// Set the parent so the window appears centered over the main window.
	win.SetTransientFor(&parent.Window)
	win.SetModal(true)

	win.Present()
}

// shortcutDef is a simple struct to define a keyboard shortcut for the
// shortcuts window. It pairs an accelerator string with a human-readable
// description.
type shortcutDef struct {
	// accel is the GTK accelerator string, e.g. "<Control>o".
	accel string

	// title is the human-readable description, e.g. "Open repository".
	title string
}

// buildShortcutGroup creates a GtkShortcutsGroup with the given title and
// list of shortcut definitions.
//
// Parameters:
//   - title: the group heading displayed in the shortcuts window.
//   - defs: a slice of shortcut definitions to display in this group.
func buildShortcutGroup(title string, defs []shortcutDef) *gtk.ShortcutsGroup {
	group := gtk.NewShortcutsGroup()
	group.SetProperty("title", title)

	for _, def := range defs {
		shortcut := gtk.NewShortcutsShortcut()
		shortcut.SetProperty("accelerator", def.accel)
		shortcut.SetProperty("title", def.title)
		group.AddShortcut(shortcut)
	}

	return group
}
