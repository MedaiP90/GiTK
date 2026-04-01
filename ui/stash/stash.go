// Package stash implements the stash management view for GiTK.
//
// The stash view shows a list of all stash entries for the current repository.
// For each stash entry the user can:
//   - Apply  — restores the stash without removing it.
//   - Pop    — restores the stash and removes it from the list.
//   - Drop   — discards the stash without applying it (asks for confirmation).
//
// A "Clear Stash" button at the top deletes all stash entries after confirmation.
package stash

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// StashView is the stash management page widget.
type StashView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *gtk.Box

	// repo is the currently open repository.
	repo *git.Repository

	// listBox holds the stash entry rows.
	listBox *gtk.ListBox

	// parentWindow is used as presenter for confirmation dialogs.
	parentWindow gtk.Widgetter
}

// New creates a new StashView.
func New() *StashView {
	sv := &StashView{}
	sv.build()
	return sv
}

// SetWindow stores the parent window reference needed for dialogs.
func (sv *StashView) SetWindow(w gtk.Widgetter) {
	sv.parentWindow = w
}

// build constructs the stash view widgets.
func (sv *StashView) build() {
	sv.Root = gtk.NewBox(gtk.OrientationVertical, 0)
	sv.Root.SetVExpand(true)

	// Top bar with "Clear Stash" button.
	topBar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	topBar.SetMarginTop(12)
	topBar.SetMarginBottom(12)
	topBar.SetMarginStart(18)
	topBar.SetMarginEnd(18)

	titleLabel := gtk.NewLabel("Stash")
	titleLabel.AddCSSClass("title-4")
	titleLabel.SetHExpand(true)
	titleLabel.SetXAlign(0)
	topBar.Append(titleLabel)

	clearStashBtn := gtk.NewButtonWithLabel("Clear Stash")
	clearStashBtn.AddCSSClass("destructive-action")
	clearStashBtn.SetTooltipText("Delete all stashed changes")
	clearStashBtn.ConnectClicked(func() {
		sv.confirmClearStash()
	})
	topBar.Append(clearStashBtn)

	sv.Root.Append(topBar)

	// Separator between top bar and list.
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sv.Root.Append(sep)

	// Scrolled list of stash entries.
	sv.listBox = gtk.NewListBox()
	sv.listBox.SetSelectionMode(gtk.SelectionNone)
	sv.listBox.AddCSSClass("boxed-list")
	sv.listBox.SetMarginTop(18)
	sv.listBox.SetMarginBottom(18)
	sv.listBox.SetMarginStart(18)
	sv.listBox.SetMarginEnd(18)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(sv.listBox)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	sv.Root.Append(scrolled)
}

// confirmClearStash shows a confirmation dialog before clearing all stashes.
func (sv *StashView) confirmClearStash() {
	if sv.repo == nil {
		return
	}

	stashes, err := sv.repo.StashList()
	if err != nil || len(stashes) == 0 {
		return
	}

	body := fmt.Sprintf("Delete all %d stash entries? This cannot be undone.", len(stashes))
	dialog := adw.NewAlertDialog("Clear Stash", body)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("clear", "Clear All")
	dialog.SetResponseAppearance("clear", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "clear" {
			return
		}
		go func() {
			err := sv.repo.StashClear()
			glib.IdleAdd(func() {
				if err != nil {
					slog.Warn("stash clear failed", "error", err)
				}
				sv.RefreshStashes()
			})
		}()
	})
	dialog.Present(sv.parentWindow)
}

// SetRepository sets the current repository and refreshes the stash list.
func (sv *StashView) SetRepository(repo *git.Repository) {
	sv.repo = repo
	sv.RefreshStashes()
}

// RefreshStashes reloads the stash list from the repository.
func (sv *StashView) RefreshStashes() {
	// Clear existing rows.
	for {
		row := sv.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		sv.listBox.Remove(row)
	}

	if sv.repo == nil {
		sv.showEmpty("No repository open")
		return
	}

	stashes, err := sv.repo.StashList()
	if err != nil {
		slog.Warn("failed to list stashes", "error", err)
		sv.showEmpty("Could not load stash list")
		return
	}

	if len(stashes) == 0 {
		sv.showEmpty("No stashed changes")
		return
	}

	for _, stash := range stashes {
		sv.listBox.Append(sv.buildStashRow(stash))
	}
}

// showEmpty adds a placeholder row when the stash is empty or unavailable.
func (sv *StashView) showEmpty(msg string) {
	row := adw.NewActionRow()
	row.SetTitle(msg)
	row.AddCSSClass("dim-label")
	sv.listBox.Append(row)
}

// buildStashRow creates an expandable row widget for a single stash entry.
// When expanded for the first time, it loads the changed files and their diffs.
func (sv *StashView) buildStashRow(stash git.StashInfo) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(stash.Message)
	row.SetSubtitle(fmt.Sprintf("stash@{%d}", stash.Index))
	row.SetIconName("sidebar-show-symbolic")

	idx := stash.Index
	var diffLoaded bool

	// Apply button.
	applyBtn := gtk.NewButtonWithLabel("Apply")
	applyBtn.SetTooltipText("Apply this stash without removing it")
	applyBtn.AddCSSClass("flat")
	applyBtn.SetVAlign(gtk.AlignCenter)
	applyBtn.ConnectClicked(func() {
		go func() {
			err := sv.repo.StashApply(idx)
			glib.IdleAdd(func() {
				if err != nil {
					slog.Warn("stash apply failed", "error", err)
				}
				sv.RefreshStashes()
			})
		}()
	})
	row.AddSuffix(applyBtn)

	// Pop button.
	popBtn := gtk.NewButtonWithLabel("Pop")
	popBtn.SetTooltipText("Apply and remove this stash")
	popBtn.AddCSSClass("flat")
	popBtn.AddCSSClass("suggested-action")
	popBtn.SetVAlign(gtk.AlignCenter)
	popBtn.ConnectClicked(func() {
		go func() {
			err := sv.repo.StashPop(idx)
			glib.IdleAdd(func() {
				if err != nil {
					slog.Warn("stash pop failed", "error", err)
				}
				sv.RefreshStashes()
			})
		}()
	})
	row.AddSuffix(popBtn)

	// Drop button — asks for confirmation before discarding.
	dropBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
	dropBtn.SetTooltipText("Discard this stash")
	dropBtn.AddCSSClass("flat")
	dropBtn.AddCSSClass("error")
	dropBtn.SetVAlign(gtk.AlignCenter)
	dropBtn.ConnectClicked(func() {
		sv.confirmDrop(idx, stash.Message)
	})
	row.AddSuffix(dropBtn)

	// Lazy-load diffs when the row is first expanded.
	row.ConnectActivated(func() {
		if diffLoaded || !row.Expanded() {
			return
		}
		diffLoaded = true
		sv.loadStashDiff(row, idx)
	})

	return row
}

// loadStashDiff loads the diff for a stash entry and populates child rows.
func (sv *StashView) loadStashDiff(row *adw.ExpanderRow, index int) {
	go func() {
		files, err := sv.repo.StashShow(index)
		glib.IdleAdd(func() {
			if err != nil {
				errRow := adw.NewActionRow()
				errRow.SetTitle("Could not load diff: " + err.Error())
				errRow.AddCSSClass("error")
				row.AddRow(errRow)
				return
			}
			if len(files) == 0 {
				emptyRow := adw.NewActionRow()
				emptyRow.SetTitle("No changes")
				emptyRow.AddCSSClass("dim-label")
				row.AddRow(emptyRow)
				return
			}
			for _, f := range files {
				fileExpander := adw.NewExpanderRow()
				fileExpander.SetTitle(f.Path)
				fileExpander.SetIconName("text-x-generic-symbolic")

				if f.Diff != "" {
					diffLabel := gtk.NewLabel(f.Diff)
					diffLabel.AddCSSClass("monospace")
					diffLabel.SetXAlign(0)
					diffLabel.SetSelectable(true)
					diffLabel.SetWrap(false)
					diffLabel.SetMarginTop(6)
					diffLabel.SetMarginBottom(6)
					diffLabel.SetMarginStart(12)
					diffLabel.SetMarginEnd(12)

					diffRow := adw.NewActionRow()
					diffRow.SetChild(diffLabel)
					fileExpander.AddRow(diffRow)
				}

				row.AddRow(fileExpander)
			}
		})
	}()
}

// confirmDrop shows a confirmation dialog before dropping a single stash entry.
func (sv *StashView) confirmDrop(index int, message string) {
	title := fmt.Sprintf("Drop stash@{%d}?", index)
	body := fmt.Sprintf("'%s' will be permanently discarded.", message)
	dialog := adw.NewAlertDialog(title, body)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("drop", "Drop")
	dialog.SetResponseAppearance("drop", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "drop" {
			return
		}
		go func() {
			err := sv.repo.StashDrop(index)
			glib.IdleAdd(func() {
				if err != nil {
					slog.Warn("stash drop failed", "error", err)
				}
				sv.RefreshStashes()
			})
		}()
	})
	dialog.Present(sv.parentWindow)
}
