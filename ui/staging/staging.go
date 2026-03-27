// Package staging implements the staging area view for GiTK.
//
// The staging area shows the current changes in the working tree split
// into two groups:
//   - Unstaged changes: modified files that haven't been staged yet.
//   - Staged changes: files that are ready to be committed (git add).
//
// The view also includes:
//   - A hunk-level diff viewer for the selected file.
//   - Stage/unstage buttons for individual files.
//   - A commit message area with subject + optional description.
//   - An AI button to generate commit messages via Claude.
//   - A stash button to stash current changes.
//   - A commit button that is only enabled when files are staged and a
//     message has been written.
//   - Live updates when the repository state changes.
//
// Widget hierarchy:
//
//	AdwToolbarView
//	  └─ [content] GtkPaned (horizontal)
//	       ├─ [start] GtkBox (file lists + commit area)
//	       └─ [end] HunkView (diff + hunk staging)
package staging

import (
	"context"
	"log/slog"
	"time"

	"github.com/MedaiP90/GiTK/ai"
	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnCommitCreated is called when the user successfully creates a commit.
type OnCommitCreated func(hash string)

// OnStashRequested is called when the user clicks the stash button
// in the staging area, allowing the parent to open the stash dialog.
type OnStashRequested func()

// OnChangesUpdated is called whenever the staging area detects that
// the number of changes may have changed (after stage/unstage/discard/commit).
type OnChangesUpdated func()

// StagingView is the staging area widget.
type StagingView struct {
	// Root is the top-level widget.
	Root *adw.ToolbarView

	// repo is the current repository.
	repo *git.Repository

	// cfg is the app configuration for AI settings.
	cfg *config.Config

	// onCommitCreated is called after a successful commit.
	onCommitCreated OnCommitCreated

	// onStashRequested is called when the stash button is clicked.
	onStashRequested OnStashRequested

	// onChangesUpdated is called when changes are staged/unstaged/discarded.
	onChangesUpdated OnChangesUpdated

	// stagedListBox shows staged files.
	stagedListBox *gtk.ListBox

	// unstagedListBox shows unstaged/modified files.
	unstagedListBox *gtk.ListBox

	// hunkView shows the selected file's diff with hunk-level staging.
	hunkView *HunkView

	// commitSubject is the text entry for the commit subject line.
	commitSubject *gtk.Entry

	// commitDescription is the text view for the optional commit body.
	commitDescription *gtk.TextView

	// commitBtn is the commit button.
	commitBtn *gtk.Button

	// aiBtn is the AI commit message generation button.
	aiBtn *gtk.Button

	// charCounter shows the subject line character count.
	charCounter *gtk.Label

	// toastOverlay for inline notifications.
	toastOverlay *adw.ToastOverlay

	// hasStagedFiles tracks whether any files are currently staged.
	hasStagedFiles bool

	// watcher monitors the repository for live changes.
	watcher *git.Watcher
}

// New creates a new StagingView.
//
// Parameters:
//   - cfg: the app configuration.
//   - onCommitCreated: callback after a successful commit.
func New(cfg *config.Config, onCommitCreated OnCommitCreated, onStashRequested OnStashRequested, onChangesUpdated OnChangesUpdated) *StagingView {
	sv := &StagingView{
		cfg:              cfg,
		onCommitCreated:  onCommitCreated,
		onStashRequested: onStashRequested,
		onChangesUpdated: onChangesUpdated,
	}

	sv.build()
	return sv
}

// build constructs all the staging view widgets.
func (sv *StagingView) build() {
	// --- File lists panel (left side) ---
	filePanel := gtk.NewBox(gtk.OrientationVertical, 0)

	// Action buttons: Stage all / Unstage all / Stash.
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

	// Spacer.
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	actionBar.Append(spacer)

	// Stash button — opens the stash dialog via callback.
	stashBtn := gtk.NewButtonFromIconName("sidebar-show-symbolic")
	stashBtn.SetTooltipText("Stash Changes")
	stashBtn.ConnectClicked(func() {
		if sv.onStashRequested != nil {
			sv.onStashRequested()
		}
	})
	actionBar.Append(stashBtn)

	filePanel.Append(actionBar)

	// --- Unstaged changes section (FIRST — above staged) ---
	unstagedLabel := gtk.NewLabel("Unstaged Changes")
	unstagedLabel.SetXAlign(0)
	unstagedLabel.AddCSSClass("heading")
	unstagedLabel.SetMarginTop(12)
	unstagedLabel.SetMarginStart(12)
	unstagedLabel.SetMarginBottom(6)
	filePanel.Append(unstagedLabel)

	sv.unstagedListBox = gtk.NewListBox()
	sv.unstagedListBox.SetSelectionMode(gtk.SelectionSingle)
	sv.unstagedListBox.AddCSSClass("boxed-list")
	sv.unstagedListBox.SetMarginStart(12)
	sv.unstagedListBox.SetMarginEnd(12)
	sv.unstagedListBox.SetMarginBottom(12)

	// Wire selection to show diff in hunk view.
	sv.unstagedListBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row != nil {
			// Deselect in the other list box.
			sv.stagedListBox.UnselectAll()
			sv.showDiffForSelectedRow(row, false)
		}
	})
	filePanel.Append(sv.unstagedListBox)

	// --- Staged changes section (SECOND — below unstaged) ---
	stagedLabel := gtk.NewLabel("Staged Changes")
	stagedLabel.SetXAlign(0)
	stagedLabel.AddCSSClass("heading")
	stagedLabel.SetMarginTop(12)
	stagedLabel.SetMarginStart(12)
	stagedLabel.SetMarginBottom(6)
	filePanel.Append(stagedLabel)

	sv.stagedListBox = gtk.NewListBox()
	sv.stagedListBox.SetSelectionMode(gtk.SelectionSingle)
	sv.stagedListBox.AddCSSClass("boxed-list")
	sv.stagedListBox.SetMarginStart(12)
	sv.stagedListBox.SetMarginEnd(12)
	sv.stagedListBox.SetMarginBottom(12)

	// Wire selection to show diff in hunk view.
	sv.stagedListBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row != nil {
			// Deselect in the other list box.
			sv.unstagedListBox.UnselectAll()
			sv.showDiffForSelectedRow(row, true)
		}
	})
	filePanel.Append(sv.stagedListBox)

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

// buildCommitArea creates the commit message (subject + description) and
// the commit button area.
func (sv *StagingView) buildCommitArea(parent *gtk.Box) {
	commitBox := gtk.NewBox(gtk.OrientationVertical, 6)
	commitBox.SetMarginTop(12)
	commitBox.SetMarginStart(12)
	commitBox.SetMarginEnd(12)
	commitBox.SetMarginBottom(12)

	// Commit subject label + counter.
	subjectHeader := gtk.NewBox(gtk.OrientationHorizontal, 0)
	msgLabel := gtk.NewLabel("Commit Message")
	msgLabel.SetXAlign(0)
	msgLabel.AddCSSClass("heading")
	msgLabel.SetHExpand(true)
	subjectHeader.Append(msgLabel)

	sv.charCounter = gtk.NewLabel("0 / 72")
	sv.charCounter.SetXAlign(1)
	sv.charCounter.AddCSSClass("dim-label")
	sv.charCounter.AddCSSClass("caption")
	subjectHeader.Append(sv.charCounter)
	commitBox.Append(subjectHeader)

	// Subject line entry (single line).
	sv.commitSubject = gtk.NewEntry()
	sv.commitSubject.SetPlaceholderText("Summary (required)")
	sv.commitSubject.ConnectChanged(func() {
		sv.updateCharCounter()
		sv.updateCommitButton()
	})
	commitBox.Append(sv.commitSubject)

	// Optional description (multi-line).
	descLabel := gtk.NewLabel("Description (optional)")
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetMarginTop(6)
	commitBox.Append(descLabel)

	sv.commitDescription = gtk.NewTextView()
	sv.commitDescription.SetWrapMode(gtk.WrapWord)
	sv.commitDescription.SetTopMargin(6)
	sv.commitDescription.SetBottomMargin(6)
	sv.commitDescription.SetLeftMargin(6)
	sv.commitDescription.SetRightMargin(6)
	sv.commitDescription.SetVExpand(false)
	sv.commitDescription.SetAcceptsTab(false)

	// Give the description a reasonable default height.
	sv.commitDescription.SetSizeRequest(-1, 80)

	descFrame := gtk.NewFrame("")
	descFrame.SetChild(sv.commitDescription)
	commitBox.Append(descFrame)

	// Buttons row: AI generate + Commit.
	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btnRow.SetMarginTop(6)

	// AI commit message button — only visible if AI is enabled.
	sv.aiBtn = gtk.NewButtonFromIconName("applications-science-symbolic")
	sv.aiBtn.SetTooltipText("Generate commit message with AI")
	sv.aiBtn.SetVisible(sv.cfg.AI.Enabled)
	sv.aiBtn.ConnectClicked(func() {
		sv.generateAICommitMessage()
	})
	btnRow.Append(sv.aiBtn)

	// Spacer.
	btnSpacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	btnSpacer.SetHExpand(true)
	btnRow.Append(btnSpacer)

	// Commit button — disabled until files are staged and message is written.
	sv.commitBtn = gtk.NewButtonWithLabel("Commit")
	sv.commitBtn.AddCSSClass("suggested-action")
	sv.commitBtn.SetSensitive(false)
	sv.commitBtn.ConnectClicked(func() {
		sv.doCommit()
	})
	btnRow.Append(sv.commitBtn)

	commitBox.Append(btnRow)

	parent.Append(commitBox)
}

// generateAICommitMessage uses Claude AI to generate a commit message
// from the staged diff.
func (sv *StagingView) generateAICommitMessage() {
	if sv.repo == nil {
		return
	}
	if !sv.cfg.AI.Enabled {
		sv.showToast("AI features are disabled. Enable in Preferences.")
		return
	}

	client := ai.NewProvider(sv.cfg.AI)
	if client == nil {
		sv.showToast("AI not configured. Set API key in Preferences.")
		return
	}

	sv.aiBtn.SetSensitive(false)
	sv.showToast("Generating commit message…")

	go func() {
		// Get staged diffs.
		diffs, err := sv.repo.DiffStaged()
		if err != nil {
			glib.IdleAdd(func() {
				sv.aiBtn.SetSensitive(true)
				sv.showToast("Failed to get staged diff: " + err.Error())
			})
			return
		}

		// Build diff text.
		var diffText string
		for _, d := range diffs {
			diffText += "--- " + d.OldPath + "\n+++ " + d.NewPath + "\n"
			for _, h := range d.Hunks {
				diffText += h.Header + "\n"
				for _, l := range h.Lines {
					switch l.Type {
					case git.DiffLineAdd:
						diffText += "+" + l.Content + "\n"
					case git.DiffLineDelete:
						diffText += "-" + l.Content + "\n"
					default:
						diffText += " " + l.Content + "\n"
					}
				}
			}
		}

		if diffText == "" {
			glib.IdleAdd(func() {
				sv.aiBtn.SetSensitive(true)
				sv.showToast("No staged changes to analyze.")
			})
			return
		}

		msg, err := client.GenerateCommitMessage(context.Background(), diffText, sv.cfg.AI.SystemPrompt)
		glib.IdleAdd(func() {
			sv.aiBtn.SetSensitive(true)
			if err != nil {
				sv.showToast("AI failed: " + err.Error())
				return
			}

			// Parse the message: first line is subject, rest is description.
			lines := splitMessage(msg)
			sv.commitSubject.SetText(lines[0])
			if len(lines) > 1 {
				sv.commitDescription.Buffer().SetText(lines[1])
			}
			sv.showToast("AI commit message generated")
		})
	}()
}

// splitMessage splits an AI-generated commit message into subject and body.
func splitMessage(msg string) [2]string {
	result := [2]string{}
	for i, ch := range msg {
		if ch == '\n' {
			result[0] = msg[:i]
			// Skip blank lines between subject and body.
			body := msg[i+1:]
			for len(body) > 0 && body[0] == '\n' {
				body = body[1:]
			}
			result[1] = body
			return result
		}
	}
	result[0] = msg
	return result
}

// SetRepository sets the current repository and refreshes the staging area.
// It also starts a file watcher for live updates.
func (sv *StagingView) SetRepository(repo *git.Repository) {
	// Stop any existing watcher.
	if sv.watcher != nil {
		sv.watcher.Stop()
		sv.watcher = nil
	}

	sv.repo = repo

	// Update AI button visibility.
	sv.aiBtn.SetVisible(sv.cfg.AI.Enabled)

	sv.Refresh()

	// Start a watcher for live updates.
	if repo != nil {
		sv.watcher = git.NewWatcher(repo, 2*time.Second)
		sv.watcher.Start()
		go sv.watchLoop()
	}
}

// watchLoop listens for watcher events and refreshes the staging view
// when the working tree or index changes.
func (sv *StagingView) watchLoop() {
	watcher := sv.watcher
	if watcher == nil {
		return
	}
	for event := range watcher.Events {
		switch event {
		case git.WatchEventWorkTree, git.WatchEventIndex, git.WatchEventHead:
			glib.IdleAdd(func() {
				sv.Refresh()
			})
		}
	}
}

// Refresh reloads the staging area from the current repository.
func (sv *StagingView) Refresh() {
	if sv.repo == nil {
		return
	}

	// Remember what was selected so we can re-select after rebuild.
	selectedPath := sv.hunkView.currentPath
	selectedStaged := sv.hunkView.isStaged

	// Clear existing rows.
	clearListBox(sv.stagedListBox)
	clearListBox(sv.unstagedListBox)

	// Get file status.
	changes, err := sv.repo.Status()
	if err != nil {
		slog.Warn("failed to get status", "error", err)
		return
	}

	sv.hasStagedFiles = false
	fileStillExists := false

	for _, change := range changes {
		// Unstaged changes.
		if change.Worktree != git.StatusUnmodified {
			row := sv.createFileRow(change, false)
			sv.unstagedListBox.Append(row)
			if change.Path == selectedPath && !selectedStaged {
				fileStillExists = true
			}
		}

		// Staged changes.
		if change.Staging != git.StatusUnmodified && change.Staging != git.FileStatusCode('?') {
			row := sv.createFileRow(change, true)
			sv.stagedListBox.Append(row)
			sv.hasStagedFiles = true
			if change.Path == selectedPath && selectedStaged {
				fileStillExists = true
			}
		}
	}

	// Re-show the diff for the previously selected file, or clear if gone.
	if selectedPath != "" && fileStillExists {
		sv.refreshHunkView(selectedPath, selectedStaged)
	} else {
		sv.hunkView.Clear()
	}

	sv.updateCommitButton()

	// Notify the parent that changes may have been updated.
	if sv.onChangesUpdated != nil {
		sv.onChangesUpdated()
	}
}

// refreshHunkView recomputes and shows the diff for the given file path.
func (sv *StagingView) refreshHunkView(path string, isStaged bool) {
	if sv.repo == nil {
		return
	}

	var diffs []git.DiffResult
	var err error
	if isStaged {
		diffs, err = sv.repo.DiffStaged()
	} else {
		diffs, err = sv.repo.DiffWorking()
	}

	if err != nil {
		slog.Warn("failed to refresh hunk view", "path", path, "error", err)
		sv.hunkView.Clear()
		return
	}

	for _, diff := range diffs {
		if diff.NewPath == path || diff.OldPath == path {
			sv.hunkView.SetFile(path, diff, isStaged)
			return
		}
	}

	sv.hunkView.Clear()
}

// showDiffForSelectedRow computes and shows the diff for the selected file.
func (sv *StagingView) showDiffForSelectedRow(row *gtk.ListBoxRow, isStaged bool) {
	if sv.repo == nil || row == nil {
		return
	}

	// Get the file path from the row's name (set when creating the row).
	path := row.Name()
	if path == "" {
		return
	}

	// Compute the diff for this file.
	var diffs []git.DiffResult
	var err error
	if isStaged {
		diffs, err = sv.repo.DiffStaged()
	} else {
		diffs, err = sv.repo.DiffWorking()
	}

	if err != nil {
		slog.Warn("failed to compute diff for staging", "path", path, "error", err)
		return
	}

	// Find the diff for this specific file.
	for _, diff := range diffs {
		if diff.NewPath == path || diff.OldPath == path {
			sv.hunkView.SetFile(path, diff, isStaged)
			return
		}
	}

	// No diff found — file might be binary or empty.
	sv.hunkView.SetFile(path, git.DiffResult{NewPath: path}, isStaged)
}

// createFileRow creates an AdwActionRow for a file change with a
// stage/unstage action button.
func (sv *StagingView) createFileRow(change git.FileChange, isStaged bool) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(change.Path)
	row.SetActivatable(true)

	// Store the path on the row's name so we can retrieve it on selection.
	row.SetName(change.Path)

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

		// Discard button for unstaged changes.
		discardBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
		discardBtn.SetTooltipText("Discard Changes")
		discardBtn.AddCSSClass("destructive-action")
		discardBtn.SetVAlign(gtk.AlignCenter)
		discardPath := change.Path
		discardBtn.ConnectClicked(func() {
			sv.discardFile(discardPath)
		})
		row.AddSuffix(discardBtn)
	}
	actionBtn.SetVAlign(gtk.AlignCenter)
	row.AddSuffix(actionBtn)

	return row
}

// updateCommitButton enables/disables the commit button based on whether
// there are staged files and a non-empty commit message.
func (sv *StagingView) updateCommitButton() {
	subject := sv.commitSubject.Text()
	enabled := sv.hasStagedFiles && len(subject) > 0
	sv.commitBtn.SetSensitive(enabled)
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

// discardFile discards unstaged changes to a single file by checking
// it out from the index (restoring to the last staged/committed state).
func (sv *StagingView) discardFile(path string) {
	if sv.repo == nil {
		return
	}

	if err := sv.repo.DiscardFile(path); err != nil {
		slog.Warn("failed to discard file", "path", path, "error", err)
		sv.showToast("Failed to discard " + path)
		return
	}

	sv.Refresh()
	sv.showToast("Discarded changes to " + path)
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

	// Build the commit message from subject + optional description.
	subject := sv.commitSubject.Text()
	if subject == "" {
		sv.showToast("Please enter a commit message")
		return
	}

	message := subject
	// Append description if provided.
	descBuffer := sv.commitDescription.Buffer()
	descStart := descBuffer.StartIter()
	descEnd := descBuffer.EndIter()
	description := descBuffer.Text(descStart, descEnd, false)
	if description != "" {
		message = subject + "\n\n" + description
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

			// Clear the message fields and refresh.
			sv.commitSubject.SetText("")
			descBuffer.SetText("")
			sv.Refresh()
			sv.showToast("Committed " + hash[:7])

			if sv.onCommitCreated != nil {
				sv.onCommitCreated(hash)
			}
		})
	}()
}

// updateCharCounter updates the character counter label for the subject line.
func (sv *StagingView) updateCharCounter() {
	text := sv.commitSubject.Text()
	count := len(text)

	// Remove all existing style classes.
	sv.charCounter.RemoveCSSClass("warning")
	sv.charCounter.RemoveCSSClass("error")

	// Color the counter based on length.
	if count > 72 {
		sv.charCounter.AddCSSClass("error")
	} else if count > 50 {
		sv.charCounter.AddCSSClass("warning")
	}

	sv.charCounter.SetText(formatCharCount(count))
}

// formatCharCount formats the character count as "N / 72".
func formatCharCount(count int) string {
	if count >= 100 {
		return "99+ / 72"
	}
	tens := count / 10
	ones := count % 10
	return string(rune('0'+tens)) + string(rune('0'+ones)) + " / 72"
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
