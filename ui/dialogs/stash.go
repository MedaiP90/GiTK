// Package dialogs — stash.go implements the stash creation dialog.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowStashDialog shows the stash creation dialog.
// On success, onDone is called with a status message.
func ShowStashDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Stash Changes")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(180)

	// Message entry.
	messageEntry := adw.NewEntryRow()
	messageEntry.SetTitle("Message (optional)")

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Stash")
	group.Add(messageEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	stashBtn := gtk.NewButtonWithLabel("Stash")
	stashBtn.AddCSSClass("suggested-action")
	stashBtn.ConnectClicked(func() {
		msg := messageEntry.Text()
		go func() {
			err := repo.StashSave(msg)
			glib.IdleAdd(func() {
				dialog.Close()
				if err != nil {
					slog.Warn("stash save failed", "error", err)
					if onDone != nil {
						onDone("Stash failed: " + err.Error())
					}
					return
				}
				if onDone != nil {
					onDone("Changes stashed")
				}
			})
		}()
	})

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
