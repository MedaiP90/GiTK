// Package blame implements the blame/annotate view.
//
// The blame view shows a file's contents with each line annotated by the
// commit that last modified it: short hash, author name, date, and the
// commit subject. It looks similar to "git blame" terminal output but in
// a GNOME-native style.
//
// Widget hierarchy:
//
//	GtkBox (vertical)
//	  ├─ AdwHeaderBar (title + Back button)
//	  └─ GtkScrolledWindow
//	       └─ GtkListBox (one row per line)
//	            └─ GtkBox: [hash pill] [author] [date] │ [line content]
package blame

import (
	"fmt"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// BlameView shows blame annotations for a single file.
type BlameView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *gtk.Box

	// repo is the current repository.
	repo *git.Repository

	// listBox holds the annotated lines.
	listBox *gtk.ListBox

	// titleLabel shows the file path and commit.
	titleLabel *gtk.Label

	// onBack is called when the user clicks the back button.
	onBack func()
}

// New creates a new BlameView.
func New(onBack func()) *BlameView {
	bv := &BlameView{onBack: onBack}
	bv.build()
	return bv
}

func (bv *BlameView) build() {
	bv.Root = gtk.NewBox(gtk.OrientationVertical, 0)

	// Header bar.
	header := adw.NewHeaderBar()
	header.SetShowTitle(true)
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(false)

	backBtn := gtk.NewButtonWithLabel("← Back")
	backBtn.AddCSSClass("flat")
	backBtn.ConnectClicked(func() {
		if bv.onBack != nil {
			bv.onBack()
		}
	})
	header.PackStart(backBtn)

	bv.titleLabel = gtk.NewLabel("Blame")
	bv.titleLabel.AddCSSClass("title")
	header.SetTitleWidget(bv.titleLabel)

	bv.Root.Append(header)

	// Scrolled window + list box.
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)

	bv.listBox = gtk.NewListBox()
	bv.listBox.SetSelectionMode(gtk.SelectionNone)
	bv.listBox.AddCSSClass("boxed-list-separate")

	scrolled.SetChild(bv.listBox)
	bv.Root.Append(scrolled)
}

// SetRepository sets the repository for blame operations.
func (bv *BlameView) SetRepository(repo *git.Repository) {
	bv.repo = repo
}

// Load loads blame data for the given file at the given commit hash
// (empty = HEAD). The operation runs in a goroutine; the UI updates on
// the GTK main thread via glib.IdleAdd.
func (bv *BlameView) Load(path, commitHash string) {
	bv.clearLines()

	label := path
	if commitHash != "" {
		short := commitHash
		if len(commitHash) > 7 {
			short = commitHash[:7]
		}
		label = path + " @ " + short
	}
	bv.titleLabel.SetText("Blame: " + label)

	if bv.repo == nil {
		return
	}

	go func() {
		lines, err := bv.repo.BlameFile(path, commitHash)
		glib.IdleAdd(func() {
			if err != nil {
				bv.showError(err.Error())
				return
			}
			bv.populateLines(lines)
		})
	}()
}

func (bv *BlameView) clearLines() {
	for {
		row := bv.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		bv.listBox.Remove(row)
	}
}

func (bv *BlameView) showError(msg string) {
	row := gtk.NewListBoxRow()
	row.SetActivatable(false)
	lbl := gtk.NewLabel("Error: " + msg)
	lbl.SetXAlign(0)
	lbl.AddCSSClass("error")
	lbl.SetMarginTop(12)
	lbl.SetMarginBottom(12)
	lbl.SetMarginStart(12)
	lbl.SetMarginEnd(12)
	row.SetChild(lbl)
	bv.listBox.Append(row)
}

func (bv *BlameView) populateLines(lines []git.BlameLine) {
	for _, bl := range lines {
		row := gtk.NewListBoxRow()
		row.SetActivatable(false)

		lineBox := gtk.NewBox(gtk.OrientationHorizontal, 0)

		// Line number.
		lineNoLabel := gtk.NewLabel(fmt.Sprintf("%4d", bl.LineNo))
		lineNoLabel.AddCSSClass("dim-label")
		lineNoLabel.AddCSSClass("monospace")
		lineNoLabel.SetMarginStart(8)
		lineNoLabel.SetMarginEnd(8)
		lineNoLabel.SetVAlign(gtk.AlignCenter)
		lineBox.Append(lineNoLabel)

		// Annotation column: hash pill + author + date.
		annotBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
		annotBox.SetMarginEnd(8)
		annotBox.SetSizeRequest(300, -1)

		hashLabel := gtk.NewLabel(bl.ShortHash)
		hashLabel.AddCSSClass("accent")
		hashLabel.AddCSSClass("caption")
		hashLabel.AddCSSClass("monospace")
		hashLabel.SetVAlign(gtk.AlignCenter)
		annotBox.Append(hashLabel)

		authorLabel := gtk.NewLabel(bl.Author)
		authorLabel.AddCSSClass("dim-label")
		authorLabel.AddCSSClass("caption")
		authorLabel.SetEllipsize(3)
		authorLabel.SetMaxWidthChars(14)
		authorLabel.SetVAlign(gtk.AlignCenter)
		annotBox.Append(authorLabel)

		dateLabel := gtk.NewLabel(bl.Date.Format("2006-01-02"))
		dateLabel.AddCSSClass("dim-label")
		dateLabel.AddCSSClass("caption")
		dateLabel.SetVAlign(gtk.AlignCenter)
		annotBox.Append(dateLabel)

		lineBox.Append(annotBox)

		// Separator.
		sep := gtk.NewSeparator(gtk.OrientationVertical)
		sep.SetMarginTop(4)
		sep.SetMarginBottom(4)
		lineBox.Append(sep)

		// Source line.
		codeLabel := gtk.NewLabel(bl.Text)
		codeLabel.SetXAlign(0)
		codeLabel.AddCSSClass("monospace")
		codeLabel.SetMarginStart(8)
		codeLabel.SetHExpand(true)
		codeLabel.SetSelectable(true)
		codeLabel.SetVAlign(gtk.AlignCenter)
		lineBox.Append(codeLabel)

		row.SetChild(lineBox)
		bv.listBox.Append(row)
	}
}
