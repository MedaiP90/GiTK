// Package dialogs — branch.go implements the branch creation dialog.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowCreateBranchDialog shows the dialog for creating a new local branch.
//
// Parameters:
//   - parent: the parent window.
//   - repo: the repository to create the branch in.
//   - commitHash: the commit hash to branch from (empty = HEAD).
//   - onDone: callback with result message, also receives the new branch name.
func ShowCreateBranchDialog(parent *adw.ApplicationWindow, repo *git.Repository, commitHash string, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Create Branch")
	dialog.SetContentWidth(420)
	dialog.SetContentHeight(230)

	// Error banner shown on failure.
	errorBanner := adw.NewBanner("")
	errorBanner.SetRevealed(false)

	nameEntry := adw.NewEntryRow()
	nameEntry.SetTitle("Branch Name")

	// Target commit.
	targetEntry := adw.NewEntryRow()
	targetEntry.SetTitle("From Commit")
	if commitHash != "" {
		targetEntry.SetText(commitHash)
	} else {
		targetEntry.SetText("HEAD")
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Branch")
	group.Add(nameEntry)
	group.Add(targetEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	createBtn := gtk.NewButtonWithLabel("Create Branch")
	createBtn.AddCSSClass("suggested-action")
	createBtn.ConnectClicked(func() {
		name := nameEntry.Text()
		target := targetEntry.Text()
		if name == "" {
			errorBanner.SetTitle("Branch name is required")
			errorBanner.SetRevealed(true)
			return
		}

		go func() {
			err := repo.CreateBranch(name, target)
			glib.IdleAdd(func() {
				dialog.Close()
				if err != nil {
					slog.Warn("create branch failed", "name", name, "error", err)
					if onDone != nil {
						onDone("Create branch failed: " + err.Error())
					}
					return
				}
				if onDone != nil {
					onDone("Branch '" + name + "' created")
				}
			})
		}()
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(createBtn)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(12)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(errorBanner)
	content.Append(group)
	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.Present(parent)
}
