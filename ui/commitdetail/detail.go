// Package commitdetail implements the commit detail view — the right
// panel that shows full metadata and file changes for a selected commit.
//
// When the user clicks a commit in the log table, this view shows:
//   - Full commit hash (copyable).
//   - Author name + email, committer name + email.
//   - Authored and committed dates.
//   - Full commit message.
//   - List of files changed with expandable inline diffs.
//
// Clicking a file expands/collapses the inline diff for that file,
// similar to how branches are shown in expandable rows in the sidebar.
//
// Widget hierarchy:
//
//	GtkScrolledWindow
//	  └─ GtkBox (vertical)
//	       ├─ AdwPreferencesGroup "Commit Info"
//	       │    ├─ AdwActionRow "Hash"
//	       │    ├─ AdwActionRow "Author"
//	       │    └─ AdwActionRow "Date"
//	       ├─ AdwPreferencesGroup "Message"
//	       │    └─ GtkLabel (full message)
//	       └─ AdwPreferencesGroup "Changed Files"
//	            └─ GtkListBox (expandable file rows with inline diffs)
package commitdetail

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// CommitDetail is the commit detail view widget.
type CommitDetail struct {
	// Root is the top-level widget to embed in the content area.
	Root *gtk.ScrolledWindow

	// repo is the current repository.
	repo *git.Repository

	// cfg holds the app configuration.
	cfg *config.Config

	// commit is the currently displayed commit.
	commit *git.CommitInfo

	// hashRow shows the full commit hash.
	hashRow *adw.ActionRow

	// authorRow shows the author name and email.
	authorRow *adw.ActionRow

	// dateRow shows the commit date.
	dateRow *adw.ActionRow

	// messageLabel shows the full commit message.
	messageLabel *gtk.Label

	// filesListBox shows the list of changed files with expandable diffs.
	filesListBox *gtk.ListBox

	// infoGroup is the commit info section.
	infoGroup *adw.PreferencesGroup

	// filesGroup is the changed files section.
	filesGroup *adw.PreferencesGroup

	// actionsBtn is the menu button for commit actions (tag creation, etc.).
	actionsBtn *gtk.MenuButton

	// contentBox is the main vertical layout box.
	contentBox *gtk.Box

	// expandedFiles tracks which files have their diff expanded.
	expandedFiles map[string]bool

	// fileDiffs stores the computed diffs keyed by file path.
	fileDiffs map[string]git.DiffResult
}

// New creates a new CommitDetail widget.
//
// Parameters:
//   - cfg: the app configuration.
func New(cfg *config.Config) *CommitDetail {
	cd := &CommitDetail{
		cfg:           cfg,
		expandedFiles: make(map[string]bool),
		fileDiffs:     make(map[string]git.DiffResult),
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

	// Actions menu button in the header area — for tag creation etc.
	actionsMenu := gio.NewMenu()
	actionsMenu.Append("Create Tag on this Commit…", "detail.create-tag")

	cd.actionsBtn = gtk.NewMenuButton()
	cd.actionsBtn.SetIconName("view-more-symbolic")
	cd.actionsBtn.SetMenuModel(actionsMenu)
	cd.actionsBtn.SetTooltipText("Actions")
	cd.actionsBtn.AddCSSClass("flat")
	cd.actionsBtn.SetVAlign(gtk.AlignCenter)
	cd.actionsBtn.SetVisible(false) // Hidden until a commit is selected.
	cd.infoGroup.SetHeaderSuffix(cd.actionsBtn)

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

	// Author row — subtitle style, matching hash and date rows.
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
	cd.filesListBox.SetSelectionMode(gtk.SelectionNone)
	cd.filesListBox.AddCSSClass("boxed-list")
	cd.filesGroup.Add(cd.filesListBox)

	cd.contentBox.Append(cd.filesGroup)

	// Scrolled window.
	cd.Root = gtk.NewScrolledWindow()
	cd.Root.SetChild(cd.contentBox)
	cd.Root.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Register the "detail.create-tag" action on the scrolled window.
	tagAction := gio.NewSimpleAction("create-tag", nil)
	tagAction.ConnectActivate(func(param *glib.Variant) {
		if cd.commit != nil {
			// Activate the window-level create-tag action with the commit hash.
			cd.Root.ActivateAction("win.create-tag", glib.NewVariantString(cd.commit.Hash))
		}
	})
	actionGroup := gio.NewSimpleActionGroup()
	actionGroup.AddAction(tagAction)
	cd.Root.InsertActionGroup("detail", actionGroup)
}

// SetRepository sets the current repository for diff computation.
func (cd *CommitDetail) SetRepository(repo *git.Repository) {
	cd.repo = repo
}

// SetCommit loads and displays the details of a commit.
func (cd *CommitDetail) SetCommit(commit git.CommitInfo) {
	cd.commit = &commit
	cd.expandedFiles = make(map[string]bool)
	cd.fileDiffs = make(map[string]git.DiffResult)

	// Update info rows.
	cd.hashRow.SetSubtitle(commit.Hash)

	// Set author using subtitle (same style as hash and date).
	authorText := fmt.Sprintf("%s <%s>", commit.Author, commit.AuthorEmail)
	cd.authorRow.SetSubtitle(authorText)

	cd.dateRow.SetSubtitle(commit.AuthorTime.Format("2006-01-02 15:04:05 -0700"))
	cd.messageLabel.SetText(commit.Body)

	// Show the actions button.
	cd.actionsBtn.SetVisible(true)

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

	// Store diffs for inline expansion.
	for _, diff := range diffs {
		cd.fileDiffs[diff.NewPath] = diff
	}

	// Update the section title with count.
	cd.filesGroup.SetTitle(fmt.Sprintf("Changed Files (%d)", len(diffs)))

	// Add a row for each changed file with expandable inline diff.
	for _, diff := range diffs {
		fileBox := cd.createExpandableFileRow(diff)
		cd.filesListBox.Append(fileBox)
	}
}

// createExpandableFileRow creates a row for a changed file. Clicking the
// row toggles an inline diff display below the file name, similar to how
// branches are shown in expandable sidebar rows.
func (cd *CommitDetail) createExpandableFileRow(diff git.DiffResult) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetActivatable(true)

	outerBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// --- File header row ---
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	headerBox.SetMarginTop(8)
	headerBox.SetMarginBottom(8)
	headerBox.SetMarginStart(12)
	headerBox.SetMarginEnd(12)

	// Expand/collapse arrow.
	arrow := gtk.NewImageFromIconName("go-next-symbolic")
	arrow.SetVAlign(gtk.AlignCenter)
	headerBox.Append(arrow)

	// Change type icon.
	var iconName string
	switch diff.ChangeType {
	case git.ChangeAdded:
		iconName = "list-add-symbolic"
	case git.ChangeDeleted:
		iconName = "list-remove-symbolic"
	case git.ChangeModified:
		iconName = "document-edit-symbolic"
	case git.ChangeRenamed:
		iconName = "edit-find-replace-symbolic"
	default:
		iconName = "document-edit-symbolic"
	}
	icon := gtk.NewImageFromIconName(iconName)
	icon.SetVAlign(gtk.AlignCenter)
	headerBox.Append(icon)

	// File name.
	nameLabel := gtk.NewLabel(diff.NewPath)
	nameLabel.SetXAlign(0)
	nameLabel.SetEllipsize(3) // PANGO_ELLIPSIZE_END
	nameLabel.SetHExpand(true)
	headerBox.Append(nameLabel)

	// Stats.
	statsLabel := gtk.NewLabel(fmt.Sprintf("+%d -%d", diff.Stats.Additions, diff.Stats.Deletions))
	statsLabel.AddCSSClass("dim-label")
	statsLabel.AddCSSClass("caption")
	statsLabel.SetVAlign(gtk.AlignCenter)
	headerBox.Append(statsLabel)

	outerBox.Append(headerBox)

	// --- Inline diff content (hidden by default) ---
	diffBox := gtk.NewBox(gtk.OrientationVertical, 0)
	diffBox.SetVisible(false)
	diffBox.SetMarginStart(12)
	diffBox.SetMarginEnd(12)
	diffBox.SetMarginBottom(8)

	// Build the diff text view.
	diffWidget := cd.buildInlineDiff(diff)
	diffBox.Append(diffWidget)

	outerBox.Append(diffBox)

	// Toggle expand/collapse on row activation.
	filePath := diff.NewPath
	row.SetChild(outerBox)

	// Use a click gesture on the header to toggle.
	gesture := gtk.NewGestureClick()
	gesture.ConnectReleased(func(nPress int, x, y float64) {
		expanded := cd.expandedFiles[filePath]
		cd.expandedFiles[filePath] = !expanded

		if !expanded {
			arrow.SetFromIconName("go-down-symbolic")
			diffBox.SetVisible(true)
		} else {
			arrow.SetFromIconName("go-next-symbolic")
			diffBox.SetVisible(false)
		}
	})
	headerBox.AddController(gesture)

	return row
}

// buildInlineDiff creates a GtkTextView showing the diff for a file.
func (cd *CommitDetail) buildInlineDiff(diff git.DiffResult) *gtk.Frame {
	frame := gtk.NewFrame("")

	buffer := gtk.NewTextBuffer(nil)
	tagTable := buffer.TagTable()

	addedTag := gtk.NewTextTag("added")
	addedTag.SetObjectProperty("foreground", "#26a269")
	addedTag.SetObjectProperty("background", "#26a26920")
	tagTable.Add(addedTag)

	deletedTag := gtk.NewTextTag("deleted")
	deletedTag.SetObjectProperty("foreground", "#e01b24")
	deletedTag.SetObjectProperty("background", "#e01b2420")
	tagTable.Add(deletedTag)

	hunkTag := gtk.NewTextTag("hunk")
	hunkTag.SetObjectProperty("foreground", "#1c71d8")
	tagTable.Add(hunkTag)

	linenoTag := gtk.NewTextTag("lineno")
	linenoTag.SetObjectProperty("foreground", "#77767b")
	tagTable.Add(linenoTag)

	// Render hunks.
	for _, hunk := range diff.Hunks {
		// Hunk header.
		insertWithTag(buffer, hunk.Header+"\n", "hunk")

		for _, line := range hunk.Lines {
			prefix := " "
			tag := ""

			switch line.Type {
			case git.DiffLineContext:
				prefix = " "
			case git.DiffLineAdd:
				prefix = "+"
				tag = "added"
			case git.DiffLineDelete:
				prefix = "-"
				tag = "deleted"
			}

			// Line numbers.
			linenoStr := formatLineNumbers(line)
			insertWithTag(buffer, linenoStr, "lineno")

			text := prefix + line.Content + "\n"
			if tag != "" {
				insertWithTag(buffer, text, tag)
			} else {
				insertPlain(buffer, text)
			}
		}

		insertPlain(buffer, "\n")
	}

	if len(diff.Hunks) == 0 {
		if diff.IsBinary {
			insertPlain(buffer, "Binary file — cannot display diff.\n")
		} else {
			insertPlain(buffer, "No changes to display.\n")
		}
	}

	textView := gtk.NewTextViewWithBuffer(buffer)
	textView.SetEditable(false)
	textView.SetCursorVisible(false)
	textView.SetMonospace(true)
	textView.SetWrapMode(gtk.WrapNone)
	textView.SetTopMargin(6)
	textView.SetBottomMargin(6)
	textView.SetLeftMargin(6)
	textView.SetRightMargin(6)

	frame.SetChild(textView)
	return frame
}

// insertWithTag appends text to the buffer with a named tag.
func insertWithTag(buffer *gtk.TextBuffer, text, tagName string) {
	endIter := buffer.EndIter()
	offset := endIter.Offset()
	buffer.Insert(endIter, text)
	startIter := buffer.IterAtOffset(offset)
	newEndIter := buffer.EndIter()
	buffer.ApplyTagByName(tagName, startIter, newEndIter)
}

// insertPlain appends untagged text to the buffer.
func insertPlain(buffer *gtk.TextBuffer, text string) {
	endIter := buffer.EndIter()
	buffer.Insert(endIter, text)
}

// formatLineNumbers formats the line numbers for a diff line.
func formatLineNumbers(line git.DiffLine) string {
	old := "    "
	new := "    "

	if line.OldLineNo > 0 {
		old = fmt.Sprintf("%4d", line.OldLineNo)
	}
	if line.NewLineNo > 0 {
		new = fmt.Sprintf("%4d", line.NewLineNo)
	}

	return old + " " + new + " "
}

// formatFileHeader creates the file header line for a diff.
func formatFileHeader(diff git.DiffResult) string {
	var b strings.Builder
	b.WriteString("--- ")

	switch diff.ChangeType {
	case git.ChangeAdded:
		b.WriteString("/dev/null")
	default:
		b.WriteString("a/" + diff.OldPath)
	}

	b.WriteString("\n+++ ")

	switch diff.ChangeType {
	case git.ChangeDeleted:
		b.WriteString("/dev/null")
	default:
		b.WriteString("b/" + diff.NewPath)
	}

	return b.String()
}

// Clear clears the detail view (e.g., when no commit is selected).
func (cd *CommitDetail) Clear() {
	cd.commit = nil
	cd.actionsBtn.SetVisible(false)
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
