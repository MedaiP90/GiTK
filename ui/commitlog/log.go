// Package commitlog implements the commit history table view.
//
// The commit log is displayed using a GtkColumnView (a table widget)
// with the following columns:
//   - Graph:   A custom GtkDrawingArea showing the DAG lane lines.
//   - Hash:    The short commit hash.
//   - Subject: The first line of the commit message.
//   - Author:  The commit author's name.
//   - Date:    A relative timestamp (e.g., "2 hours ago").
//   - Refs:    Branch/tag labels as colored pills.
//
// The commit log supports:
//   - Clicking a row to load the commit detail view.
//   - Filtering by message, author, or hash via GtkSearchEntry.
//   - Right-click context menu for tag creation on a commit.
//
// GtkColumnView requires a data model (GtkSelectionModel wrapping a
// GtkListModel). We use a GtkStringList as a simple list model where
// each string entry is a commit hash. The actual commit data is looked
// up from a map when the factory binds a row.
package commitlog

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MedaiP90/GiTK/git"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnCommitSelected is called when the user clicks a commit in the log.
type OnCommitSelected func(commit git.CommitInfo)

// CommitLog is the commit history table widget.
type CommitLog struct {
	// Root is the top-level widget (AdwToolbarView with header + table).
	Root *adw.ToolbarView

	// repo is the currently loaded repository.
	repo *git.Repository

	// commits is the list of commits currently displayed.
	commits []git.CommitInfo

	// allCommits is the full (unfiltered) commit list.
	allCommits []git.CommitInfo

	// commitMap maps hash → CommitInfo for quick lookup during binding.
	commitMap map[string]git.CommitInfo

	// graphCommits holds the graph layout data for the graph column.
	graphCommits []git.GraphCommit

	// graphCommitMap maps hash → GraphCommit for O(1) lookup during bind.
	graphCommitMap map[string]git.GraphCommit

	// currentBranch is the name of the currently checked-out branch, used
	// to make the corresponding ref pill stand out in the refs column.
	currentBranch string

	// graphBox holds one DrawingArea per commit row for the external graph panel.
	graphBox *gtk.Box

	// graphScrolled is the scrolled window for the external graph panel.
	// Its vertical adjustment is kept in sync with the table scrolled window.
	graphScrolled *gtk.ScrolledWindow

	// columnView is the GtkColumnView table widget.
	columnView *gtk.ColumnView

	// model is the string list backing the column view.
	model *gtk.StringList

	// selection is the selection model wrapping the list model.
	selection *gtk.SingleSelection

	// searchEntry is the filter/search bar.
	searchEntry *gtk.SearchEntry

	// onCommitSelected is called when a commit is clicked.
	onCommitSelected OnCommitSelected
}

// New creates a new CommitLog widget.
//
// Parameters:
//   - onCommitSelected: callback when a commit row is activated.
func New(onCommitSelected OnCommitSelected) *CommitLog {
	cl := &CommitLog{
		commitMap:        make(map[string]git.CommitInfo),
		graphCommitMap:   make(map[string]git.GraphCommit),
		onCommitSelected: onCommitSelected,
	}

	cl.build()
	return cl
}

// build constructs all the commit log widgets.
func (cl *CommitLog) build() {
	// --- Search/filter bar ---
	cl.searchEntry = gtk.NewSearchEntry()
	cl.searchEntry.SetPlaceholderText("Filter commits by message, author, or hash…")
	cl.searchEntry.SetHExpand(true)

	// Wrap search in a clamp for consistent width.
	searchClamp := adw.NewClamp()
	searchClamp.SetMaximumSize(800)
	searchClamp.SetChild(cl.searchEntry)

	searchBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	searchBox.SetMarginTop(6)
	searchBox.SetMarginBottom(6)
	searchBox.SetMarginStart(12)
	searchBox.SetMarginEnd(12)
	searchBox.Append(searchClamp)

	// --- Column View ---
	// GtkStringList is a simple list model where each entry is a string.
	// We store commit hashes as the strings.
	cl.model = gtk.NewStringList(nil)

	// SingleSelection allows selecting one row at a time.
	cl.selection = gtk.NewSingleSelection(cl.model)

	// Create the column view.
	cl.columnView = gtk.NewColumnView(cl.selection)
	cl.columnView.SetShowRowSeparators(true)
	cl.columnView.SetShowColumnSeparators(false)
	cl.columnView.SetVExpand(true)

	// Add columns — graph is now a separate element, NOT a column.
	cl.addHashColumn()
	cl.addSubjectColumn()
	cl.addAuthorColumn()
	cl.addDateColumn()
	cl.addRefsColumn()

	// Handle row activation (double-click/enter).
	cl.columnView.ConnectActivate(func(pos uint) {
		if int(pos) < len(cl.commits) {
			commit := cl.commits[pos]
			if cl.onCommitSelected != nil {
				cl.onCommitSelected(commit)
			}
		}
	})

	// Handle single-click selection changes to show commit detail.
	cl.selection.ConnectSelectionChanged(func(position, nItems uint) {
		pos := cl.selection.Selected()
		if int(pos) < len(cl.commits) {
			commit := cl.commits[pos]
			if cl.onCommitSelected != nil {
				cl.onCommitSelected(commit)
			}
		}
	})

	// Handle search filtering.
	cl.searchEntry.ConnectSearchChanged(func() {
		cl.applyFilter(cl.searchEntry.Text())
	})

	// --- Graph panel (left of table) ---
	cl.graphBox = gtk.NewBox(gtk.OrientationVertical, 0)
	cl.graphBox.SetVExpand(true)

	cl.graphScrolled = gtk.NewScrolledWindow()
	cl.graphScrolled.SetChild(cl.graphBox)
	cl.graphScrolled.SetVExpand(true)
	cl.graphScrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyExternal)
	cl.graphScrolled.SetSizeRequest(120, -1)

	// --- Table scrolled window ---
	tableScrolled := gtk.NewScrolledWindow()
	tableScrolled.SetChild(cl.columnView)
	tableScrolled.SetVExpand(true)
	tableScrolled.SetHExpand(true)

	// Sync graph scroll with table scroll (share vertical adjustment).
	tableScrolled.ConnectMap(func() {
		cl.graphScrolled.SetVAdjustment(tableScrolled.VAdjustment())
	})

	// Horizontal box combining graph panel + table.
	tableArea := gtk.NewBox(gtk.OrientationHorizontal, 0)
	tableArea.Append(cl.graphScrolled)
	tableArea.Append(tableScrolled)
	tableArea.SetVExpand(true)

	// Main content box.
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.Append(searchBox)
	contentBox.Append(tableArea)

	// Assemble into toolbar view.
	cl.Root = adw.NewToolbarView()
	cl.Root.SetContent(contentBox)
}

// SetRepository loads commits from the given repository.
func (cl *CommitLog) SetRepository(repo *git.Repository) {
	cl.repo = repo
	cl.Refresh()
}

// Refresh reloads commits from the current repository.
func (cl *CommitLog) Refresh() {
	if cl.repo == nil {
		return
	}

	// Load all commits (all branches) so the list matches the graph data.
	commits, err := cl.repo.LogAll(2000)
	if err != nil {
		slog.Warn("failed to load commit log", "error", err)
		return
	}

	// Build graph layout for the graph column.
	graphCommits, err := git.BuildGraph(cl.repo, git.DefaultGraphOptions())
	if err != nil {
		slog.Warn("failed to build graph", "error", err)
	}
	cl.graphCommits = graphCommits

	// Build a hash → GraphCommit map for O(1) lookup in the bind callback.
	cl.graphCommitMap = make(map[string]git.GraphCommit, len(graphCommits))
	for _, gc := range graphCommits {
		cl.graphCommitMap[gc.Hash] = gc
	}

	// Track the current branch for highlighted ref pill.
	cl.currentBranch = cl.repo.CurrentBranch()

	cl.allCommits = commits
	cl.setCommits(commits)
}

// setCommits replaces the displayed commits.
func (cl *CommitLog) setCommits(commits []git.CommitInfo) {
	cl.commits = commits

	// Rebuild the commit map.
	cl.commitMap = make(map[string]git.CommitInfo, len(commits))
	for _, c := range commits {
		cl.commitMap[c.Hash] = c
	}

	// Rebuild the string list model.
	// Clear existing entries.
	cl.model.Splice(0, cl.model.NItems(), nil)

	// Add new entries.
	hashes := make([]string, len(commits))
	for i, c := range commits {
		hashes[i] = c.Hash
	}
	cl.model.Splice(0, 0, hashes)

	// Rebuild the graph panel (one DrawingArea per commit row).
	cl.rebuildGraphPanel()

	// Pre-select the first commit so the detail panel is populated.
	if len(commits) > 0 {
		cl.selection.SetSelected(0)
	}

	slog.Debug("commit log updated", "count", len(commits))
}

// rebuildGraphPanel clears and refills the external graph box with one
// DrawingArea per commit row, each 120 px wide and 28 px tall (matching
// the row height used by the column view).
func (cl *CommitLog) rebuildGraphPanel() {
	// Clear existing drawing areas.
	for child := cl.graphBox.FirstChild(); child != nil; child = cl.graphBox.FirstChild() {
		cl.graphBox.Remove(child)
	}

	for _, commit := range cl.commits {
		da := NewGraphRenderer()
		da.SetSizeRequest(120, 28)
		if gc, ok := cl.graphCommitMap[commit.Hash]; ok {
			SetGraphCommit(da, gc)
		}
		cl.graphBox.Append(da)
	}
}

// applyFilter filters the displayed commits by the search text.
func (cl *CommitLog) applyFilter(query string) {
	if cl.repo == nil {
		return
	}

	query = strings.ToLower(strings.TrimSpace(query))

	if query == "" {
		// No filter — show all commits.
		cl.setCommits(cl.allCommits)
		return
	}

	// Filter commits by message, author, or hash.
	var filtered []git.CommitInfo
	for _, c := range cl.allCommits {
		if strings.Contains(strings.ToLower(c.Subject), query) ||
			strings.Contains(strings.ToLower(c.Author), query) ||
			strings.Contains(strings.ToLower(c.Hash), query) {
			filtered = append(filtered, c)
		}
	}

	cl.setCommits(filtered)
}

// RefsForCommit returns the graph refs (branches/tags) for a given commit hash.
func (cl *CommitLog) RefsForCommit(hash string) []git.GraphRef {
	if gc, ok := cl.graphCommitMap[hash]; ok {
		return gc.Refs
	}
	return nil
}

// SelectByHash selects the row matching the given commit hash in the table.
// If the hash is not found in the current (possibly filtered) commit list,
// the selection is left unchanged.
func (cl *CommitLog) SelectByHash(hash string) {
	for i, c := range cl.commits {
		if c.Hash == hash {
			cl.selection.SetSelected(uint(i))
			return
		}
	}
}

// toCell casts a *coreglib.Object to a *gtk.ColumnViewCell.
// In gotk4 v0.3.2+, GtkColumnView's SignalListItemFactory callbacks
// receive a GtkColumnViewCell (not GtkListItem). ColumnViewCell embeds
// ListItem and provides the same SetChild/Child/Position API.
func toCell(obj *coreglib.Object) *gtk.ColumnViewCell {
	return obj.Cast().(*gtk.ColumnViewCell)
}

// addGraphColumn adds the DAG graph column.
func (cl *CommitLog) addGraphColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		renderer := NewGraphRenderer()
		renderer.SetSizeRequest(100, 28)
		item.SetChild(renderer)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		pos := item.Position()
		da := item.Child().(*gtk.DrawingArea)
		if int(pos) < len(cl.commits) {
			if gc, ok := cl.graphCommitMap[cl.commits[pos].Hash]; ok {
				SetGraphCommit(da, gc)
			}
		}
	})

	col := gtk.NewColumnViewColumn("Graph", &factory.ListItemFactory)
	col.SetFixedWidth(120)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// addHashColumn adds the short hash column.
func (cl *CommitLog) addHashColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		label := gtk.NewLabel("")
		label.SetXAlign(0)
		label.AddCSSClass("monospace")
		label.AddCSSClass("dim-label")
		item.SetChild(label)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		pos := item.Position()
		if int(pos) < len(cl.commits) {
			label := item.Child().(*gtk.Label)
			label.SetText(cl.commits[pos].ShortHash)
		}
	})

	col := gtk.NewColumnViewColumn("Hash", &factory.ListItemFactory)
	col.SetFixedWidth(80)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// addSubjectColumn adds the commit message subject column.
func (cl *CommitLog) addSubjectColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		label := gtk.NewLabel("")
		label.SetXAlign(0)
		label.SetEllipsize(3) // PANGO_ELLIPSIZE_END
		label.SetHExpand(true)
		item.SetChild(label)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		pos := item.Position()
		if int(pos) < len(cl.commits) {
			label := item.Child().(*gtk.Label)
			label.SetText(cl.commits[pos].Subject)
		}
	})

	col := gtk.NewColumnViewColumn("Subject", &factory.ListItemFactory)
	col.SetFixedWidth(400)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// addAuthorColumn adds the commit author column.
func (cl *CommitLog) addAuthorColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		label := gtk.NewLabel("")
		label.SetXAlign(0)
		label.SetEllipsize(3) // PANGO_ELLIPSIZE_END
		item.SetChild(label)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		pos := item.Position()
		if int(pos) < len(cl.commits) {
			label := item.Child().(*gtk.Label)
			label.SetText(cl.commits[pos].Author)
		}
	})

	col := gtk.NewColumnViewColumn("Author", &factory.ListItemFactory)
	col.SetFixedWidth(150)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// addDateColumn adds the relative date column.
func (cl *CommitLog) addDateColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		label := gtk.NewLabel("")
		label.SetXAlign(0)
		label.AddCSSClass("dim-label")
		item.SetChild(label)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		pos := item.Position()
		if int(pos) < len(cl.commits) {
			label := item.Child().(*gtk.Label)
			label.SetText(relativeTime(cl.commits[pos].AuthorTime))
		}
	})

	col := gtk.NewColumnViewColumn("Date", &factory.ListItemFactory)
	col.SetFixedWidth(120)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// addRefsColumn adds the refs (branches/tags) column with colored pills.
// Refs are grouped by kind (local, remote, tag) into horizontal rows,
// then the rows are stacked vertically — at most 3 rows.
func (cl *CommitLog) addRefsColumn() {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := toCell(obj)
		box := gtk.NewBox(gtk.OrientationVertical, 1)
		box.SetVAlign(gtk.AlignCenter)
		item.SetChild(box)
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := toCell(obj)
		box := item.Child().(*gtk.Box)

		// Clear existing children.
		for child := box.FirstChild(); child != nil; child = box.FirstChild() {
			box.Remove(child)
		}

		pos := item.Position()
		if int(pos) < len(cl.graphCommits) {
			gc := cl.graphCommits[pos]

			// Group refs by kind.
			var locals, remotes, tags []git.GraphRef
			for _, ref := range gc.Refs {
				switch ref.Kind {
				case git.RefLocalBranch, git.RefHEAD:
					locals = append(locals, ref)
				case git.RefRemoteBranch:
					remotes = append(remotes, ref)
				case git.RefTag:
					tags = append(tags, ref)
				}
			}

			// Add a horizontal row per group.
			for _, group := range [][]git.GraphRef{locals, remotes, tags} {
				if len(group) == 0 {
					continue
				}
				row := gtk.NewBox(gtk.OrientationHorizontal, 4)
				for _, ref := range group {
					pill := createRefPill(ref, cl.currentBranch)
					row.Append(pill)
				}
				box.Append(row)
			}
		}
	})

	col := gtk.NewColumnViewColumn("Refs", &factory.ListItemFactory)
	col.SetFixedWidth(200)
	col.SetResizable(true)
	cl.columnView.AppendColumn(col)
}

// createRefPill creates a colored label "pill" for a branch/tag ref.
// currentBranch is the name of the checked-out branch; its pill gets an extra
// "suggested-action" class so it stands out from other local branch refs.
func createRefPill(ref git.GraphRef, currentBranch string) *gtk.Label {
	pill := gtk.NewLabel(ref.Name)
	pill.AddCSSClass("caption")

	// Style based on ref kind.
	switch ref.Kind {
	case git.RefLocalBranch:
		if ref.Name == currentBranch {
			// Checked-out branch gets a prominent accent-background pill.
			pill.AddCSSClass("suggested-action")
		} else {
			pill.AddCSSClass("accent")
		}
	case git.RefRemoteBranch:
		pill.AddCSSClass("dim-label")
	case git.RefTag:
		pill.AddCSSClass("warning")
	case git.RefHEAD:
		pill.AddCSSClass("success")
	}

	return pill
}

// relativeTime converts an absolute time to a human-readable relative string.
// Examples: "just now", "5 minutes ago", "2 hours ago", "3 days ago".
func relativeTime(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}
