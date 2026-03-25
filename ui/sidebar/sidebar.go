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

	// recentExpander wraps the recent repos section so it's collapsible.
	recentExpander *adw.ExpanderRow

	// recentRows tracks rows added to recentExpander for clearing.
	recentRows []gtk.Widgetter

	// branchBox holds the branch tree (shown when a repo is open).
	branchBox *gtk.Box

	// branchListBox holds the expander rows for branches/tags.
	// We recreate expander rows on each refresh to avoid duplicates.
	branchListBox *gtk.ListBox

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
	// Hide window control buttons since the main window header already has them.
	header := adw.NewHeaderBar()
	header.SetShowTitle(true)
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(false)

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

// buildRecentSection creates the "Recent Repositories" section as a
// collapsible AdwExpanderRow so it can be collapsed to give more room
// to the branches section.
func (s *Sidebar) buildRecentSection() {
	// Wrapper list box for the expander row (AdwExpanderRow must be
	// inside a GtkListBox).
	recentWrapperList := gtk.NewListBox()
	recentWrapperList.SetSelectionMode(gtk.SelectionNone)
	recentWrapperList.AddCSSClass("boxed-list")
	recentWrapperList.SetMarginStart(12)
	recentWrapperList.SetMarginEnd(12)
	recentWrapperList.SetMarginTop(12)
	recentWrapperList.SetMarginBottom(12)

	// Collapsible expander for recent repos.
	s.recentExpander = adw.NewExpanderRow()
	s.recentExpander.SetTitle("Recent Repositories")
	s.recentExpander.SetIconName("document-open-recent-symbolic")
	s.recentExpander.SetExpanded(true)
	recentWrapperList.Append(s.recentExpander)

	s.contentBox.Append(recentWrapperList)

	// Populate with current recent repos.
	s.RefreshRecent()
}

// buildBranchSection creates the branch tree section. The actual expander
// rows are recreated on each RefreshBranches() call to avoid the duplicate
// entries problem (AdwExpanderRow has no reliable clear method).
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

	// Branch list box — will be rebuilt on each refresh.
	s.branchListBox = gtk.NewListBox()
	s.branchListBox.SetSelectionMode(gtk.SelectionNone)
	s.branchListBox.AddCSSClass("boxed-list")
	s.branchListBox.SetMarginStart(12)
	s.branchListBox.SetMarginEnd(12)
	s.branchListBox.SetMarginBottom(12)

	s.branchBox.Append(s.branchListBox)
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
	// Remove previously added rows.
	for _, row := range s.recentRows {
		s.recentExpander.Remove(row)
	}
	s.recentRows = nil

	// Populate from config.
	recents := s.cfg.GetRecentRepositories()

	if len(recents) == 0 {
		row := adw.NewActionRow()
		row.SetTitle("No recent repositories")
		row.SetSubtitle("Open or clone a repository to get started")
		row.AddCSSClass("dim-label")
		s.recentExpander.AddRow(row)
		s.recentRows = append(s.recentRows, row)
		return
	}

	for i, path := range recents {
		row := NewRepoRow(path)
		idx := i // capture for closure
		row.SetActivatable(true)
		row.ConnectActivated(func() {
			paths := s.cfg.GetRecentRepositories()
			if idx < len(paths) {
				s.openRepo(paths[idx])
			}
		})
		s.recentExpander.AddRow(row)
		s.recentRows = append(s.recentRows, row)
	}

	s.recentExpander.SetSubtitle(formatCount(len(recents)))
}

// RefreshBranches reloads the branch tree from the current repository.
// It completely rebuilds the branch list box to avoid duplicate entries.
func (s *Sidebar) RefreshBranches() {
	if s.repo == nil {
		return
	}

	// Remove all existing rows from the branch list box by clearing it.
	for {
		row := s.branchListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		s.branchListBox.Remove(row)
	}

	// Load branches.
	branches, err := s.repo.Branches()
	if err != nil {
		slog.Warn("failed to load branches", "error", err)
		return
	}

	currentBranch := s.repo.CurrentBranch()

	// Create fresh expander rows each time.
	localExpander := adw.NewExpanderRow()
	localExpander.SetTitle("Local")
	localExpander.SetIconName("vcs-branch-symbolic")
	localExpander.SetExpanded(true)

	remoteExpander := adw.NewExpanderRow()
	remoteExpander.SetTitle("Remote")
	remoteExpander.SetIconName("network-server-symbolic")
	remoteExpander.SetExpanded(false)

	localCount := 0
	remoteCount := 0

	for _, branch := range branches {
		row := NewBranchRow(branch, branch.Name == currentBranch)

		// Wire click to trigger branch checkout.
		branchName := branch.Name
		isRemote := branch.IsRemote
		row.ConnectActivated(func() {
			if s.onBranchSelected != nil {
				s.onBranchSelected(branchName, isRemote)
			}
		})

		if branch.IsRemote {
			remoteExpander.AddRow(row)
			remoteCount++
		} else {
			localExpander.AddRow(row)
			localCount++
		}
	}

	localExpander.SetSubtitle(formatCount(localCount))
	remoteExpander.SetSubtitle(formatCount(remoteCount))

	s.branchListBox.Append(localExpander)
	s.branchListBox.Append(remoteExpander)

	// Load tags.
	tags, err := s.repo.Tags()
	if err != nil {
		slog.Warn("failed to load tags", "error", err)
		return
	}

	tagsExpander := adw.NewExpanderRow()
	tagsExpander.SetTitle("Tags")
	tagsExpander.SetIconName("tag-symbolic")
	tagsExpander.SetExpanded(false)

	for _, tag := range tags {
		row := NewTagRow(tag)
		tagsExpander.AddRow(row)
	}

	tagsExpander.SetSubtitle(formatCount(len(tags)))
	s.branchListBox.Append(tagsExpander)
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
