// Package dialogs — tag.go implements the tag creation dialog.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowCreateTagDialog shows the dialog for creating a new tag.
// After creation, the tag is automatically pushed to the remote.
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
	dialog.SetContentHeight(350)

	// Error banner.
	errorBanner := adw.NewBanner("")
	errorBanner.SetRevealed(false)

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
	group.SetDescription("The tag will be created locally and pushed to the remote automatically.")
	group.Add(nameEntry)
	group.Add(messageEntry)
	group.Add(targetEntry)

	// Spinner for push progress.
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	createBtn := gtk.NewButtonWithLabel("Create & Push Tag")
	createBtn.AddCSSClass("suggested-action")
	createBtn.ConnectClicked(func() {
		name := nameEntry.Text()
		message := messageEntry.Text()
		target := targetEntry.Text()

		if name == "" {
			errorBanner.SetTitle("Please enter a tag name.")
			errorBanner.SetRevealed(true)
			return
		}

		// If target is "HEAD", pass empty string so the git backend uses HEAD.
		if target == "HEAD" {
			target = ""
		}

		// Disable UI during operation.
		createBtn.SetSensitive(false)
		cancelBtn.SetSensitive(false)
		spinner.SetVisible(true)
		spinner.Start()
		errorBanner.SetRevealed(false)

		// Run in background goroutine.
		go func() {
			// Step 1: Create the tag locally.
			err := repo.CreateTag(name, target, message)
			if err != nil {
				glib.IdleAdd(func() {
					spinner.Stop()
					spinner.SetVisible(false)
					createBtn.SetSensitive(true)
					cancelBtn.SetSensitive(true)
					slog.Warn("create tag failed", "name", name, "error", err)
					errorBanner.SetTitle("Failed to create tag: " + err.Error())
					errorBanner.SetRevealed(true)
				})
				return
			}

			// Step 2: Push the tag to remote.
			pushErr := repo.PushTag(name, "origin")

			glib.IdleAdd(func() {
				spinner.Stop()
				spinner.SetVisible(false)
				dialog.Close()

				if pushErr != nil {
					slog.Warn("push tag failed", "name", name, "error", pushErr)
					if onDone != nil {
						onDone("Tag " + name + " created locally but push failed: " + pushErr.Error())
					}
					return
				}

				if onDone != nil {
					onDone("Created and pushed tag " + name)
				}
			})
		}()
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(spinner)
	btnBox.Append(cancelBtn)
	btnBox.Append(createBtn)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(errorBanner)
	content.Append(group)
	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.Present(parent)
}
