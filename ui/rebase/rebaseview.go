// Package rebase implements the interactive rebase UI.
//
// The user selects a base commit (or branch) and sees the commits between
// that base and HEAD. Each commit has a dropdown to choose an action:
// pick, reword, squash, fixup, or drop. Commits can be reordered with
// Up/Down buttons. Clicking "Start Rebase" runs git rebase -i with the
// configured todo list.
//
// Widget hierarchy:
//
//	GtkBox (vertical)
//	  ├─ AdwHeaderBar (title + Back button)
//	  ├─ GtkBox (base selection row)
//	  └─ GtkScrolledWindow
//	       └─ GtkListBox (one row per commit)
//	            └─ [↑][↓] [action dropdown] [hash] [subject]
package rebase

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// actionLabels maps RebaseTodoAction to display strings.
var actionLabels = []string{"pick", "reword", "squash", "fixup", "drop"}

var actionValues = []git.RebaseTodoAction{
	git.RebasePick,
	git.RebaseReword,
	git.RebaseSquash,
	git.RebaseFixup,
	git.RebaseDrop,
}

// todoEntry is the internal mutable model for one rebase todo item.
type todoEntry struct {
	commit git.CommitInfo
	action git.RebaseTodoAction
}

// RebaseView is the interactive rebase UI widget.
type RebaseView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *gtk.Box

	// repo is the current repository.
	repo *git.Repository

	// todos is the ordered list of rebase todo entries.
	todos []todoEntry

	// listBox holds the todo rows.
	listBox *gtk.ListBox

	// baseEntry lets the user type a base ref (branch, hash, HEAD~N).
	baseEntry *adw.EntryRow

	// startBtn triggers the rebase.
	startBtn *gtk.Button

	// onBack is called when the user navigates away.
	onBack func()

	// onDone is called with a result message when the rebase completes.
	onDone func(msg string)
}

// New creates a new RebaseView.
func New(onBack func(), onDone func(string)) *RebaseView {
	rv := &RebaseView{onBack: onBack, onDone: onDone}
	rv.build()
	return rv
}

func (rv *RebaseView) build() {
	rv.Root = gtk.NewBox(gtk.OrientationVertical, 0)

	// Header bar.
	header := adw.NewHeaderBar()
	header.SetShowTitle(true)
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(false)

	backBtn := gtk.NewButtonWithLabel("← Back")
	backBtn.AddCSSClass("flat")
	backBtn.ConnectClicked(func() {
		if rv.onBack != nil {
			rv.onBack()
		}
	})
	header.PackStart(backBtn)

	titleLabel := gtk.NewLabel("Interactive Rebase")
	titleLabel.AddCSSClass("title")
	header.SetTitleWidget(titleLabel)

	rv.Root.Append(header)

	// Base selection group.
	baseGroup := adw.NewPreferencesGroup()
	baseGroup.SetTitle("Rebase Base")
	baseGroup.SetDescription("Enter a branch name, commit hash, or relative ref (e.g. HEAD~3, main)")
	baseGroup.SetMarginTop(12)
	baseGroup.SetMarginStart(12)
	baseGroup.SetMarginEnd(12)

	rv.baseEntry = adw.NewEntryRow()
	rv.baseEntry.SetTitle("Base Ref")
	rv.baseEntry.SetText("HEAD~5")
	baseGroup.Add(rv.baseEntry)

	loadBtn := gtk.NewButtonWithLabel("Load Commits")
	loadBtn.AddCSSClass("suggested-action")
	loadBtn.SetMarginTop(8)
	loadBtn.SetMarginBottom(8)
	loadBtn.ConnectClicked(func() {
		rv.loadCommits(rv.baseEntry.Text())
	})

	loadBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	loadBox.SetHAlign(gtk.AlignEnd)
	loadBox.SetMarginTop(8)
	loadBox.SetMarginStart(12)
	loadBox.SetMarginEnd(12)
	loadBox.Append(loadBtn)

	rv.Root.Append(baseGroup)
	rv.Root.Append(loadBox)

	// Commit list.
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	rv.listBox = gtk.NewListBox()
	rv.listBox.SetSelectionMode(gtk.SelectionNone)
	rv.listBox.AddCSSClass("boxed-list")
	rv.listBox.SetMarginStart(12)
	rv.listBox.SetMarginEnd(12)
	rv.listBox.SetMarginTop(8)
	rv.listBox.SetMarginBottom(8)

	scrolled.SetChild(rv.listBox)
	rv.Root.Append(scrolled)

	// Start Rebase button.
	rv.startBtn = gtk.NewButtonWithLabel("Start Rebase")
	rv.startBtn.AddCSSClass("destructive-action")
	rv.startBtn.SetSensitive(false)
	rv.startBtn.SetMarginStart(12)
	rv.startBtn.SetMarginEnd(12)
	rv.startBtn.SetMarginBottom(12)
	rv.startBtn.SetHAlign(gtk.AlignEnd)
	rv.startBtn.ConnectClicked(rv.startRebase)
	rv.Root.Append(rv.startBtn)
}

// SetRepository sets the repository.
func (rv *RebaseView) SetRepository(repo *git.Repository) {
	rv.repo = repo
}

// PrepareFromHash pre-fills the base entry with the given commit hash
// and loads the commits above it. Useful when opened from a commit action.
func (rv *RebaseView) PrepareFromHash(hash string) {
	if len(hash) > 7 {
		rv.baseEntry.SetText(hash)
	}
	rv.loadCommits(hash)
}

func (rv *RebaseView) loadCommits(base string) {
	if rv.repo == nil || base == "" {
		return
	}

	rv.clearRows()
	rv.startBtn.SetSensitive(false)

	go func() {
		commits, err := rv.repo.ListRebaseCommits(base)
		glib.IdleAdd(func() {
			if err != nil {
				rv.showError(fmt.Sprintf("Failed to list commits for %q: %s", base, err.Error()))
				return
			}
			if len(commits) == 0 {
				rv.showError("No commits between " + base + " and HEAD")
				return
			}
			rv.todos = make([]todoEntry, len(commits))
			for i, c := range commits {
				rv.todos[i] = todoEntry{commit: c, action: git.RebasePick}
			}
			rv.rebuildRows()
			rv.startBtn.SetSensitive(true)
		})
	}()
}

func (rv *RebaseView) clearRows() {
	for {
		row := rv.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		rv.listBox.Remove(row)
	}
}

func (rv *RebaseView) showError(msg string) {
	row := gtk.NewListBoxRow()
	row.SetActivatable(false)
	lbl := gtk.NewLabel(msg)
	lbl.SetXAlign(0)
	lbl.AddCSSClass("error")
	lbl.SetMarginTop(12)
	lbl.SetMarginBottom(12)
	lbl.SetMarginStart(12)
	row.SetChild(lbl)
	rv.listBox.Append(row)
}

func (rv *RebaseView) rebuildRows() {
	rv.clearRows()
	for i := range rv.todos {
		rv.listBox.Append(rv.buildRow(i))
	}
}

func (rv *RebaseView) buildRow(idx int) *gtk.ListBoxRow {
	entry := &rv.todos[idx]
	row := gtk.NewListBoxRow()
	row.SetActivatable(false)

	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.SetMarginTop(6)
	box.SetMarginBottom(6)
	box.SetMarginStart(8)
	box.SetMarginEnd(8)

	// Up / Down reorder buttons.
	upBtn := gtk.NewButtonFromIconName("go-up-symbolic")
	upBtn.AddCSSClass("flat")
	upBtn.SetSensitive(idx > 0)
	upBtn.SetVAlign(gtk.AlignCenter)
	upBtn.ConnectClicked(func() {
		rv.swapEntries(idx, idx-1)
	})

	downBtn := gtk.NewButtonFromIconName("go-down-symbolic")
	downBtn.AddCSSClass("flat")
	downBtn.SetSensitive(idx < len(rv.todos)-1)
	downBtn.SetVAlign(gtk.AlignCenter)
	downBtn.ConnectClicked(func() {
		rv.swapEntries(idx, idx+1)
	})

	box.Append(upBtn)
	box.Append(downBtn)

	// Action dropdown.
	actionList := gtk.NewStringList(actionLabels)
	actionDrop := gtk.NewDropDown(actionList, nil)
	actionDrop.SetSizeRequest(90, -1)
	actionDrop.SetVAlign(gtk.AlignCenter)
	// Set initial selection.
	for j, av := range actionValues {
		if av == entry.action {
			actionDrop.SetSelected(uint(j))
			break
		}
	}
	actionDrop.ConnectNotify("selected", func() {
		sel := actionDrop.Selected()
		if int(sel) < len(actionValues) {
			rv.todos[idx].action = actionValues[sel]
		}
	})
	box.Append(actionDrop)

	// Commit hash pill.
	hashLabel := gtk.NewLabel(entry.commit.ShortHash)
	hashLabel.AddCSSClass("accent")
	hashLabel.AddCSSClass("caption")
	hashLabel.AddCSSClass("monospace")
	hashLabel.SetVAlign(gtk.AlignCenter)
	box.Append(hashLabel)

	// Subject.
	subjectLabel := gtk.NewLabel(entry.commit.Subject)
	subjectLabel.SetXAlign(0)
	subjectLabel.SetEllipsize(3)
	subjectLabel.SetHExpand(true)
	subjectLabel.SetVAlign(gtk.AlignCenter)
	box.Append(subjectLabel)

	// Author + date.
	metaLabel := gtk.NewLabel(fmt.Sprintf("%s · %s",
		entry.commit.Author,
		entry.commit.AuthorTime.Format("2006-01-02"),
	))
	metaLabel.AddCSSClass("dim-label")
	metaLabel.AddCSSClass("caption")
	metaLabel.SetVAlign(gtk.AlignCenter)
	box.Append(metaLabel)

	row.SetChild(box)
	return row
}

func (rv *RebaseView) swapEntries(i, j int) {
	if i < 0 || j < 0 || i >= len(rv.todos) || j >= len(rv.todos) {
		return
	}
	rv.todos[i], rv.todos[j] = rv.todos[j], rv.todos[i]
	rv.rebuildRows()
}

func (rv *RebaseView) startRebase() {
	if rv.repo == nil || len(rv.todos) == 0 {
		return
	}

	base := rv.baseEntry.Text()
	todos := make([]git.RebaseTodo, len(rv.todos))
	for i, e := range rv.todos {
		todos[i] = git.RebaseTodo{
			Action:  e.action,
			Hash:    e.commit.Hash,
			Subject: e.commit.Subject,
		}
	}

	rv.startBtn.SetSensitive(false)

	go func() {
		err := rv.repo.InteractiveRebase(base, todos)
		glib.IdleAdd(func() {
			rv.startBtn.SetSensitive(true)
			if err != nil {
				slog.Warn("rebase failed", "error", err)
				if rv.onDone != nil {
					rv.onDone("Rebase failed: " + err.Error())
				}
				return
			}
			if rv.onDone != nil {
				rv.onDone("Rebase completed successfully")
			}
			if rv.onBack != nil {
				rv.onBack()
			}
		})
	}()
}
