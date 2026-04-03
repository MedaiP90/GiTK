// Package app — shortcuts.go defines the keyboard shortcuts help window.
//
// GNOME HIG requires that applications provide a shortcuts window accessible
// via Ctrl+? (question mark). This window lists all available keyboard
// shortcuts grouped by category, using GtkShortcutsWindow.
//
// GtkShortcutsWindow and its child widgets (ShortcutsSection, ShortcutsGroup,
// ShortcutsShortcut) are designed to be constructed from GtkBuilder XML
// definitions. The gotk4 bindings follow this pattern — there are no
// NewShortcutsWindow() constructors. Instead, we define the UI in XML
// and load it with GtkBuilder.
package app

import (
	"log/slog"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// shortcutsUI is the GtkBuilder XML definition for the shortcuts window.
// It follows the standard format that GTK expects for GtkShortcutsWindow.
//
// The hierarchy is:
//
//	GtkShortcutsWindow
//	  └─ GtkShortcutsSection
//	       ├─ GtkShortcutsGroup "General"
//	       │    ├─ GtkShortcutsShortcut "Open repository"     Ctrl+O
//	       │    ├─ GtkShortcutsShortcut "Preferences"          Ctrl+,
//	       │    ├─ GtkShortcutsShortcut "Keyboard shortcuts"   Ctrl+?
//	       │    └─ GtkShortcutsShortcut "Quit"                 Ctrl+Q
//	       ├─ GtkShortcutsGroup "Repository"
//	       │    ├─ ...
//	       ├─ GtkShortcutsGroup "Staging"
//	       │    ├─ ...
//	       └─ GtkShortcutsGroup "Navigation"
//	            ├─ ...
const shortcutsUI = `<?xml version="1.0" encoding="UTF-8"?>
<interface>
  <object class="GtkShortcutsWindow" id="shortcuts_window">
    <property name="modal">true</property>
    <child>
      <object class="GtkShortcutsSection">
        <property name="section-name">shortcuts</property>
        <property name="title">Shortcuts</property>

        <!-- General -->
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title">General</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;o</property>
                <property name="title">Open repository</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;comma</property>
                <property name="title">Preferences</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;question</property>
                <property name="title">Keyboard shortcuts</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;q</property>
                <property name="title">Quit</property>
              </object>
            </child>
          </object>
        </child>

        <!-- Repository -->
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title">Repository</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;r</property>
                <property name="title">Refresh</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;&lt;Shift&gt;p</property>
                <property name="title">Push</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;&lt;Shift&gt;f</property>
                <property name="title">Fetch all</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;&lt;Shift&gt;l</property>
                <property name="title">Pull</property>
              </object>
            </child>
          </object>
        </child>

        <!-- Staging -->
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title">Staging</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;Return</property>
                <property name="title">Commit</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;&lt;Shift&gt;a</property>
                <property name="title">Stage all</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;&lt;Shift&gt;u</property>
                <property name="title">Unstage all</property>
              </object>
            </child>
          </object>
        </child>

        <!-- Navigation -->
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title">Navigation</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;1</property>
                <property name="title">Show commit log</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;2</property>
                <property name="title">Show graph view</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;3</property>
                <property name="title">Show staging area</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="accelerator">&lt;Control&gt;f</property>
                <property name="title">Search / filter</property>
              </object>
            </child>
          </object>
        </child>

      </object>
    </child>
  </object>
</interface>`

// ShowShortcutsWindow creates and presents the keyboard shortcuts help window.
// It is called when the user presses Ctrl+? or selects "Keyboard Shortcuts"
// from the application menu.
//
// The shortcuts window is built from a GtkBuilder XML definition (above)
// because GtkShortcutsWindow widgets don't have direct constructor functions
// in the gotk4 bindings — this is by design, as GTK recommends using Builder
// XML for these widgets.
//
// Parameters:
//   - parent: the parent window to attach the shortcuts window to.
func ShowShortcutsWindow(parent *adw.ApplicationWindow) {
	// GtkBuilder loads the XML definition and creates all the widget objects.
	builder := gtk.NewBuilderFromString(shortcutsUI)

	// Retrieve the top-level ShortcutsWindow from the builder.
	obj := builder.GetObject("shortcuts_window")
	if obj == nil {
		slog.Error("failed to load shortcuts window from builder XML")
		return
	}

	// Cast the GObject to a ShortcutsWindow. The gotk4 binding provides
	// type-safe casting via the Object's Cast method.
	win, ok := obj.Cast().(*gtk.ShortcutsWindow)
	if !ok {
		slog.Error("shortcuts_window is not a GtkShortcutsWindow")
		return
	}

	// Set the parent so the window appears centered over the main window.
	win.SetTransientFor(&parent.Window)
	win.SetModal(true)

	win.Present()
}
