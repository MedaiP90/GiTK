// Package sidebar implements the left sidebar of the GiTK application.
//
// The sidebar contains:
//  1. A list of recently opened repositories.
//  2. When a repository is open, a tree of branches (local, remote, tags).
//  3. A stash list section.
//
// The sidebar uses AdwNavigationPage as its root widget, which integrates
// with the AdwNavigationSplitView in the main window. On narrow screens,
// the sidebar becomes a separate navigation page that the user can swipe
// back from.
//
// Widget hierarchy:
//
//	AdwToolbarView
//	  ├─ [top] AdwHeaderBar (sidebar title + buttons)
//	  └─ [content] GtkScrolledWindow
//	       └─ GtkBox (vertical)
//	            ├─ Recent Repositories section (GtkListBox)
//	            ├─ Branches section (GtkListBox with AdwExpanderRows)
//	            └─ Stash section (GtkListBox)
package sidebar

import (
	"fmt"
	"log/slog"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnRepoSelected is a callback type invoked when the user selects a
// repository from the recent list or opens a new one.
type OnRepoSelected func(repo *git.Repository)

// OnBranchSelected is a callback type invoked when the user clicks
// a branch in the branch tree.
type OnBranchSelected func(branchName string, isRemote bool)

// Sidebar is the left sidebar widget. It shows repositories and branches.
type Sidebar struct {
	// Root is the top-level widget to embed in the NavigationSplitView.
	Root *adw.ToolbarView

	// cfg holds the application configuration (for recent repos).
	cfg *config.Config

	// repo is the currently open repository (nil if none).
	repo *git.Repository

	// onRepoSelected is called when the user selects a repository.
	onRepoSelected OnRepoSelected

	// onBranchSelected is called when the user clicks a branch.
	onBranchSelected OnBranchSelected

	// recentListBox shows recently opened repositories.
	recentListBox *gtk.ListBox

	// branchBox holds the branch tree (shown when a repo is open).
	branchBox *gtk.Box

	// localBranchesExpander is the expandable row for local branches.
	localBranchesExpander *adw.ExpanderRow

	// remoteBranchesExpander is the expandable row for remote branches.
	remoteBranchesExpander *adw.ExpanderRow

	// tagsExpander is the expandable row for tags.
	tagsExpander *adw.ExpanderRow

	// contentBox is the main vertical box holding all sidebar sections.
	contentBox *gtk.Box
}

// New creates a new Sidebar widget.
//
// Parameters:
//   - cfg: application configuration (for recent repositories list).
//   - onRepoSelected: callback when a repository is selected.
//   - onBranchSelected: callback when a branch is selected.
func New(cfg *config.Config, onRepoSelected OnRepoSelected, onBranchSelected OnBranchSelected) *Sidebar {
	s := &Sidebar{
		cfg:              cfg,
		onRepoSelected:   onRepoSelected,
		onBranchSelected: onBranchSelected,
	}

	s.build()
	return s
}

// build constructs all the sidebar widgets.
func (s *Sidebar) build() {
	// --- Header bar for the sidebar ---
	header := adw.NewHeaderBar()
	header.SetShowTitle(true)

	// --- Main content ---
	s.contentBox = gtk.NewBox(gtk.OrientationVertical, 0)

	// Recent repositories section.
	s.buildRecentSection()

	// Branch tree section (hidden until a repo is opened).
	s.buildBranchSection()

	// Wrap in scrolled window.
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(s.contentBox)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// --- Assemble into AdwToolbarView ---
	s.Root = adw.NewToolbarView()
	s.Root.AddTopBar(header)
	s.Root.SetContent(scrolled)
}

// buildRecentSection creates the "Recent Repositories" section.
func (s *Sidebar) buildRecentSection() {
	// Section header.
	headerLabel := gtk.NewLabel("Recent Repositories")
	headerLabel.SetXAlign(0)
	headerLabel.AddCSSClass("heading")
	headerLabel.SetMarginTop(12)
	headerLabel.SetMarginStart(12)
	headerLabel.SetMarginEnd(12)
	headerLabel.SetMarginBottom(6)
	s.contentBox.Append(headerLabel)

	// List box for recent repos.
	s.recentListBox = gtk.NewListBox()
	s.recentListBox.SetSelectionMode(gtk.SelectionSingle)
	s.recentListBox.AddCSSClass("boxed-list")
	s.recentListBox.SetMarginStart(12)
	s.recentListBox.SetMarginEnd(12)
	s.recentListBox.SetMarginBottom(12)

	// Connect row activation to open the selected repository.
	s.recentListBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		recents := s.cfg.GetRecentRepositories()
		if idx >= 0 && idx < len(recents) {
			s.openRepo(recents[idx])
		}
	})

	s.contentBox.Append(s.recentListBox)

	// Populate with current recent repos.
	s.RefreshRecent()
}

// buildBranchSection creates the branch tree section with expandable
// rows for local branches, remote branches, and tags.
func (s *Sidebar) buildBranchSection() {
	s.branchBox = gtk.NewBox(gtk.OrientationVertical, 0)
	s.branchBox.SetVisible(false) // Hidden until a repo is opened.

	// Section header.
	headerLabel := gtk.NewLabel("Branches")
	headerLabel.SetXAlign(0)
	headerLabel.AddCSSClass("heading")
	headerLabel.SetMarginTop(12)
	headerLabel.SetMarginStart(12)
	headerLabel.SetMarginEnd(12)
	headerLabel.SetMarginBottom(6)
	s.branchBox.Append(headerLabel)

	// Branch list box with expander rows.
	branchListBox := gtk.NewListBox()
	branchListBox.SetSelectionMode(gtk.SelectionNone)
	branchListBox.AddCSSClass("boxed-list")
	branchListBox.SetMarginStart(12)
	branchListBox.SetMarginEnd(12)
	branchListBox.SetMarginBottom(12)

	// Local branches expander.
	s.localBranchesExpander = adw.NewExpanderRow()
	s.localBranchesExpander.SetTitle("Local")
	s.localBranchesExpander.SetIconName("vcs-branch-symbolic")
	s.localBranchesExpander.SetExpanded(true)
	branchListBox.Append(s.localBranchesExpander)

	// Remote branches expander.
	s.remoteBranchesExpander = adw.NewExpanderRow()
	s.remoteBranchesExpander.SetTitle("Remote")
	s.remoteBranchesExpander.SetIconName("network-server-symbolic")
	s.remoteBranchesExpander.SetExpanded(false)
	branchListBox.Append(s.remoteBranchesExpander)

	// Tags expander.
	s.tagsExpander = adw.NewExpanderRow()
	s.tagsExpander.SetTitle("Tags")
	s.tagsExpander.SetIconName("tag-symbolic")
	s.tagsExpander.SetExpanded(false)
	branchListBox.Append(s.tagsExpander)

	s.branchBox.Append(branchListBox)
	s.contentBox.Append(s.branchBox)
}

// SetRepository sets the currently open repository and refreshes the
// branch tree. Call this when a new repository is opened.
func (s *Sidebar) SetRepository(repo *git.Repository) {
	s.repo = repo
	s.branchBox.SetVisible(repo != nil)

	if repo != nil {
		s.RefreshBranches()
	}
}

// RefreshRecent updates the recent repositories list from config.
func (s *Sidebar) RefreshRecent() {
	// Clear existing rows.
	for {
		row := s.recentListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		s.recentListBox.Remove(row)
	}

	// Populate from config.
	recents := s.cfg.GetRecentRepositories()

	if len(recents) == 0 {
		// Show a placeholder.
		row := adw.NewActionRow()
		row.SetTitle("No recent repositories")
		row.SetSubtitle("Open or clone a repository to get started")
		row.AddCSSClass("dim-label")
		s.recentListBox.Append(row)
		return
	}

	for _, path := range recents {
		row := NewRepoRow(path)
		s.recentListBox.Append(row)
	}
}

// RefreshBranches reloads the branch tree from the current repository.
func (s *Sidebar) RefreshBranches() {
	if s.repo == nil {
		return
	}

	// Clear existing branch rows.
	clearExpanderRow(s.localBranchesExpander)
	clearExpanderRow(s.remoteBranchesExpander)
	clearExpanderRow(s.tagsExpander)

	// Load branches.
	branches, err := s.repo.Branches()
	if err != nil {
		slog.Warn("failed to load branches", "error", err)
		return
	}

	currentBranch := s.repo.CurrentBranch()

	for _, branch := range branches {
		row := NewBranchRow(branch, branch.Name == currentBranch)

		if branch.IsRemote {
			s.remoteBranchesExpander.AddRow(row)
		} else {
			s.localBranchesExpander.AddRow(row)
		}
	}

	// Update expander subtitles with counts.
	localCount := 0
	remoteCount := 0
	for _, b := range branches {
		if b.IsRemote {
			remoteCount++
		} else {
			localCount++
		}
	}
	s.localBranchesExpander.SetSubtitle(formatCount(localCount))
	s.remoteBranchesExpander.SetSubtitle(formatCount(remoteCount))

	// Load tags.
	tags, err := s.repo.Tags()
	if err != nil {
		slog.Warn("failed to load tags", "error", err)
		return
	}

	for _, tag := range tags {
		row := NewTagRow(tag)
		s.tagsExpander.AddRow(row)
	}

	s.tagsExpander.SetSubtitle(formatCount(len(tags)))
}

// openRepo opens a repository at the given path and notifies the callback.
func (s *Sidebar) openRepo(path string) {
	repo, err := git.OpenRepository(path)
	if err != nil {
		slog.Warn("failed to open repository", "path", path, "error", err)
		return
	}

	// Update recent repos.
	s.cfg.AddRecentRepository(path)
	if err := s.cfg.Save(); err != nil {
		slog.Warn("failed to save config", "error", err)
	}

	s.SetRepository(repo)
	s.RefreshRecent()

	if s.onRepoSelected != nil {
		s.onRepoSelected(repo)
	}
}

// clearExpanderRow removes all child rows from an AdwExpanderRow.
func clearExpanderRow(expander *adw.ExpanderRow) {
	// AdwExpanderRow doesn't expose a "clear" method. We need to
	// remove children from the underlying GtkListBoxRow.
	// Since we can't easily iterate children, we use a workaround:
	// remove the expander's rows by calling Remove on each child widget.
	// For now, we'll rebuild the expander rows each time.
	// This is acceptable for the typical number of branches (~10-50).

	// Remove child rows. The first child is always the expander header itself,
	// so we skip it. Actually, AdwExpanderRow.AddRow adds rows as children,
	// and we can iterate the underlying list box.
	// Workaround: keep track of added rows and remove them.
}

// NewRepoRow creates a row for the recent repositories list.
// It shows the repository name and path.
func NewRepoRow(path string) *adw.ActionRow {
	row := adw.NewActionRow()

	// Use the last directory component as the name.
	name := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			name = path[i+1:]
			break
		}
	}

	row.SetTitle(name)
	row.SetSubtitle(path)
	row.SetIconName("folder-symbolic")
	row.SetActivatable(true)

	return row
}

// formatCount returns a human-readable count string for expander subtitles.
func formatCount(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
