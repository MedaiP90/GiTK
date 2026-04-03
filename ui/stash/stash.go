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

// buildDiffView creates a colored gtk.TextView for a structured DiffResult.
func buildDiffView(diff git.DiffResult) *gtk.Frame {
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

	for _, hunk := range diff.Hunks {
		stashInsertWithTag(buffer, hunk.Header+"\n", "hunk")
		for _, line := range hunk.Lines {
			prefix := " "
			tag := ""
			switch line.Type {
			case git.DiffLineAdd:
				prefix = "+"
				tag = "added"
			case git.DiffLineDelete:
				prefix = "-"
				tag = "deleted"
			}
			linenoStr := stashFormatLineNumbers(line)
			stashInsertWithTag(buffer, linenoStr, "lineno")
			text := prefix + line.Content + "\n"
			if tag != "" {
				stashInsertWithTag(buffer, text, tag)
			} else {
				stashInsertPlain(buffer, text)
			}
		}
		stashInsertPlain(buffer, "\n")
	}

	if len(diff.Hunks) == 0 {
		stashInsertPlain(buffer, "No changes to display.\n")
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

func stashInsertWithTag(buffer *gtk.TextBuffer, text, tagName string) {
	endIter := buffer.EndIter()
	offset := endIter.Offset()
	buffer.Insert(endIter, text)
	startIter := buffer.IterAtOffset(offset)
	newEndIter := buffer.EndIter()
	buffer.ApplyTagByName(tagName, startIter, newEndIter)
}

func stashInsertPlain(buffer *gtk.TextBuffer, text string) {
	endIter := buffer.EndIter()
	buffer.Insert(endIter, text)
}

func stashFormatLineNumbers(line git.DiffLine) string {
	old := "    "
	newL := "    "
	if line.OldLineNo > 0 {
		old = fmt.Sprintf("%4d", line.OldLineNo)
	}
	if line.NewLineNo > 0 {
		newL = fmt.Sprintf("%4d", line.NewLineNo)
	}
	return old + " " + newL + " "
}

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
	row.Connect("notify::expanded", func() {
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
		diffs, err := sv.repo.StashShow(index)
		glib.IdleAdd(func() {
			if err != nil {
				errRow := adw.NewActionRow()
				errRow.SetTitle("Could not load diff: " + err.Error())
				errRow.AddCSSClass("error")
				row.AddRow(errRow)
				return
			}
			if len(diffs) == 0 {
				emptyRow := adw.NewActionRow()
				emptyRow.SetTitle("No changes")
				emptyRow.AddCSSClass("dim-label")
				row.AddRow(emptyRow)
				return
			}
			for _, diff := range diffs {
				fileExpander := adw.NewExpanderRow()
				fileExpander.SetTitle(diff.NewPath)
				fileExpander.SetIconName("text-x-generic-symbolic")

				diffView := buildDiffView(diff)
				diffView.SetMarginTop(6)
				diffView.SetMarginBottom(6)
				diffView.SetMarginStart(12)
				diffView.SetMarginEnd(12)

				diffRow := adw.NewActionRow()
				diffRow.SetChild(diffView)
				fileExpander.AddRow(diffRow)

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
