// Package dialogs — submodule.go implements the submodule management dialogs.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowAddSubmoduleDialog shows a dialog to add a new git submodule.
func ShowAddSubmoduleDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Add Submodule")
	dialog.SetContentWidth(440)
	dialog.SetContentHeight(220)

	urlEntry := adw.NewEntryRow()
	urlEntry.SetTitle("Repository URL")

	pathEntry := adw.NewEntryRow()
	pathEntry.SetTitle("Local Path")

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Submodule")
	group.Add(urlEntry)
	group.Add(pathEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	addBtn := gtk.NewButtonWithLabel("Add Submodule")
	addBtn.AddCSSClass("suggested-action")
	addBtn.ConnectClicked(func() {
		url := urlEntry.Text()
		path := pathEntry.Text()
		if url == "" || path == "" {
			return
		}
		go func() {
			err := repo.AddSubmodule(url, path)
			glib.IdleAdd(func() {
				dialog.Close()
				if err != nil {
					slog.Warn("add submodule failed", "url", url, "error", err)
					if onDone != nil {
						onDone("Add submodule failed: " + err.Error())
					}
					return
				}
				if onDone != nil {
					onDone("Submodule '" + path + "' added")
				}
			})
		}()
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(addBtn)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(12)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(group)
	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.Present(parent)
}

// ShowRemoveSubmoduleConfirmDialog shows a confirmation dialog before removing a submodule.
func ShowRemoveSubmoduleConfirmDialog(parent *adw.ApplicationWindow, path string, onConfirm func()) {
	dialog := adw.NewAlertDialog(
		"Remove Submodule?",
		"This will deinitialise and remove '"+path+"' from the repository. This action cannot be easily undone.",
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("remove", "Remove")
	dialog.SetResponseAppearance("remove", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "remove" {
			onConfirm()
		}
	})

	dialog.Present(parent)
}
