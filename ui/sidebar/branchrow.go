// Package sidebar — branchrow.go provides custom row widgets for
// branches and tags in the sidebar branch tree.
//
// Each branch is displayed as an AdwActionRow inside an AdwExpanderRow.
// The current branch gets a checkmark icon suffix and accent styling.
// Non-current local branches get a single "..." menu button that opens
// a popover with Merge, Rebase, and Delete actions.
// Remote branches show the remote name as a prefix.
// Tags show whether they are annotated or lightweight.
package sidebar

import (
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// BranchActions groups the optional action callbacks for a branch row.
// Nil callbacks mean the action is not available for that branch.
type BranchActions struct {
	OnDelete func()
	OnMerge  func()
	OnRebase func()
}

// NewBranchRow creates an AdwActionRow for a branch in the sidebar.
//
// Parameters:
//   - branch: the branch information from the git backend.
//   - isCurrent: true if this is the currently checked-out branch.
//   - actions: optional callbacks; non-current local branches get a menu button.
func NewBranchRow(branch git.BranchInfo, isCurrent bool, actions BranchActions) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(branch.Name)

	// Show short hash as subtitle.
	if len(branch.Hash) >= 7 {
		row.SetSubtitle(branch.Hash[:7])
	}

	// Icon: local branches get a branch icon, remote branches get a network icon.
	if branch.IsRemote {
		row.SetIconName("network-server-symbolic")
	} else {
		row.SetIconName("vcs-branch-symbolic")
	}

	// Current branch indicator: checkmark suffix + accent class.
	if isCurrent {
		checkIcon := gtk.NewImageFromIconName("emblem-ok-symbolic")
		checkIcon.AddCSSClass("success")
		row.AddSuffix(checkIcon)
		row.AddCSSClass("accent")
	}

	// Actions menu — only for non-current local branches with at least one action.
	hasActions := actions.OnDelete != nil || actions.OnMerge != nil || actions.OnRebase != nil
	if hasActions {
		// Build a GMenu model with the available actions.
		menu := gio.NewMenu()
		if actions.OnMerge != nil {
			menu.Append("Merge into current", "branchrow.merge")
		}
		if actions.OnRebase != nil {
			menu.Append("Rebase onto current", "branchrow.rebase")
		}
		if actions.OnDelete != nil {
			menu.Append("Delete branch", "branchrow.delete")
		}

		// Register simple actions on an action group attached to the row.
		ag := gio.NewSimpleActionGroup()

		if actions.OnMerge != nil {
			mergeAction := gio.NewSimpleAction("merge", nil)
			mergeAction.ConnectActivate(func(param *glib.Variant) {
				actions.OnMerge()
			})
			ag.AddAction(mergeAction)
		}
		if actions.OnRebase != nil {
			rebaseAction := gio.NewSimpleAction("rebase", nil)
			rebaseAction.ConnectActivate(func(param *glib.Variant) {
				actions.OnRebase()
			})
			ag.AddAction(rebaseAction)
		}
		if actions.OnDelete != nil {
			deleteAction := gio.NewSimpleAction("delete", nil)
			deleteAction.ConnectActivate(func(param *glib.Variant) {
				actions.OnDelete()
			})
			ag.AddAction(deleteAction)
		}

		row.InsertActionGroup("branchrow", ag)

		// Menu button with a popover driven by the GMenu model.
		menuBtn := gtk.NewMenuButton()
		menuBtn.SetIconName("view-more-symbolic")
		menuBtn.SetMenuModel(menu)
		menuBtn.SetTooltipText("Branch actions")
		menuBtn.AddCSSClass("flat")
		menuBtn.SetVAlign(gtk.AlignCenter)
		// Prevent row activation when the menu button is clicked.
		menuBtn.SetFocusOnClick(false)

		row.AddSuffix(menuBtn)
	}

	// Make the row activatable so clicking it triggers checkout.
	row.SetActivatable(true)

	return row
}

// NewTagRow creates an AdwActionRow for a tag in the sidebar.
//
// Parameters:
//   - tag: the tag information from the git backend.
//   - onDelete: if non-nil, a delete button is shown for the tag.
func NewTagRow(tag git.TagInfo, onDelete func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(tag.Name)
	row.SetIconName("tag-symbolic")

	// Show tag type and hash.
	subtitle := ""
	if tag.IsAnnotated {
		subtitle = "annotated"
	} else {
		subtitle = "lightweight"
	}
	if len(tag.Hash) >= 7 {
		subtitle += " \u00b7 " + tag.Hash[:7]
	}
	row.SetSubtitle(subtitle)

	// Delete button.
	if onDelete != nil {
		deleteBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
		deleteBtn.SetTooltipText("Delete tag")
		deleteBtn.AddCSSClass("flat")
		deleteBtn.AddCSSClass("error")
		deleteBtn.SetVAlign(gtk.AlignCenter)
		deleteBtn.ConnectClicked(func() { onDelete() })
		row.AddSuffix(deleteBtn)
	}

	row.SetActivatable(false)

	return row
}
