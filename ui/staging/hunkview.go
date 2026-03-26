// Package staging — hunkview.go implements the hunk-level staging widget.
//
// When the user selects a file in the staging area, the HunkView shows
// each hunk (contiguous block of changes) as a card with:
//   - The hunk header (@@ -10,5 +10,7 @@).
//   - The diff lines (context, added, deleted).
//   - A "Stage Hunk" or "Unstage Hunk" button.
//
// This allows fine-grained staging: the user can stage individual hunks
// within a file rather than the entire file.
//
// Widget hierarchy:
//
//	GtkScrolledWindow
//	  └─ GtkBox (vertical, one card per hunk)
//	       ├─ HunkCard 1
//	       │    ├─ Header: "@@ -10,5 +10,7 @@"  [Stage Hunk]
//	       │    └─ GtkTextView (diff lines)
//	       ├─ HunkCard 2
//	       │    └─ ...
//	       └─ ...
package staging

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// HunkView displays file hunks with per-hunk staging actions.
type HunkView struct {
	// Root is the top-level widget.
	Root *gtk.ScrolledWindow

	// contentBox holds the hunk cards.
	contentBox *gtk.Box

	// staging is a back-reference to the parent StagingView.
	staging *StagingView

	// currentPath is the path of the currently displayed file.
	currentPath string

	// isStaged indicates whether we're viewing a staged or unstaged file.
	isStaged bool
}

// NewHunkView creates a new hunk-level staging view.
func NewHunkView(staging *StagingView) *HunkView {
	hv := &HunkView{
		staging: staging,
	}

	hv.contentBox = gtk.NewBox(gtk.OrientationVertical, 12)
	hv.contentBox.SetMarginTop(12)
	hv.contentBox.SetMarginBottom(12)
	hv.contentBox.SetMarginStart(12)
	hv.contentBox.SetMarginEnd(12)

	// Placeholder.
	placeholder := gtk.NewLabel("Select a file to view its changes")
	placeholder.AddCSSClass("dim-label")
	hv.contentBox.Append(placeholder)

	hv.Root = gtk.NewScrolledWindow()
	hv.Root.SetChild(hv.contentBox)
	hv.Root.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	return hv
}

// SetFile loads and displays the hunks for a file.
//
// Parameters:
//   - path: the file path relative to the repository root.
//   - diff: the diff result containing the hunks.
//   - isStaged: true if this is a staged file (for the action button label).
func (hv *HunkView) SetFile(path string, diff git.DiffResult, isStaged bool) {
	hv.currentPath = path
	hv.isStaged = isStaged

	// Clear existing cards.
	for child := hv.contentBox.FirstChild(); child != nil; child = hv.contentBox.FirstChild() {
		hv.contentBox.Remove(child)
	}

	if len(diff.Hunks) == 0 {
		label := gtk.NewLabel("No changes to display")
		label.AddCSSClass("dim-label")
		hv.contentBox.Append(label)
		return
	}

	// File header.
	headerLabel := gtk.NewLabel(path)
	headerLabel.SetXAlign(0)
	headerLabel.AddCSSClass("title-4")
	headerLabel.SetMarginBottom(6)
	hv.contentBox.Append(headerLabel)

	// Stats.
	statsLabel := gtk.NewLabel(fmt.Sprintf("+%d -%d lines", diff.Stats.Additions, diff.Stats.Deletions))
	statsLabel.SetXAlign(0)
	statsLabel.AddCSSClass("dim-label")
	hv.contentBox.Append(statsLabel)

	// Create a card for each hunk.
	for i, hunk := range diff.Hunks {
		card := hv.createHunkCard(i, hunk)
		hv.contentBox.Append(card)
	}
}

// createHunkCard creates a widget card for a single hunk.
func (hv *HunkView) createHunkCard(index int, hunk git.Hunk) *gtk.Frame {
	// Card container.
	card := gtk.NewFrame("")

	cardBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// --- Hunk header with action button ---
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	headerBox.SetMarginTop(6)
	headerBox.SetMarginBottom(6)
	headerBox.SetMarginStart(12)
	headerBox.SetMarginEnd(12)

	// Hunk header text (e.g., "@@ -10,5 +10,7 @@").
	headerLabel := gtk.NewLabel(hunk.Header)
	headerLabel.SetXAlign(0)
	headerLabel.AddCSSClass("monospace")
	headerLabel.AddCSSClass("dim-label")
	headerLabel.SetHExpand(true)
	headerBox.Append(headerLabel)

	// Capture values for closures.
	hunkCopy := hunk
	filePath := hv.currentPath
	staged := hv.isStaged

	// Discard hunk button (only for unstaged files).
	if !staged {
		discardBtn := gtk.NewButtonWithLabel("Discard Hunk")
		discardBtn.AddCSSClass("destructive-action")
		discardBtn.SetVAlign(gtk.AlignCenter)
		discardBtn.ConnectClicked(func() {
			if hv.staging.repo == nil {
				return
			}
			if err := hv.staging.repo.DiscardHunk(filePath, hunkCopy); err != nil {
				slog.Warn("discard hunk failed", "path", filePath, "error", err)
				hv.staging.showToast("Failed: " + err.Error())
				return
			}
			hv.staging.Refresh()
		})
		headerBox.Append(discardBtn)
	}

	// Stage/Unstage hunk button.
	var actionBtn *gtk.Button
	if staged {
		actionBtn = gtk.NewButtonWithLabel("Unstage Hunk")
	} else {
		actionBtn = gtk.NewButtonWithLabel("Stage Hunk")
		actionBtn.AddCSSClass("suggested-action")
	}
	actionBtn.SetVAlign(gtk.AlignCenter)

	// Wire the stage/unstage hunk action.
	actionBtn.ConnectClicked(func() {
		if hv.staging.repo == nil {
			return
		}
		var err error
		if staged {
			err = hv.staging.repo.UnstageHunk(filePath, hunkCopy)
		} else {
			err = hv.staging.repo.StageHunk(filePath, hunkCopy)
		}
		if err != nil {
			slog.Warn("hunk stage/unstage failed", "path", filePath, "error", err)
			hv.staging.showToast("Failed: " + err.Error())
			return
		}
		hv.staging.Refresh()
	})
	headerBox.Append(actionBtn)

	cardBox.Append(headerBox)

	// --- Diff lines ---
	buffer := gtk.NewTextBuffer(nil)

	// Create tags for coloring.
	tagTable := buffer.TagTable()

	addedTag := gtk.NewTextTag("added")
	addedTag.SetObjectProperty("foreground", "#26a269")
	tagTable.Add(addedTag)

	deletedTag := gtk.NewTextTag("deleted")
	deletedTag.SetObjectProperty("foreground", "#e01b24")
	tagTable.Add(deletedTag)

	linenoTag := gtk.NewTextTag("lineno")
	linenoTag.SetObjectProperty("foreground", "#77767b")
	tagTable.Add(linenoTag)

	// Populate the buffer with diff lines.
	for _, line := range hunk.Lines {
		endIter := buffer.EndIter()
		offset := endIter.Offset()

		prefix := " "
		tagName := ""

		switch line.Type {
		case git.DiffLineAdd:
			prefix = "+"
			tagName = "added"
		case git.DiffLineDelete:
			prefix = "-"
			tagName = "deleted"
		}

		// Line number prefix.
		linenoStr := fmt.Sprintf("%4d %4d ", line.OldLineNo, line.NewLineNo)
		buffer.Insert(endIter, linenoStr)
		if tagName == "" {
			tagName = "lineno"
		}
		startIter := buffer.IterAtOffset(offset)
		linenoEndIter := buffer.IterAtOffset(offset + len(linenoStr))
		buffer.ApplyTagByName("lineno", startIter, linenoEndIter)

		// Line content.
		contentOffset := buffer.EndIter().Offset()
		content := prefix + line.Content + "\n"
		endIter2 := buffer.EndIter()
		buffer.Insert(endIter2, content)

		if tagName != "" && tagName != "lineno" {
			contentStart := buffer.IterAtOffset(contentOffset)
			contentEnd := buffer.EndIter()
			buffer.ApplyTagByName(tagName, contentStart, contentEnd)
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

	cardBox.Append(textView)

	card.SetChild(cardBox)
	return card
}

// Clear resets the hunk view to its empty state.
func (hv *HunkView) Clear() {
	for child := hv.contentBox.FirstChild(); child != nil; child = hv.contentBox.FirstChild() {
		hv.contentBox.Remove(child)
	}

	placeholder := gtk.NewLabel("Select a file to view its changes")
	placeholder.AddCSSClass("dim-label")
	hv.contentBox.Append(placeholder)
}
