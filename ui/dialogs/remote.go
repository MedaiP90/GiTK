// Package dialogs — remote.go implements the remote management dialog.
//
// This dialog allows the user to:
//   - View all configured remotes with their URLs.
//   - Add a new remote.
//   - Remove an existing remote.
//   - Push to / pull from a specific remote.
package dialogs

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowPushDialog shows the push dialog.
func ShowPushDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Push")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(250)

	// Remote selector.
	remoteEntry := adw.NewEntryRow()
	remoteEntry.SetTitle("Remote")
	remoteEntry.SetText("origin")

	// Force push toggle.
	forceRow := adw.NewSwitchRow()
	forceRow.SetTitle("Force Push")
	forceRow.SetSubtitle("Overwrites remote history — use with caution")

	group := adw.NewPreferencesGroup()
	group.SetTitle("Push to Remote")
	group.Add(remoteEntry)
	group.Add(forceRow)

	// Buttons.
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	pushBtn := gtk.NewButtonWithLabel("Push")
	pushBtn.AddCSSClass("suggested-action")
	pushBtn.ConnectClicked(func() {
		remote := remoteEntry.Text()
		force := forceRow.Active()

		if force {
			// Show destructive action confirmation.
			showForceConfirmation(parent, func() {
				doPush(dialog, repo, remote, true, onDone)
			})
			return
		}

		doPush(dialog, repo, remote, false, onDone)
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(pushBtn)

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

// doPush runs the push operation in a goroutine.
func doPush(dialog *adw.Dialog, repo *git.Repository, remote string, force bool, onDone func(string)) {
	go func() {
		err := repo.Push(remote, force)
		glib.IdleAdd(func() {
			dialog.Close()
			if err != nil {
				slog.Warn("push failed", "remote", remote, "error", err)
				if onDone != nil {
					onDone("Push failed: " + err.Error())
				}
				return
			}
			if onDone != nil {
				onDone("Pushed to " + remote)
			}
		})
	}()
}

// showForceConfirmation shows a destructive action confirmation dialog.
func showForceConfirmation(parent *adw.ApplicationWindow, onConfirm func()) {
	dialog := adw.NewAlertDialog("Force Push?", "This will overwrite the remote branch history. This action cannot be undone.")
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("force", "Force Push")
	dialog.SetResponseAppearance("force", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "force" {
			onConfirm()
		}
	})

	dialog.Present(parent)
}

// ShowPullDialog shows the pull dialog.
func ShowPullDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Pull")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(200)

	remoteEntry := adw.NewEntryRow()
	remoteEntry.SetTitle("Remote")
	remoteEntry.SetText("origin")

	branchEntry := adw.NewEntryRow()
	branchEntry.SetTitle("Branch")
	branchEntry.SetText(repo.CurrentBranch())

	group := adw.NewPreferencesGroup()
	group.SetTitle("Pull from Remote")
	group.Add(remoteEntry)
	group.Add(branchEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	pullBtn := gtk.NewButtonWithLabel("Pull")
	pullBtn.AddCSSClass("suggested-action")
	pullBtn.ConnectClicked(func() {
		remote := remoteEntry.Text()
		branch := branchEntry.Text()

		go func() {
			err := repo.Pull(remote, branch)
			glib.IdleAdd(func() {
				dialog.Close()
				if err != nil {
					slog.Warn("pull failed", "error", err)
					if onDone != nil {
						onDone("Pull failed: " + err.Error())
					}
					return
				}
				if onDone != nil {
					onDone("Pulled from " + remote + "/" + branch)
				}
			})
		}()
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(cancelBtn)
	btnBox.Append(pullBtn)

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
