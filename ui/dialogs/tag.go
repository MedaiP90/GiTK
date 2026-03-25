// Package dialogs — tag.go implements the tag creation dialog.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowCreateTagDialog shows the dialog for creating a new tag.
//
// Parameters:
//   - parent: the parent window.
//   - repo: the repository to create the tag in.
//   - commitHash: optional commit hash (empty = HEAD).
//   - onDone: callback with result message.
func ShowCreateTagDialog(parent *adw.ApplicationWindow, repo *git.Repository, commitHash string, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Create Tag")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(300)

	// Tag name.
	nameEntry := adw.NewEntryRow()
	nameEntry.SetTitle("Tag Name")

	// Message (for annotated tags).
	messageEntry := adw.NewEntryRow()
	messageEntry.SetTitle("Message (optional)")

	// Target commit.
	targetEntry := adw.NewEntryRow()
	targetEntry.SetTitle("Target Commit")
	if commitHash != "" {
		targetEntry.SetText(commitHash)
	} else {
		targetEntry.SetText("HEAD")
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Tag")
	group.SetDescription("Leave message empty for a lightweight tag.")
	group.Add(nameEntry)
	group.Add(messageEntry)
	group.Add(targetEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	createBtn := gtk.NewButtonWithLabel("Create Tag")
	createBtn.AddCSSClass("suggested-action")
	createBtn.ConnectClicked(func() {
		name := nameEntry.Text()
		message := messageEntry.Text()
		target := targetEntry.Text()

		if name == "" {
			return
		}

		// If target is "HEAD", pass empty string so the git backend uses HEAD.
		if target == "HEAD" {
			target = ""
		}

		err := repo.CreateTag(name, target, message)
		dialog.Close()

		if err != nil {
			slog.Warn("create tag failed", "name", name, "error", err)
			if onDone != nil {
				onDone("Failed to create tag: " + err.Error())
			}
			return
		}

		if onDone != nil {
			onDone("Created tag " + name)
		}
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(createBtn)

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
