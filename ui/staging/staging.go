// Package staging implements the staging area view for GiTK.
//
// The staging area shows the current changes in the working tree split
// into two groups:
//   - Staged changes: files that are ready to be committed (git add).
//   - Unstaged changes: modified files that haven't been staged yet.
//
// The view also includes:
//   - A hunk-level diff viewer for the selected file.
//   - Stage/unstage buttons for individual files.
//   - A commit message area with the commit button.
//
// Widget hierarchy:
//
//	AdwToolbarView
//	  ├─ [top] AdwHeaderBar
//	  └─ [content] GtkPaned (horizontal)
//	       ├─ [start] GtkBox (file lists)
//	       │    ├─ "Staged Changes" GtkListBox
//	       │    ├─ "Unstaged Changes" GtkListBox
//	       │    └─ Commit message + button
//	       └─ [end] HunkView (diff + hunk staging)
package staging

import (
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnCommitCreated is called when the user successfully creates a commit.
type OnCommitCreated func(hash string)

// StagingView is the staging area widget.
type StagingView struct {
	// Root is the top-level widget.
	Root *adw.ToolbarView

	// repo is the current repository.
	repo *git.Repository

	// onCommitCreated is called after a successful commit.
	onCommitCreated OnCommitCreated

	// stagedListBox shows staged files.
	stagedListBox *gtk.ListBox

	// unstagedListBox shows unstaged/modified files.
	unstagedListBox *gtk.ListBox

	// hunkView shows the selected file's diff with hunk-level staging.
	hunkView *HunkView

	// commitMessage is the text view for the commit message.
	commitMessage *gtk.TextView

	// commitBtn is the commit button.
	commitBtn *gtk.Button

	// charCounter shows the subject line character count.
	charCounter *gtk.Label

	// toastOverlay for inline notifications.
	toastOverlay *adw.ToastOverlay
}

// New creates a new StagingView.
//
// Parameters:
//   - onCommitCreated: callback after a successful commit.
func New(onCommitCreated OnCommitCreated) *StagingView {
	sv := &StagingView{
		onCommitCreated: onCommitCreated,
	}

	sv.build()
	return sv
}

// build constructs all the staging view widgets.
func (sv *StagingView) build() {
	// --- File lists panel (left side) ---
	filePanel := gtk.NewBox(gtk.OrientationVertical, 0)

	// Stage all / Unstage all buttons in a header.
	actionBar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	actionBar.SetMarginTop(6)
	actionBar.SetMarginBottom(6)
	actionBar.SetMarginStart(12)
	actionBar.SetMarginEnd(12)

	stageAllBtn := gtk.NewButtonWithLabel("Stage All")
	stageAllBtn.AddCSSClass("suggested-action")
	stageAllBtn.ConnectClicked(func() {
		sv.stageAll()
	})
	actionBar.Append(stageAllBtn)

	unstageAllBtn := gtk.NewButtonWithLabel("Unstage All")
	unstageAllBtn.ConnectClicked(func() {
		sv.unstageAll()
	})
	actionBar.Append(unstageAllBtn)

	filePanel.Append(actionBar)

	// --- Staged changes section ---
	stagedLabel := gtk.NewLabel("Staged Changes")
	stagedLabel.SetXAlign(0)
	stagedLabel.AddCSSClass("heading")
	stagedLabel.SetMarginTop(6)
	stagedLabel.SetMarginStart(12)
	filePanel.Append(stagedLabel)

	sv.stagedListBox = gtk.NewListBox()
	sv.stagedListBox.SetSelectionMode(gtk.SelectionSingle)
	sv.stagedListBox.AddCSSClass("boxed-list")
	sv.stagedListBox.SetMarginStart(12)
	sv.stagedListBox.SetMarginEnd(12)
	sv.stagedListBox.SetMarginBottom(6)
	filePanel.Append(sv.stagedListBox)

	// --- Unstaged changes section ---
	unstagedLabel := gtk.NewLabel("Unstaged Changes")
	unstagedLabel.SetXAlign(0)
	unstagedLabel.AddCSSClass("heading")
	unstagedLabel.SetMarginTop(6)
	unstagedLabel.SetMarginStart(12)
	filePanel.Append(unstagedLabel)

	sv.unstagedListBox = gtk.NewListBox()
	sv.unstagedListBox.SetSelectionMode(gtk.SelectionSingle)
	sv.unstagedListBox.AddCSSClass("boxed-list")
	sv.unstagedListBox.SetMarginStart(12)
	sv.unstagedListBox.SetMarginEnd(12)
	sv.unstagedListBox.SetMarginBottom(12)
	filePanel.Append(sv.unstagedListBox)

	// --- Commit message area ---
	sv.buildCommitArea(filePanel)

	// Scroll the file panel.
	fileScrolled := gtk.NewScrolledWindow()
	fileScrolled.SetChild(filePanel)
	fileScrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// --- Hunk view (right side) ---
	sv.hunkView = NewHunkView(sv)

	// --- Split pane ---
	paned := gtk.NewPaned(gtk.OrientationHorizontal)
	paned.SetStartChild(fileScrolled)
	paned.SetEndChild(sv.hunkView.Root)
	paned.SetPosition(350)
	paned.SetShrinkStartChild(false)
	paned.SetShrinkEndChild(false)

	// --- Toast overlay ---
	sv.toastOverlay = adw.NewToastOverlay()
	sv.toastOverlay.SetChild(paned)

	// --- Assemble ---
	sv.Root = adw.NewToolbarView()
	sv.Root.SetContent(sv.toastOverlay)
}

// buildCommitArea creates the commit message and button area.
func (sv *StagingView) buildCommitArea(parent *gtk.Box) {
	commitBox := gtk.NewBox(gtk.OrientationVertical, 6)
	commitBox.SetMarginTop(12)
	commitBox.SetMarginStart(12)
	commitBox.SetMarginEnd(12)
	commitBox.SetMarginBottom(12)

	// Commit message label.
	msgLabel := gtk.NewLabel("Commit Message")
	msgLabel.SetXAlign(0)
	msgLabel.AddCSSClass("heading")
	commitBox.Append(msgLabel)

	// Character counter for subject line.
	sv.charCounter = gtk.NewLabel("0 / 72")
	sv.charCounter.SetXAlign(1)
	sv.charCounter.AddCSSClass("dim-label")
	sv.charCounter.AddCSSClass("caption")
	commitBox.Append(sv.charCounter)

	// Text view for commit message.
	sv.commitMessage = gtk.NewTextView()
	sv.commitMessage.SetWrapMode(gtk.WrapWord)
	sv.commitMessage.SetTopMargin(6)
	sv.commitMessage.SetBottomMargin(6)
	sv.commitMessage.SetLeftMargin(6)
	sv.commitMessage.SetRightMargin(6)
	sv.commitMessage.SetVExpand(false)
	sv.commitMessage.SetAcceptsTab(false)

	// Frame around the text view.
	msgFrame := gtk.NewFrame("")
	msgFrame.SetChild(sv.commitMessage)
	commitBox.Append(msgFrame)

	// Update character counter as the user types.
	buffer := sv.commitMessage.Buffer()
	buffer.ConnectChanged(func() {
		sv.updateCharCounter()
	})

	// Commit button.
	sv.commitBtn = gtk.NewButtonWithLabel("Commit")
	sv.commitBtn.AddCSSClass("suggested-action")
	sv.commitBtn.SetMarginTop(6)
	sv.commitBtn.ConnectClicked(func() {
		sv.doCommit()
	})
	commitBox.Append(sv.commitBtn)

	parent.Append(commitBox)
}

// SetRepository sets the current repository and refreshes the staging area.
func (sv *StagingView) SetRepository(repo *git.Repository) {
	sv.repo = repo
	sv.Refresh()
}

// Refresh reloads the staging area from the current repository.
func (sv *StagingView) Refresh() {
	if sv.repo == nil {
		return
	}

	// Clear existing rows.
	clearListBox(sv.stagedListBox)
	clearListBox(sv.unstagedListBox)

	// Get file status.
	changes, err := sv.repo.Status()
	if err != nil {
		slog.Warn("failed to get status", "error", err)
		return
	}

	for _, change := range changes {
		// Staged changes.
		if change.Staging != git.StatusUnmodified && change.Staging != git.FileStatusCode('?') {
			row := sv.createFileRow(change, true)
			sv.stagedListBox.Append(row)
		}

		// Unstaged changes.
		if change.Worktree != git.StatusUnmodified {
			row := sv.createFileRow(change, false)
			sv.unstagedListBox.Append(row)
		}
	}
}

// createFileRow creates an AdwActionRow for a file change with a
// stage/unstage action button.
func (sv *StagingView) createFileRow(change git.FileChange, isStaged bool) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(change.Path)
	row.SetActivatable(true)

	// Change type icon.
	var iconName string
	code := change.Worktree
	if isStaged {
		code = change.Staging
	}
	switch code {
	case git.StatusAdded, git.StatusUntracked:
		iconName = "list-add-symbolic"
	case git.StatusDeleted:
		iconName = "list-remove-symbolic"
	case git.StatusModified:
		iconName = "document-edit-symbolic"
	case git.StatusRenamed:
		iconName = "edit-find-replace-symbolic"
	default:
		iconName = "document-edit-symbolic"
	}
	row.SetIconName(iconName)

	// Stage/unstage button as suffix.
	var actionBtn *gtk.Button
	if isStaged {
		actionBtn = gtk.NewButtonFromIconName("list-remove-symbolic")
		actionBtn.SetTooltipText("Unstage")
		path := change.Path
		actionBtn.ConnectClicked(func() {
			sv.unstageFile(path)
		})
	} else {
		actionBtn = gtk.NewButtonFromIconName("list-add-symbolic")
		actionBtn.SetTooltipText("Stage")
		path := change.Path
		actionBtn.ConnectClicked(func() {
			sv.stageFile(path)
		})
	}
	actionBtn.SetVAlign(gtk.AlignCenter)
	row.AddSuffix(actionBtn)

	return row
}

// stageFile stages a single file.
func (sv *StagingView) stageFile(path string) {
	if sv.repo == nil {
		return
	}

	if err := sv.repo.StageFile(path); err != nil {
		slog.Warn("failed to stage file", "path", path, "error", err)
		sv.showToast("Failed to stage " + path)
		return
	}

	sv.Refresh()
}

// unstageFile unstages a single file.
func (sv *StagingView) unstageFile(path string) {
	if sv.repo == nil {
		return
	}

	if err := sv.repo.UnstageFile(path); err != nil {
		slog.Warn("failed to unstage file", "path", path, "error", err)
		sv.showToast("Failed to unstage " + path)
		return
	}

	sv.Refresh()
}

// stageAll stages all changes.
func (sv *StagingView) stageAll() {
	if sv.repo == nil {
		return
	}

	if err := sv.repo.StageAll(); err != nil {
		slog.Warn("failed to stage all", "error", err)
		sv.showToast("Failed to stage all changes")
		return
	}

	sv.Refresh()
	sv.showToast("All changes staged")
}

// unstageAll unstages all changes by doing a mixed reset.
func (sv *StagingView) unstageAll() {
	if sv.repo == nil {
		return
	}

	head, err := sv.repo.Head()
	if err != nil {
		slog.Warn("failed to get HEAD for unstage all", "error", err)
		return
	}

	if err := sv.repo.Reset(head.Hash().String(), git.ResetMixed); err != nil {
		slog.Warn("failed to unstage all", "error", err)
		sv.showToast("Failed to unstage all changes")
		return
	}

	sv.Refresh()
	sv.showToast("All changes unstaged")
}

// doCommit creates a new commit with the staged changes.
func (sv *StagingView) doCommit() {
	if sv.repo == nil {
		return
	}

	// Get the commit message.
	buffer := sv.commitMessage.Buffer()
	start := buffer.StartIter()
	end := buffer.EndIter()
	message := buffer.Text(start, end, false)

	if message == "" {
		sv.showToast("Please enter a commit message")
		return
	}

	// Perform the commit in a goroutine.
	go func() {
		hash, err := sv.repo.Commit(git.CommitOptions{
			Message: message,
		})

		glib.IdleAdd(func() {
			if err != nil {
				slog.Warn("commit failed", "error", err)
				sv.showToast("Commit failed: " + err.Error())
				return
			}

			// Clear the message and refresh.
			buffer.SetText("")
			sv.Refresh()
			sv.showToast("Committed " + hash[:7])

			if sv.onCommitCreated != nil {
				sv.onCommitCreated(hash)
			}
		})
	}()
}

// updateCharCounter updates the character counter label.
// The counter tracks the subject line (first line) length.
func (sv *StagingView) updateCharCounter() {
	buffer := sv.commitMessage.Buffer()
	start := buffer.StartIter()
	end := buffer.EndIter()
	text := buffer.Text(start, end, false)

	// Get the first line (subject).
	subject := text
	for i, ch := range text {
		if ch == '\n' {
			subject = text[:i]
			break
		}
	}

	count := len(subject)
	sv.charCounter.SetText(string(rune('0'+count/10)) + string(rune('0'+count%10)) + " / 72")

	// Remove all existing style classes.
	sv.charCounter.RemoveCSSClass("warning")
	sv.charCounter.RemoveCSSClass("error")

	// Color the counter based on length.
	if count > 72 {
		sv.charCounter.AddCSSClass("error")
	} else if count > 50 {
		sv.charCounter.AddCSSClass("warning")
	}

	// Simple counter display.
	counterText := ""
	if count < 100 {
		counterText = string(rune('0'+count/10)) + string(rune('0'+count%10))
	} else {
		counterText = "99+"
	}
	sv.charCounter.SetText(counterText + " / 72")
}

// showToast shows a toast notification in the staging view.
func (sv *StagingView) showToast(message string) {
	toast := adw.NewToast(message)
	sv.toastOverlay.AddToast(toast)
}

// clearListBox removes all rows from a GtkListBox.
func clearListBox(lb *gtk.ListBox) {
	for {
		row := lb.RowAtIndex(0)
		if row == nil {
			break
		}
		lb.Remove(row)
	}
}
