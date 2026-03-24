// Package commitdetail — diffview.go implements an inline diff viewer.
//
// When the user clicks a file in the commit detail view, this widget
// shows the unified diff with:
//   - File header (old path → new path, change type).
//   - Hunk headers (@@ -10,5 +10,7 @@).
//   - Context lines (dim text).
//   - Added lines (green background).
//   - Deleted lines (red background).
//   - Line numbers for both old and new versions.
//
// The diff is rendered as a GtkTextView with tags for coloring.
// For a more advanced implementation, we could use GtkSourceView with
// syntax highlighting, but a plain GtkTextView works well for diffs.
//
// Widget hierarchy:
//
//	AdwToolbarView
//	  ├─ [top] AdwHeaderBar (file path + close button)
//	  └─ [content] GtkScrolledWindow
//	       └─ GtkTextView (with colored diff text)
package commitdetail

import (
	"fmt"
	"strings"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// DiffView displays a unified diff for a single file.
type DiffView struct {
	// Root is the top-level widget.
	Root *adw.ToolbarView

	// textView is the GtkTextView showing the diff.
	textView *gtk.TextView

	// buffer is the GtkTextBuffer containing the diff text.
	buffer *gtk.TextBuffer

	// titleLabel shows the file path in the header bar.
	titleLabel *gtk.Label
}

// NewDiffView creates a new diff viewer.
func NewDiffView() *DiffView {
	dv := &DiffView{}
	dv.build()
	return dv
}

// build constructs the diff view widgets.
func (dv *DiffView) build() {
	// --- Header bar ---
	header := adw.NewHeaderBar()
	header.SetShowBackButton(true)

	dv.titleLabel = gtk.NewLabel("")
	dv.titleLabel.AddCSSClass("heading")
	header.SetTitleWidget(dv.titleLabel)

	// --- Text view ---
	dv.buffer = gtk.NewTextBuffer(nil)
	dv.textView = gtk.NewTextViewWithBuffer(dv.buffer)
	dv.textView.SetEditable(false)
	dv.textView.SetCursorVisible(false)
	dv.textView.SetMonospace(true)
	dv.textView.SetWrapMode(gtk.WrapNone)
	dv.textView.SetTopMargin(6)
	dv.textView.SetBottomMargin(6)
	dv.textView.SetLeftMargin(12)
	dv.textView.SetRightMargin(12)

	// Create text tags for diff coloring.
	// Tags are named formatting rules that can be applied to ranges of text.
	tagTable := dv.buffer.TagTable()

	addedTag := gtk.NewTextTag("added")
	addedTag.SetObjectProperty("foreground", "#26a269")     // GNOME green
	addedTag.SetObjectProperty("background", "#26a26920")   // Transparent green
	tagTable.Add(addedTag)

	deletedTag := gtk.NewTextTag("deleted")
	deletedTag.SetObjectProperty("foreground", "#e01b24")     // GNOME red
	deletedTag.SetObjectProperty("background", "#e01b2420")   // Transparent red
	tagTable.Add(deletedTag)

	hunkTag := gtk.NewTextTag("hunk")
	hunkTag.SetObjectProperty("foreground", "#1c71d8") // GNOME blue
	tagTable.Add(hunkTag)

	linenoTag := gtk.NewTextTag("lineno")
	linenoTag.SetObjectProperty("foreground", "#77767b") // GNOME grey
	tagTable.Add(linenoTag)

	// Scrolled window.
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(dv.textView)
	scrolled.SetVExpand(true)

	// Assemble.
	dv.Root = adw.NewToolbarView()
	dv.Root.AddTopBar(header)
	dv.Root.SetContent(scrolled)
}

// SetDiff loads and displays a diff result.
func (dv *DiffView) SetDiff(diff git.DiffResult) {
	// Update title.
	title := diff.NewPath
	if diff.ChangeType == git.ChangeRenamed && diff.OldPath != diff.NewPath {
		title = diff.OldPath + " → " + diff.NewPath
	}
	dv.titleLabel.SetLabel(title)

	// Build the diff text with tags.
	dv.renderDiff(diff)
}

// renderDiff renders the diff hunks into the text buffer with tags.
func (dv *DiffView) renderDiff(diff git.DiffResult) {
	// Clear the buffer.
	dv.buffer.SetText("")

	// File header.
	header := formatFileHeader(diff)
	dv.insertWithTag(header+"\n", "hunk")

	// Render each hunk.
	for _, hunk := range diff.Hunks {
		// Hunk header.
		dv.insertWithTag(hunk.Header+"\n", "hunk")

		// Hunk lines.
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

			// Format: old_lineno new_lineno prefix content
			linenoStr := formatLineNumbers(line)
			dv.insertWithTag(linenoStr, "lineno")

			text := prefix + line.Content + "\n"
			if tag != "" {
				dv.insertWithTag(text, tag)
			} else {
				dv.insertPlain(text)
			}
		}

		dv.insertPlain("\n")
	}

	// If no hunks, show a message.
	if len(diff.Hunks) == 0 {
		if diff.IsBinary {
			dv.insertPlain("Binary file — cannot display diff.\n")
		} else {
			dv.insertPlain("No changes to display.\n")
		}
	}
}

// insertWithTag appends text to the buffer with a named tag.
func (dv *DiffView) insertWithTag(text, tagName string) {
	endIter := dv.buffer.EndIter()
	offset := endIter.Offset()
	dv.buffer.Insert(endIter, text)
	startIter := dv.buffer.IterAtOffset(offset)
	newEndIter := dv.buffer.EndIter()
	dv.buffer.ApplyTagByName(tagName, startIter, newEndIter)
}

// insertPlain appends untagged text to the buffer.
func (dv *DiffView) insertPlain(text string) {
	endIter := dv.buffer.EndIter()
	dv.buffer.Insert(endIter, text)
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

// formatLineNumbers formats the line numbers for a diff line.
// Format: "  old_no  new_no  " (right-aligned in 4-char columns).
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
