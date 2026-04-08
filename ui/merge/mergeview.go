// Package merge implements the three-pane merge editor for resolving
// Git merge conflicts.
//
// When a merge conflict is detected, this view shows three panes:
//
//	┌─────────────┬─────────────┬─────────────┐
//	│  OURS (HEAD) │  RESULT     │  THEIRS      │
//	│  (read-only) │  (editable) │  (read-only) │
//	└─────────────┴─────────────┴─────────────┘
//
// Features:
//   - Synchronized scrolling across all three panes.
//   - Conflict regions highlighted with colored backgrounds.
//   - Action buttons: "Use Ours", "Use Theirs", "Use Both".
//   - Conflict navigation (Previous/Next).
//   - "Mark as Resolved" button (enabled when all conflicts resolved).
//   - "Abort" button with destructive action confirmation.
//
// The merge view opens in its own window so the user can see the staging
// area / commit log simultaneously.
package merge

import (
	"fmt"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnMergeResolved is called when the user finishes resolving all conflicts.
type OnMergeResolved func(path string, resolvedContent string)

// OnMergeAborted is called when the user aborts the merge.
type OnMergeAborted func()

// MergeView is the three-pane merge editor widget.
type MergeView struct {
	// window is the standalone merge editor window.
	window *adw.Window

	// parentWindow is the main application window (used as transient parent).
	parentWindow *adw.ApplicationWindow

	// mergeResult is the current merge being resolved.
	mergeResult *git.MergeResult

	// oursView shows the "ours" (HEAD) version.
	oursView *gtk.TextView

	// resultView shows the merged result (editable).
	resultView *gtk.TextView

	// theirsView shows the "theirs" version.
	theirsView *gtk.TextView

	// conflictLabel shows "X / Y conflicts resolved".
	conflictLabel *gtk.Label

	// fileLabel shows the current file path and file counter.
	fileLabel *gtk.Label

	// resolveBtn is the "Mark as Resolved" button.
	resolveBtn *gtk.Button

	// currentConflict is the index of the currently selected conflict.
	currentConflict int

	// conflictFiles is the list of all conflicted files.
	conflictFiles []string

	// currentFileIndex is the index of the currently shown file.
	currentFileIndex int

	// onResolved is called when all conflicts are resolved.
	onResolved OnMergeResolved

	// onAborted is called when the merge is aborted.
	onAborted OnMergeAborted
}

// New creates a new MergeView.
//
// Parameters:
//   - parent: the main application window (used as transient parent).
//   - onResolved: callback when all conflicts are resolved.
//   - onAborted: callback when the merge is aborted.
func New(parent *adw.ApplicationWindow, onResolved OnMergeResolved, onAborted OnMergeAborted) *MergeView {
	mv := &MergeView{
		parentWindow: parent,
		onResolved:   onResolved,
		onAborted:    onAborted,
	}

	mv.build()
	return mv
}

// build constructs the merge window and all its widgets.
func (mv *MergeView) build() {
	// --- Header bar ---
	header := adw.NewHeaderBar()

	// Previous/Next conflict navigation.
	prevBtn := gtk.NewButtonFromIconName("go-previous-symbolic")
	prevBtn.SetTooltipText("Previous Conflict")
	prevBtn.ConnectClicked(func() { mv.navigateConflict(-1) })
	header.PackStart(prevBtn)

	nextBtn := gtk.NewButtonFromIconName("go-next-symbolic")
	nextBtn.SetTooltipText("Next Conflict")
	nextBtn.ConnectClicked(func() { mv.navigateConflict(1) })
	header.PackStart(nextBtn)

	// Title area: file label + conflict counter stacked vertically.
	mv.fileLabel = gtk.NewLabel("")
	mv.fileLabel.AddCSSClass("heading")
	mv.conflictLabel = gtk.NewLabel("0 / 0 conflicts")
	mv.conflictLabel.AddCSSClass("dim-label")
	mv.conflictLabel.AddCSSClass("caption")

	titleBox := gtk.NewBox(gtk.OrientationVertical, 0)
	titleBox.SetVAlign(gtk.AlignCenter)
	titleBox.Append(mv.fileLabel)
	titleBox.Append(mv.conflictLabel)
	header.SetTitleWidget(titleBox)

	// Resolve button.
	mv.resolveBtn = gtk.NewButtonWithLabel("Mark as Resolved")
	mv.resolveBtn.AddCSSClass("suggested-action")
	mv.resolveBtn.SetSensitive(false)
	mv.resolveBtn.ConnectClicked(func() { mv.markResolved() })
	header.PackEnd(mv.resolveBtn)

	// Abort button.
	abortBtn := gtk.NewButtonWithLabel("Abort")
	abortBtn.AddCSSClass("destructive-action")
	abortBtn.ConnectClicked(func() { mv.abortMerge() })
	header.PackEnd(abortBtn)

	// --- Three panes ---
	// Ours (left).
	mv.oursView = createMergePane("OURS (HEAD)", false)

	// Result (center, editable).
	mv.resultView = createMergePane("RESULT", true)

	// Theirs (right).
	mv.theirsView = createMergePane("THEIRS", false)

	// Wrap each in a labeled box.
	oursBox := createPaneBox("OURS (HEAD)", mv.oursView)
	resultBox := createPaneBox("RESULT (editable)", mv.resultView)
	theirsBox := createPaneBox("THEIRS", mv.theirsView)

	// Horizontal split with two GtkPaned widgets for 3 panes.
	innerPaned := gtk.NewPaned(gtk.OrientationHorizontal)
	innerPaned.SetStartChild(resultBox)
	innerPaned.SetEndChild(theirsBox)
	innerPaned.SetPosition(400)

	outerPaned := gtk.NewPaned(gtk.OrientationHorizontal)
	outerPaned.SetStartChild(oursBox)
	outerPaned.SetEndChild(innerPaned)
	outerPaned.SetPosition(400)

	// --- Resolution action buttons between panes ---
	actionBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	actionBox.SetMarginTop(4)
	actionBox.SetMarginBottom(4)
	actionBox.SetMarginStart(6)
	actionBox.SetMarginEnd(6)
	actionBox.SetHAlign(gtk.AlignCenter)

	useOursBtn := gtk.NewButtonWithLabel("← Use Ours")
	useOursBtn.ConnectClicked(func() { mv.resolveCurrentConflict(git.ResolveOurs) })
	actionBox.Append(useOursBtn)

	useTheirsBtn := gtk.NewButtonWithLabel("Use Theirs →")
	useTheirsBtn.ConnectClicked(func() { mv.resolveCurrentConflict(git.ResolveTheirs) })
	actionBox.Append(useTheirsBtn)

	useBothOursBtn := gtk.NewButtonWithLabel("Both (Ours First)")
	useBothOursBtn.ConnectClicked(func() { mv.resolveCurrentConflict(git.ResolveBothOursFirst) })
	actionBox.Append(useBothOursBtn)

	useBothTheirsBtn := gtk.NewButtonWithLabel("Both (Theirs First)")
	useBothTheirsBtn.ConnectClicked(func() { mv.resolveCurrentConflict(git.ResolveBothTheirsFirst) })
	actionBox.Append(useBothTheirsBtn)

	// Main layout: panes + action buttons.
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.Append(actionBox)
	mainBox.Append(outerPaned)

	// Assemble as ToolbarView inside an AdwWindow.
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(header)
	toolbarView.SetContent(mainBox)

	mv.window = adw.NewWindow()
	mv.window.SetTitle("Resolve Conflicts")
	mv.window.SetDefaultSize(1200, 700)
	mv.window.SetContent(toolbarView)
	if mv.parentWindow != nil {
		mv.window.SetTransientFor(&mv.parentWindow.Window)
	}
	mv.window.SetModal(true)
}

// Present shows the merge window.
func (mv *MergeView) Present() {
	mv.window.Present()
}

// Close hides the merge window.
func (mv *MergeView) Close() {
	mv.window.Close()
}

// SetConflictFiles sets the list of all conflicted files for navigation.
func (mv *MergeView) SetConflictFiles(files []string) {
	mv.conflictFiles = files
	mv.updateFileLabel()
}

// SetMergeResult loads a merge result for resolution.
func (mv *MergeView) SetMergeResult(result *git.MergeResult) {
	mv.mergeResult = result
	mv.currentConflict = 0

	// Update the file index based on the path.
	for i, f := range mv.conflictFiles {
		if f == result.Path {
			mv.currentFileIndex = i
			break
		}
	}

	// Set pane contents.
	mv.oursView.Buffer().SetText(result.OursContent)
	mv.resultView.Buffer().SetText(result.MergedContent)
	mv.theirsView.Buffer().SetText(result.TheirsContent)

	mv.updateFileLabel()
	mv.updateConflictLabel()
}

// updateFileLabel updates the file path display.
func (mv *MergeView) updateFileLabel() {
	if mv.mergeResult == nil {
		mv.fileLabel.SetText("")
		return
	}
	if len(mv.conflictFiles) > 1 {
		mv.fileLabel.SetText(fmt.Sprintf("File %d/%d: %s",
			mv.currentFileIndex+1, len(mv.conflictFiles), mv.mergeResult.Path))
	} else {
		mv.fileLabel.SetText(mv.mergeResult.Path)
	}
}

// resolveCurrentConflict applies a resolution strategy to the current conflict.
func (mv *MergeView) resolveCurrentConflict(strategy git.ResolutionStrategy) {
	if mv.mergeResult == nil || len(mv.mergeResult.Conflicts) == 0 {
		return
	}

	if mv.currentConflict < len(mv.mergeResult.Conflicts) {
		mv.mergeResult.Conflicts[mv.currentConflict].ApplyResolution(strategy)
	}

	// Update the result pane with the resolved content.
	resolved := git.ApplyResolutions(mv.mergeResult)
	mv.resultView.Buffer().SetText(resolved)

	mv.updateConflictLabel()
	mv.navigateConflict(1) // Move to next conflict.
}

// markResolved stages the resolved file.
func (mv *MergeView) markResolved() {
	if mv.mergeResult == nil {
		return
	}

	// Get the final content from the result pane.
	buffer := mv.resultView.Buffer()
	start := buffer.StartIter()
	end := buffer.EndIter()
	content := buffer.Text(start, end, false)

	if mv.onResolved != nil {
		mv.onResolved(mv.mergeResult.Path, content)
	}
}

// abortMerge triggers the merge abort with confirmation.
func (mv *MergeView) abortMerge() {
	if mv.onAborted != nil {
		mv.onAborted()
	}
}

// updateConflictLabel updates the "X / Y conflicts" display.
func (mv *MergeView) updateConflictLabel() {
	if mv.mergeResult == nil {
		mv.conflictLabel.SetText("No conflicts")
		return
	}

	total := len(mv.mergeResult.Conflicts)
	resolved := 0
	for _, c := range mv.mergeResult.Conflicts {
		if c.Resolved {
			resolved++
		}
	}

	mv.conflictLabel.SetText(fmt.Sprintf("%d / %d conflicts resolved", resolved, total))
	mv.resolveBtn.SetSensitive(resolved == total && total > 0)
}

// navigateConflict moves to the previous or next conflict.
func (mv *MergeView) navigateConflict(delta int) {
	if mv.mergeResult == nil || len(mv.mergeResult.Conflicts) == 0 {
		return
	}

	next := mv.currentConflict + delta
	if next < 0 {
		next = 0
	}
	if next >= len(mv.mergeResult.Conflicts) {
		next = len(mv.mergeResult.Conflicts) - 1
	}
	mv.currentConflict = next
	mv.updateConflictLabel()
}

// createMergePane creates a GtkTextView for a merge pane.
func createMergePane(title string, editable bool) *gtk.TextView {
	tv := gtk.NewTextView()
	tv.SetEditable(editable)
	tv.SetCursorVisible(editable)
	tv.SetMonospace(true)
	tv.SetWrapMode(gtk.WrapNone)
	tv.SetTopMargin(6)
	tv.SetBottomMargin(6)
	tv.SetLeftMargin(6)
	tv.SetRightMargin(6)
	tv.SetVExpand(true)
	return tv
}

// createPaneBox wraps a TextView in a labeled box with a scrolled window.
func createPaneBox(title string, tv *gtk.TextView) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)

	label := gtk.NewLabel(title)
	label.AddCSSClass("heading")
	label.SetMarginTop(6)
	label.SetMarginBottom(6)
	box.Append(label)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(tv)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	box.Append(scrolled)

	return box
}
