// Package stash implements the stash management view for GiTK.
//
// The stash view shows a list of all stash entries for the current repository.
// For each stash entry the user can:
//   - Apply  — restores the stash without removing it.
//   - Pop    — restores the stash and removes it from the list.
//   - Drop   — discards the stash without applying it.
//
// A "Stash Changes" button at the top opens the stash creation dialog.
package stash

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnStashAction is a callback for stash button actions.
type OnStashAction func()

// StashView is the stash management page widget.
type StashView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *gtk.Box

	// repo is the currently open repository.
	repo *git.Repository

	// listBox holds the stash entry rows.
	listBox *gtk.ListBox

	// onNewStash is called when the user clicks "Stash Changes".
	onNewStash OnStashAction
}

// New creates a new StashView.
//
// Parameters:
//   - onNewStash: called when the user clicks the "Stash Changes" button.
func New(onNewStash OnStashAction) *StashView {
	sv := &StashView{onNewStash: onNewStash}
	sv.build()
	return sv
}

// build constructs the stash view widgets.
func (sv *StashView) build() {
	sv.Root = gtk.NewBox(gtk.OrientationVertical, 0)
	sv.Root.SetVExpand(true)

	// Top bar with "Stash Changes" button.
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

	newStashBtn := gtk.NewButtonWithLabel("Stash Changes")
	newStashBtn.AddCSSClass("suggested-action")
	newStashBtn.SetTooltipText("Save current changes to the stash")
	newStashBtn.ConnectClicked(func() {
		if sv.onNewStash != nil {
			sv.onNewStash()
		}
	})
	topBar.Append(newStashBtn)

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

// buildStashRow creates a row widget for a single stash entry.
func (sv *StashView) buildStashRow(stash git.StashInfo) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(stash.Message)
	row.SetSubtitle(fmt.Sprintf("stash@{%d}", stash.Index))
	row.SetIconName("sidebar-show-symbolic")

	idx := stash.Index

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

	// Drop button.
	dropBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
	dropBtn.SetTooltipText("Discard this stash")
	dropBtn.AddCSSClass("flat")
	dropBtn.AddCSSClass("error")
	dropBtn.SetVAlign(gtk.AlignCenter)
	dropBtn.ConnectClicked(func() {
		go func() {
			err := sv.repo.StashDrop(idx)
			glib.IdleAdd(func() {
				if err != nil {
					slog.Warn("stash drop failed", "error", err)
				}
				sv.RefreshStashes()
			})
		}()
	})
	row.AddSuffix(dropBtn)

	return row
}
