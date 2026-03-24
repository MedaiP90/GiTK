// Package commitdetail implements the commit detail view — the right
// panel that shows full metadata and file changes for a selected commit.
//
// When the user clicks a commit in the log table, this view shows:
//   - Full commit hash (copyable).
//   - Author name + email, committer name + email.
//   - Authored and committed dates.
//   - Full commit message.
//   - List of files changed with change type icons and +N/-M stats.
//
// Clicking a file in the list shows the diff for that file.
//
// Widget hierarchy:
//
//	GtkScrolledWindow
//	  └─ GtkBox (vertical)
//	       ├─ AdwPreferencesGroup "Commit Info"
//	       │    ├─ AdwActionRow "Hash"
//	       │    ├─ AdwActionRow "Author"
//	       │    ├─ AdwActionRow "Date"
//	       │    └─ GtkLabel (full message)
//	       └─ AdwPreferencesGroup "Changed Files"
//	            └─ GtkListBox (file rows)
package commitdetail

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnFileSelected is called when the user clicks a file in the changed files list.
type OnFileSelected func(diff git.DiffResult)

// CommitDetail is the commit detail view widget.
type CommitDetail struct {
	// Root is the top-level widget to embed in the content area.
	Root *gtk.ScrolledWindow

	// repo is the current repository.
	repo *git.Repository

	// commit is the currently displayed commit.
	commit *git.CommitInfo

	// onFileSelected is called when a file row is clicked.
	onFileSelected OnFileSelected

	// hashRow shows the full commit hash.
	hashRow *adw.ActionRow

	// authorRow shows the author name and email.
	authorRow *adw.ActionRow

	// dateRow shows the commit date.
	dateRow *adw.ActionRow

	// messageLabel shows the full commit message.
	messageLabel *gtk.Label

	// filesListBox shows the list of changed files.
	filesListBox *gtk.ListBox

	// infoGroup is the commit info section.
	infoGroup *adw.PreferencesGroup

	// filesGroup is the changed files section.
	filesGroup *adw.PreferencesGroup

	// contentBox is the main vertical layout box.
	contentBox *gtk.Box
}

// New creates a new CommitDetail widget.
//
// Parameters:
//   - onFileSelected: callback when a file row is clicked.
func New(onFileSelected OnFileSelected) *CommitDetail {
	cd := &CommitDetail{
		onFileSelected: onFileSelected,
	}

	cd.build()
	return cd
}

// build constructs all the detail view widgets.
func (cd *CommitDetail) build() {
	cd.contentBox = gtk.NewBox(gtk.OrientationVertical, 18)
	cd.contentBox.SetMarginTop(18)
	cd.contentBox.SetMarginBottom(18)
	cd.contentBox.SetMarginStart(18)
	cd.contentBox.SetMarginEnd(18)

	// --- Commit Info Group ---
	cd.infoGroup = adw.NewPreferencesGroup()
	cd.infoGroup.SetTitle("Commit")

	// Hash row with copy button.
	cd.hashRow = adw.NewActionRow()
	cd.hashRow.SetTitle("Hash")
	copyBtn := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyBtn.SetTooltipText("Copy hash")
	copyBtn.SetVAlign(gtk.AlignCenter)
	copyBtn.ConnectClicked(func() {
		if cd.commit != nil {
			display := gdk.DisplayGetDefault()
			clipboard := display.Clipboard()
			clipboard.SetText(cd.commit.Hash)
		}
	})
	cd.hashRow.AddSuffix(copyBtn)
	cd.hashRow.SetActivatable(false)
	cd.infoGroup.Add(cd.hashRow)

	// Author row.
	cd.authorRow = adw.NewActionRow()
	cd.authorRow.SetTitle("Author")
	cd.authorRow.SetActivatable(false)
	cd.infoGroup.Add(cd.authorRow)

	// Date row.
	cd.dateRow = adw.NewActionRow()
	cd.dateRow.SetTitle("Date")
	cd.dateRow.SetActivatable(false)
	cd.infoGroup.Add(cd.dateRow)

	cd.contentBox.Append(cd.infoGroup)

	// --- Commit message ---
	msgGroup := adw.NewPreferencesGroup()
	msgGroup.SetTitle("Message")

	cd.messageLabel = gtk.NewLabel("")
	cd.messageLabel.SetXAlign(0)
	cd.messageLabel.SetWrap(true)
	cd.messageLabel.SetSelectable(true)
	cd.messageLabel.AddCSSClass("monospace")
	cd.messageLabel.SetMarginTop(6)
	cd.messageLabel.SetMarginBottom(6)
	cd.messageLabel.SetMarginStart(12)
	cd.messageLabel.SetMarginEnd(12)
	msgGroup.Add(cd.messageLabel)
	cd.contentBox.Append(msgGroup)

	// --- Changed Files Group ---
	cd.filesGroup = adw.NewPreferencesGroup()
	cd.filesGroup.SetTitle("Changed Files")

	cd.filesListBox = gtk.NewListBox()
	cd.filesListBox.SetSelectionMode(gtk.SelectionSingle)
	cd.filesListBox.AddCSSClass("boxed-list")
	cd.filesGroup.Add(cd.filesListBox)

	cd.contentBox.Append(cd.filesGroup)

	// Scrolled window.
	cd.Root = gtk.NewScrolledWindow()
	cd.Root.SetChild(cd.contentBox)
	cd.Root.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
}

// SetRepository sets the current repository for diff computation.
func (cd *CommitDetail) SetRepository(repo *git.Repository) {
	cd.repo = repo
}

// SetCommit loads and displays the details of a commit.
func (cd *CommitDetail) SetCommit(commit git.CommitInfo) {
	cd.commit = &commit

	// Update info rows.
	cd.hashRow.SetSubtitle(commit.Hash)
	cd.authorRow.SetSubtitle(fmt.Sprintf("%s <%s>", commit.Author, commit.AuthorEmail))
	cd.dateRow.SetSubtitle(commit.AuthorTime.Format("2006-01-02 15:04:05 -0700"))
	cd.messageLabel.SetText(commit.Body)

	// Update changed files.
	cd.loadChangedFiles(commit)
}

// loadChangedFiles computes and displays the diff for the given commit.
func (cd *CommitDetail) loadChangedFiles(commit git.CommitInfo) {
	// Clear existing file rows.
	for {
		row := cd.filesListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		cd.filesListBox.Remove(row)
	}

	if cd.repo == nil {
		return
	}

	// Compute the diff for this commit.
	diffs, err := cd.repo.DiffCommit(commit.Hash)
	if err != nil {
		slog.Warn("failed to compute diff", "hash", commit.ShortHash, "error", err)
		return
	}

	// Update the section title with count.
	cd.filesGroup.SetTitle(fmt.Sprintf("Changed Files (%d)", len(diffs)))

	// Add a row for each changed file.
	for _, diff := range diffs {
		row := cd.createFileRow(diff)
		cd.filesListBox.Append(row)
	}
}

// createFileRow creates an AdwActionRow for a changed file.
func (cd *CommitDetail) createFileRow(diff git.DiffResult) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(diff.NewPath)
	row.SetActivatable(true)

	// Icon based on change type.
	switch diff.ChangeType {
	case git.ChangeAdded:
		row.SetIconName("list-add-symbolic")
	case git.ChangeDeleted:
		row.SetIconName("list-remove-symbolic")
	case git.ChangeModified:
		row.SetIconName("document-edit-symbolic")
	case git.ChangeRenamed:
		row.SetIconName("edit-find-replace-symbolic")
	}

	// Show +N -M stats as subtitle.
	statsText := fmt.Sprintf("+%d -%d", diff.Stats.Additions, diff.Stats.Deletions)
	row.SetSubtitle(statsText)

	// Connect activation to show the diff.
	row.ConnectActivated(func() {
		if cd.onFileSelected != nil {
			cd.onFileSelected(diff)
		}
	})

	return row
}

// Clear clears the detail view (e.g., when no commit is selected).
func (cd *CommitDetail) Clear() {
	cd.commit = nil
	cd.hashRow.SetSubtitle("")
	cd.authorRow.SetSubtitle("")
	cd.dateRow.SetSubtitle("")
	cd.messageLabel.SetText("")

	// Clear file list.
	for {
		row := cd.filesListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		cd.filesListBox.Remove(row)
	}
}
