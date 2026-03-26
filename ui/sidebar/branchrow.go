// Package sidebar — branchrow.go provides custom row widgets for
// branches and tags in the sidebar branch tree.
//
// Each branch is displayed as an AdwActionRow inside an AdwExpanderRow.
// The current branch gets a checkmark icon suffix and bold styling.
// Remote branches show the remote name as a prefix.
// Tags show whether they are annotated or lightweight.
package sidebar

import (
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// NewBranchRow creates an AdwActionRow for a branch in the sidebar.
//
// Parameters:
//   - branch: the branch information from the git backend.
//   - isCurrent: true if this is the currently checked-out branch.
//   - onDelete: if non-nil, a delete button is shown (non-current local branches only).
//   - onMerge: if non-nil, a merge button is shown (non-current local branches only).
func NewBranchRow(branch git.BranchInfo, isCurrent bool, onDelete func(), onMerge func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(branch.Name)

	// Show short hash as subtitle.
	if len(branch.Hash) >= 7 {
		row.SetSubtitle(branch.Hash[:7])
	}

	// Icon: local branches get a branch icon, remote branches get a
	// network icon.
	if branch.IsRemote {
		row.SetIconName("network-server-symbolic")
	} else {
		row.SetIconName("vcs-branch-symbolic")
	}

	// Current branch indicator: add a checkmark suffix icon.
	if isCurrent {
		checkIcon := gtk.NewImageFromIconName("emblem-ok-symbolic")
		checkIcon.AddCSSClass("success")
		row.AddSuffix(checkIcon)

		// Bold the current branch name.
		row.AddCSSClass("accent")
	}

	// Merge button — only for non-current local branches.
	if onMerge != nil {
		mergeBtn := gtk.NewButtonFromIconName("vcs-merge-symbolic")
		mergeBtn.SetTooltipText("Merge into current branch")
		mergeBtn.AddCSSClass("flat")
		mergeBtn.SetVAlign(gtk.AlignCenter)
		mergeBtn.ConnectClicked(func() { onMerge() })
		row.AddSuffix(mergeBtn)
	}

	// Delete button — only for non-current local branches.
	if onDelete != nil {
		deleteBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
		deleteBtn.SetTooltipText("Delete branch")
		deleteBtn.AddCSSClass("flat")
		deleteBtn.AddCSSClass("error")
		deleteBtn.SetVAlign(gtk.AlignCenter)
		deleteBtn.ConnectClicked(func() { onDelete() })
		row.AddSuffix(deleteBtn)
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
		subtitle += " · " + tag.Hash[:7]
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
