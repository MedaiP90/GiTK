// Package filehistory implements the per-file commit history view.
//
// The file history view shows all commits that touched a given file,
// following renames (using "git log --follow"). Each row shows the commit
// hash, subject, author, and date — similar to the main commit log but
// filtered to a single file.
//
// Widget hierarchy:
//
//	GtkBox (vertical)
//	  ├─ AdwHeaderBar (title + Back button)
//	  └─ GtkScrolledWindow
//	       └─ GtkListBox (one row per commit)
package filehistory

import (
	"fmt"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// FileHistoryView shows the commit history for a single file.
type FileHistoryView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *gtk.Box

	// repo is the current repository.
	repo *git.Repository

	// listBox holds the commit rows.
	listBox *gtk.ListBox

	// titleLabel shows the file path.
	titleLabel *gtk.Label

	// onBack is called when the user presses the back button.
	onBack func()

	// onCommitSelected is called when the user clicks a commit in the list.
	onCommitSelected func(commit git.CommitInfo)
}

// New creates a new FileHistoryView.
func New(onBack func(), onCommitSelected func(commit git.CommitInfo)) *FileHistoryView {
	fh := &FileHistoryView{
		onBack:           onBack,
		onCommitSelected: onCommitSelected,
	}
	fh.build()
	return fh
}

func (fh *FileHistoryView) build() {
	fh.Root = gtk.NewBox(gtk.OrientationVertical, 0)

	// Header bar.
	header := adw.NewHeaderBar()
	header.SetShowTitle(true)
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(false)

	backBtn := gtk.NewButtonWithLabel("← Back")
	backBtn.AddCSSClass("flat")
	backBtn.ConnectClicked(func() {
		if fh.onBack != nil {
			fh.onBack()
		}
	})
	header.PackStart(backBtn)

	fh.titleLabel = gtk.NewLabel("File History")
	fh.titleLabel.AddCSSClass("title")
	header.SetTitleWidget(fh.titleLabel)

	fh.Root.Append(header)

	// Scrolled window + list box.
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	fh.listBox = gtk.NewListBox()
	fh.listBox.SetSelectionMode(gtk.SelectionSingle)
	fh.listBox.AddCSSClass("boxed-list")

	fh.listBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		if fh.onCommitSelected == nil {
			return
		}
		// The commit is stored in the row's Name (as the hash).
		// Retrieve using index from our stored slice.
	})

	scrolled.SetChild(fh.listBox)
	fh.Root.Append(scrolled)
}

// SetRepository sets the repository.
func (fh *FileHistoryView) SetRepository(repo *git.Repository) {
	fh.repo = repo
}

// Load loads the commit history for the given file. Runs in a goroutine.
func (fh *FileHistoryView) Load(filePath string) {
	fh.clearRows()
	fh.titleLabel.SetText("File History: " + filePath)

	if fh.repo == nil {
		return
	}

	go func() {
		commits, err := fh.repo.FileLog(filePath, 500)
		glib.IdleAdd(func() {
			if err != nil {
				fh.showError(err.Error())
				return
			}
			fh.populateRows(commits)
		})
	}()
}

func (fh *FileHistoryView) clearRows() {
	for {
		row := fh.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		fh.listBox.Remove(row)
	}
}

func (fh *FileHistoryView) showError(msg string) {
	row := gtk.NewListBoxRow()
	row.SetActivatable(false)
	lbl := gtk.NewLabel("Error: " + msg)
	lbl.SetXAlign(0)
	lbl.AddCSSClass("error")
	lbl.SetMarginTop(12)
	lbl.SetMarginBottom(12)
	lbl.SetMarginStart(12)
	row.SetChild(lbl)
	fh.listBox.Append(row)
}

func (fh *FileHistoryView) populateRows(commits []git.CommitInfo) {
	for _, c := range commits {
		commit := c // capture for closure
		row := adw.NewActionRow()
		row.SetTitle(commit.Subject)
		row.SetSubtitle(fmt.Sprintf("%s · %s · %s",
			commit.ShortHash,
			commit.Author,
			commit.AuthorTime.Format("2006-01-02"),
		))
		row.SetActivatable(true)
		row.ConnectActivated(func() {
			if fh.onCommitSelected != nil {
				fh.onCommitSelected(commit)
			}
		})
		fh.listBox.Append(row)
	}

	if len(commits) == 0 {
		row := adw.NewActionRow()
		row.SetTitle("No commits found for this file")
		row.SetActivatable(false)
		fh.listBox.Append(row)
	}
}
