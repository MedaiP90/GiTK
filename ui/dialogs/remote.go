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

// buildComboRow creates an AdwComboRow populated with the given items and
// pre-selects the item matching defaultValue (or index 0 if not found).
func buildComboRow(title string, items []string, defaultValue string) *adw.ComboRow {
	combo := adw.NewComboRow()
	combo.SetTitle(title)

	list := gtk.NewStringList(items)
	combo.SetModel(list)

	for i, item := range items {
		if item == defaultValue {
			combo.SetSelected(uint(i))
			break
		}
	}

	return combo
}

// comboSelectedString returns the currently selected string from a ComboRow
// that uses a StringList model.
func comboSelectedString(combo *adw.ComboRow, items []string) string {
	idx := combo.Selected()
	if int(idx) < len(items) {
		return items[idx]
	}
	if len(items) > 0 {
		return items[0]
	}
	return ""
}

// ShowPushDialog shows the push dialog.
func ShowPushDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Push")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(250)

	// Collect remote names for the dropdown.
	remoteNames := getRemoteNames(repo)

	// Remote selector.
	remoteCombo := buildComboRow("Remote", remoteNames, "origin")

	// Force push toggle.
	forceRow := adw.NewSwitchRow()
	forceRow.SetTitle("Force Push")
	forceRow.SetSubtitle("Overwrites remote history — use with caution")

	group := adw.NewPreferencesGroup()
	group.SetTitle("Push to Remote")
	group.Add(remoteCombo)
	group.Add(forceRow)

	// Buttons.
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)

	pushBtn := gtk.NewButtonWithLabel("Push")
	pushBtn.AddCSSClass("suggested-action")
	pushBtn.ConnectClicked(func() {
		remote := comboSelectedString(remoteCombo, remoteNames)
		force := forceRow.Active()

		startOp := func() {
			pushBtn.SetSensitive(false)
			cancelBtn.SetSensitive(false)
			spinner.SetVisible(true)
			spinner.Start()
			doPush(dialog, repo, remote, force, onDone)
		}

		if force {
			showForceConfirmation(parent, startOp)
			return
		}
		startOp()
	})

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetVAlign(gtk.AlignCenter)
	btnBox.SetMarginTop(18)
	btnBox.Append(spinner)
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

// ShowAddRemoteDialog shows a dialog to add a new git remote.
func ShowAddRemoteDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Add Remote")
	dialog.SetContentWidth(420)
	dialog.SetContentHeight(220)

	nameEntry := adw.NewEntryRow()
	nameEntry.SetTitle("Name")
	nameEntry.SetText("origin")

	urlEntry := adw.NewEntryRow()
	urlEntry.SetTitle("URL")

	group := adw.NewPreferencesGroup()
	group.SetTitle("New Remote")
	group.Add(nameEntry)
	group.Add(urlEntry)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	addBtn := gtk.NewButtonWithLabel("Add Remote")
	addBtn.AddCSSClass("suggested-action")
	addBtn.ConnectClicked(func() {
		name := nameEntry.Text()
		url := urlEntry.Text()
		if name == "" || url == "" {
			return
		}
		go func() {
			err := repo.AddRemote(name, url)
			glib.IdleAdd(func() {
				dialog.Close()
				if err != nil {
					slog.Warn("add remote failed", "name", name, "error", err)
					if onDone != nil {
						onDone("Add remote failed: " + err.Error())
					}
					return
				}
				if onDone != nil {
					onDone("Remote '" + name + "' added")
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
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(group)
	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.Present(parent)
}

// ShowPullDialog shows the pull dialog.
func ShowPullDialog(parent *adw.ApplicationWindow, repo *git.Repository, onDone func(string)) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Pull")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(200)

	// Collect remote and branch names for the dropdowns.
	remoteNames := getRemoteNames(repo)
	branchNames := getLocalBranchNames(repo)

	remoteCombo := buildComboRow("Remote", remoteNames, "origin")
	branchCombo := buildComboRow("Branch", branchNames, repo.CurrentBranch())

	group := adw.NewPreferencesGroup()
	group.SetTitle("Pull from Remote")
	group.Add(remoteCombo)
	group.Add(branchCombo)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { dialog.Close() })

	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)

	pullBtn := gtk.NewButtonWithLabel("Pull")
	pullBtn.AddCSSClass("suggested-action")
	pullBtn.ConnectClicked(func() {
		remote := comboSelectedString(remoteCombo, remoteNames)
		branch := comboSelectedString(branchCombo, branchNames)

		pullBtn.SetSensitive(false)
		cancelBtn.SetSensitive(false)
		spinner.SetVisible(true)
		spinner.Start()

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
	btnBox.SetVAlign(gtk.AlignCenter)
	btnBox.SetMarginTop(18)
	btnBox.Append(spinner)
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

// getRemoteNames returns a list of remote names from the repository.
// Falls back to ["origin"] if remotes cannot be loaded.
func getRemoteNames(repo *git.Repository) []string {
	remotes, err := repo.Remotes()
	if err != nil || len(remotes) == 0 {
		return []string{"origin"}
	}
	names := make([]string, len(remotes))
	for i, r := range remotes {
		names[i] = r.Name
	}
	return names
}

// getLocalBranchNames returns a list of local branch names from the repository.
// Falls back to the current branch name if branches cannot be loaded.
func getLocalBranchNames(repo *git.Repository) []string {
	branches, err := repo.Branches()
	if err != nil || len(branches) == 0 {
		return []string{repo.CurrentBranch()}
	}
	var names []string
	for _, b := range branches {
		if !b.IsRemote {
			names = append(names, b.Name)
		}
	}
	if len(names) == 0 {
		return []string{repo.CurrentBranch()}
	}
	return names
}
