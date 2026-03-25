// Package dialogs — stash.go implements the stash management dialog.
//
// Note: go-git v5 has limited stash support. This dialog provides the
// UI framework; the actual stash operations will need a future go-git
// update or shell-out to the git CLI.
package dialogs

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowStashDialog shows the stash management dialog.
func ShowStashDialog(parent *adw.ApplicationWindow, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Stash")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(200)

	// Message entry.
	messageEntry := adw.NewEntryRow()
	messageEntry.SetTitle("Stash Message (optional)")

	// Include untracked toggle.
	untrackedRow := adw.NewSwitchRow()
	untrackedRow.SetTitle("Include Untracked Files")

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Stash")
	group.SetDescription("Stash support requires git CLI (go-git limitation).")
	group.Add(messageEntry)
	group.Add(untrackedRow)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	stashBtn := gtk.NewButtonWithLabel("Stash")
	stashBtn.AddCSSClass("suggested-action")
	stashBtn.SetSensitive(false) // Disabled until go-git supports stash.
	stashBtn.SetTooltipText("Stash is not yet supported by the go-git backend")

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(stashBtn)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(group)
	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.Present(parent)
}
