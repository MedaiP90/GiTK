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
//
// The row displays:
//   - Icon: branch-symbolic for local, network-symbolic for remote.
//   - Title: the branch short name.
//   - Subtitle: the short commit hash.
//   - Suffix: checkmark icon if this is the current branch.
func NewBranchRow(branch git.BranchInfo, isCurrent bool) *adw.ActionRow {
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

	// Make the row activatable so clicking it triggers checkout.
	row.SetActivatable(true)

	return row
}

// NewTagRow creates an AdwActionRow for a tag in the sidebar.
//
// Parameters:
//   - tag: the tag information from the git backend.
//
// The row displays:
//   - Icon: tag-symbolic.
//   - Title: the tag name.
//   - Subtitle: "annotated" or "lightweight" + short hash.
func NewTagRow(tag git.TagInfo) *adw.ActionRow {
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

	row.SetActivatable(true)

	return row
}
