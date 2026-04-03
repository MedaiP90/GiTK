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
	"os"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnRepoSelected is a callback type invoked when the user selects a
// repository from the recent list or opens a new one.
type OnRepoSelected func(repo *git.Repository)

// OnBranchSelected is a callback type invoked when the user clicks
// a branch in the branch tree.
type OnBranchSelected func(branchName string, isRemote bool)

// OnRepoRemoved is a callback type invoked when a recent repository
// is found to be deleted from the filesystem and removed from the list.
type OnRepoRemoved func(path string)

// OnBranchDelete is a callback invoked when the user requests to delete a local branch.
type OnBranchDelete func(branchName string)

// OnTagDelete is a callback invoked when the user requests to delete a tag.
type OnTagDelete func(tagName string)

// OnBranchMerge is a callback invoked when the user requests to merge a branch into the current one.
type OnBranchMerge func(branchName string)

// OnBranchRebase is a callback invoked when the user wants to rebase the current branch onto the selected one.
type OnBranchRebase func(branchName string)

// OnAddRemote is a callback invoked when the user wants to add a new remote.
type OnAddRemote func()

// OnSubmoduleAdd is a callback invoked when the user wants to add a new submodule.
type OnSubmoduleAdd func()

// OnSubmoduleRemove is a callback invoked when the user wants to remove a submodule.
type OnSubmoduleRemove func(path string)

// OnSubmoduleUpdate is a callback invoked when the user wants to update all submodules.
type OnSubmoduleUpdate func()

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

	// onRepoRemoved is called when a repo is found to be deleted.
	onRepoRemoved OnRepoRemoved

	// onBranchDelete is called when the user requests to delete a local branch.
	onBranchDelete OnBranchDelete

	// onTagDelete is called when the user requests to delete a tag.
	onTagDelete OnTagDelete

	// onBranchMerge is called when the user requests to merge a branch into the current one.
	onBranchMerge OnBranchMerge

	// onBranchRebase is called when the user requests to rebase onto a branch.
	onBranchRebase OnBranchRebase

	// onAddRemote is called when the user wants to add a new remote.
	onAddRemote OnAddRemote

	// onSubmoduleAdd is called when the user wants to add a new submodule.
	onSubmoduleAdd OnSubmoduleAdd

	// onSubmoduleRemove is called when the user wants to remove a submodule.
	onSubmoduleRemove OnSubmoduleRemove

	// onSubmoduleUpdate is called when the user wants to update all submodules.
	onSubmoduleUpdate OnSubmoduleUpdate

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
//
// SidebarCallbacks groups all optional action callbacks for the sidebar.
type SidebarCallbacks struct {
	OnRepoSelected    OnRepoSelected
	OnBranchSelected  OnBranchSelected
	OnRepoRemoved     OnRepoRemoved
	OnBranchDelete    OnBranchDelete
	OnTagDelete       OnTagDelete
	OnBranchMerge     OnBranchMerge
	OnBranchRebase    OnBranchRebase
	OnAddRemote       OnAddRemote
	OnSubmoduleAdd    OnSubmoduleAdd
	OnSubmoduleRemove OnSubmoduleRemove
	OnSubmoduleUpdate OnSubmoduleUpdate
}

func New(cfg *config.Config, cb SidebarCallbacks) *Sidebar {
	s := &Sidebar{
		cfg:               cfg,
		onRepoSelected:    cb.OnRepoSelected,
		onBranchSelected:  cb.OnBranchSelected,
		onRepoRemoved:     cb.OnRepoRemoved,
		onBranchDelete:    cb.OnBranchDelete,
		onTagDelete:       cb.OnTagDelete,
		onBranchMerge:     cb.OnBranchMerge,
		onBranchRebase:    cb.OnBranchRebase,
		onAddRemote:       cb.OnAddRemote,
		onSubmoduleAdd:    cb.OnSubmoduleAdd,
		onSubmoduleRemove: cb.OnSubmoduleRemove,
		onSubmoduleUpdate: cb.OnSubmoduleUpdate,
	}

	s.build()
	return s
}

// build constructs all the sidebar widgets.
func (s *Sidebar) build() {
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
	// No internal header bar — the sidebar merges visually with the main
	// application toolbar (GNOME HIG sidebar pattern).
	s.Root = adw.NewToolbarView()
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

// CollapseRecentRepos collapses the recent repositories expander to give
// more space to the branch tree below.
func (s *Sidebar) CollapseRecentRepos() {
	s.recentExpander.SetExpanded(false)
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
		// Skip repos that no longer exist on disk.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		row := NewRepoRow(path)
		repoPath := path
		_ = i // index not used directly
		row.SetActivatable(true)
		row.ConnectActivated(func() {
			s.openRepo(repoPath)
		})

		// Remove button — lets users manually remove a repo from the recent list.
		removeBtn := gtk.NewButtonFromIconName("list-remove-symbolic")
		removeBtn.SetTooltipText("Remove from recent")
		removeBtn.AddCSSClass("flat")
		removeBtn.SetVAlign(gtk.AlignCenter)
		removeBtn.ConnectClicked(func() {
			s.cfg.RemoveRecentRepository(repoPath)
			if err := s.cfg.Save(); err != nil {
				slog.Warn("failed to save config after removing recent repo", "error", err)
			}
			s.RefreshRecent()
		})
		row.AddSuffix(removeBtn)

		// Check if repo is dirty (has uncommitted changes) in background.
		go func(p string, r *adw.ActionRow) {
			repo, err := git.OpenRepository(p)
			if err != nil {
				return
			}
			changes, err := repo.Status()
			if err != nil {
				return
			}
			if len(changes) > 0 {
				glib.IdleAdd(func() {
					// Add a badge suffix to indicate dirty state.
					badge := gtk.NewLabel(fmt.Sprintf("%d", len(changes)))
					badge.AddCSSClass("accent")
					badge.AddCSSClass("caption")
					badge.SetVAlign(gtk.AlignCenter)
					r.AddSuffix(badge)
				})
			}
		}(repoPath, row)

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

	// "Add Remote" button on the Remote branches expander header.
	addRemoteBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	addRemoteBtn.SetTooltipText("Add Remote")
	addRemoteBtn.AddCSSClass("flat")
	addRemoteBtn.SetVAlign(gtk.AlignCenter)
	addRemoteBtn.ConnectClicked(func() {
		if s.onAddRemote != nil {
			s.onAddRemote()
		}
	})
	remoteExpander.AddSuffix(addRemoteBtn)

	// Separate local and remote branches.
	var localBranches []git.BranchInfo
	var remoteBranches []git.BranchInfo
	for _, b := range branches {
		if b.IsRemote {
			remoteBranches = append(remoteBranches, b)
		} else {
			localBranches = append(localBranches, b)
		}
	}

	// Sort local branches by cfg.BranchOrder, then alphabetically.
	localBranches = s.sortBranchesByOrder(localBranches)

	for i, branch := range localBranches {
		isCurrent := branch.Name == currentBranch
		branchName := branch.Name

		var actions BranchActions
		if !isCurrent {
			actions.OnDelete = func() {
				if s.onBranchDelete != nil {
					s.onBranchDelete(branchName)
				}
			}
			actions.OnMerge = func() {
				if s.onBranchMerge != nil {
					s.onBranchMerge(branchName)
				}
			}
			actions.OnRebase = func() {
				if s.onBranchRebase != nil {
					s.onBranchRebase(branchName)
				}
			}
		}

		row := NewBranchRow(branch, isCurrent, actions)
		row.ConnectActivated(func() {
			if s.onBranchSelected != nil {
				s.onBranchSelected(branchName, false)
			}
		})

		// Add drag-and-drop for reordering.
		s.addDragAndDrop(row, branchName)

		// Add some spacing at the end of the list.
		if i == len(localBranches) - 1 {
			row.SetMarginBottom(12)
		}

		localExpander.AddRow(row)
	}

	for i, branch := range remoteBranches {
		branchName := branch.Name
		row := NewBranchRow(branch, false, BranchActions{})
		row.ConnectActivated(func() {
			if s.onBranchSelected != nil {
				s.onBranchSelected(branchName, true)
			}
		})

		// Add some spacing at the end of the list.
		if i == len(remoteBranches) - 1 {
			row.SetMarginBottom(12)
		}

		remoteExpander.AddRow(row)
	}

	localExpander.SetSubtitle(formatCount(len(localBranches)))
	remoteExpander.SetSubtitle(formatCount(len(remoteBranches)))

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

	for i, tag := range tags {
		tagName := tag.Name
		onTagDelete := func() {
			if s.onTagDelete != nil {
				s.onTagDelete(tagName)
			}
		}
		row := NewTagRow(tag, onTagDelete)

		// Add some spacing at the end of the list.
		if i == len(tags) - 1 {
			row.SetMarginBottom(12)
		}

		tagsExpander.AddRow(row)
	}

	tagsExpander.SetSubtitle(formatCount(len(tags)))
	s.branchListBox.Append(tagsExpander)

	// Load submodules (between tags and remotes).
	submodules, err := s.repo.Submodules()
	if err != nil {
		slog.Warn("failed to load submodules", "error", err)
	}

	submodulesExpander := adw.NewExpanderRow()
	submodulesExpander.SetTitle("Submodules")
	submodulesExpander.SetIconName("package-x-generic-symbolic")
	submodulesExpander.SetExpanded(false)
	submodulesExpander.SetSubtitle(formatCount(len(submodules)))

	// "Update All" button.
	updateSubmodulesBtn := gtk.NewButtonFromIconName("view-refresh-symbolic")
	updateSubmodulesBtn.SetTooltipText("Update All Submodules")
	updateSubmodulesBtn.AddCSSClass("flat")
	updateSubmodulesBtn.SetVAlign(gtk.AlignCenter)
	updateSubmodulesBtn.ConnectClicked(func() {
		if s.onSubmoduleUpdate != nil {
			s.onSubmoduleUpdate()
		}
	})
	submodulesExpander.AddSuffix(updateSubmodulesBtn)

	// "Add Submodule" button.
	addSubmoduleBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	addSubmoduleBtn.SetTooltipText("Add Submodule")
	addSubmoduleBtn.AddCSSClass("flat")
	addSubmoduleBtn.SetVAlign(gtk.AlignCenter)
	addSubmoduleBtn.ConnectClicked(func() {
		if s.onSubmoduleAdd != nil {
			s.onSubmoduleAdd()
		}
	})
	submodulesExpander.AddSuffix(addSubmoduleBtn)

	for _, sm := range submodules {
		smPath := sm.Path
		smRow := adw.NewActionRow()
		smRow.SetTitle(sm.Name)
		subtitle := sm.Path
		if sm.URL != "" {
			subtitle = sm.Path + " — " + sm.URL
		}
		smRow.SetSubtitle(subtitle)
		smRow.SetIconName("package-x-generic-symbolic")

		// Remove button.
		removeBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
		removeBtn.SetTooltipText("Remove Submodule")
		removeBtn.AddCSSClass("flat")
		removeBtn.AddCSSClass("error")
		removeBtn.SetVAlign(gtk.AlignCenter)
		removeBtn.ConnectClicked(func() {
			if s.onSubmoduleRemove != nil {
				s.onSubmoduleRemove(smPath)
			}
		})
		smRow.AddSuffix(removeBtn)

		submodulesExpander.AddRow(smRow)
	}

	s.branchListBox.Append(submodulesExpander)

}

// openRepo opens a repository at the given path and notifies the callback.
func (s *Sidebar) openRepo(path string) {
	// Check if the path still exists on the filesystem.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		slog.Warn("repository no longer exists on disk", "path", path)
		s.cfg.RemoveRecentRepository(path)
		if err := s.cfg.Save(); err != nil {
			slog.Warn("failed to save config", "error", err)
		}
		s.RefreshRecent()

		// Show a notification via a temporary label.
		if s.onRepoRemoved != nil {
			s.onRepoRemoved(path)
		}
		return
	}

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

// sortBranchesByOrder returns local branches sorted by cfg.BranchOrder,
// with any remaining branches appended in their original order.
func (s *Sidebar) sortBranchesByOrder(branches []git.BranchInfo) []git.BranchInfo {
	order := s.cfg.BranchOrder
	if len(order) == 0 {
		return branches
	}

	// Build a position map from the config order.
	pos := make(map[string]int, len(order))
	for i, name := range order {
		pos[name] = i
	}

	// Stable-sort: ordered branches first, then the rest.
	ordered := make([]git.BranchInfo, 0, len(branches))
	unordered := make([]git.BranchInfo, 0)

	// Use a slice to preserve BranchOrder sequence.
	orderedMap := make(map[string]git.BranchInfo)
	for _, b := range branches {
		if _, ok := pos[b.Name]; ok {
			orderedMap[b.Name] = b
		} else {
			unordered = append(unordered, b)
		}
	}
	for _, name := range order {
		if b, ok := orderedMap[name]; ok {
			ordered = append(ordered, b)
		}
	}

	return append(ordered, unordered...)
}

// addDragAndDrop attaches GTK4 DragSource and DropTarget controllers to a
// branch row for reordering via drag-and-drop.
func (s *Sidebar) addDragAndDrop(row *adw.ActionRow, branchName string) {
	// DragSource: initiated when the user starts dragging this row.
	dragSrc := gtk.NewDragSource()
	dragSrc.SetActions(gdk.ActionMove)
	dragSrc.ConnectPrepare(func(x, y float64) *gdk.ContentProvider {
		val := coreglib.NewValue(branchName)
		return gdk.NewContentProviderForValue(val)
	})
	dragSrc.ConnectDragBegin(func(drag gdk.Dragger) {
		row.AddCSSClass("dragging")
	})
	dragSrc.ConnectDragEnd(func(drag gdk.Dragger, deleteData bool) {
		row.RemoveCSSClass("dragging")
	})
	row.AddController(dragSrc)

	// DropTarget: accepts drops from other branch rows.
	dropTarget := gtk.NewDropTarget(coreglib.TypeString, gdk.ActionMove)
	dropTarget.ConnectDrop(func(value *coreglib.Value, x, y float64) bool {
		draggedName, ok := value.GoValue().(string)
		if !ok || draggedName == branchName {
			return false
		}
		s.reorderBranch(draggedName, branchName)
		return true
	})
	dropTarget.ConnectEnter(func(x, y float64) gdk.DragAction {
		row.AddCSSClass("drop-target")
		return gdk.ActionMove
	})
	dropTarget.ConnectLeave(func() {
		row.RemoveCSSClass("drop-target")
	})
	row.AddController(dropTarget)
}

// reorderBranch moves draggedBranch to the position of targetBranch
// in the branch order, then persists and refreshes.
func (s *Sidebar) reorderBranch(draggedBranch, targetBranch string) {
	branches, err := s.repo.Branches()
	if err != nil {
		return
	}

	var local []git.BranchInfo
	for _, b := range branches {
		if !b.IsRemote {
			local = append(local, b)
		}
	}
	local = s.sortBranchesByOrder(local)

	var names []string
	for _, b := range local {
		names = append(names, b.Name)
	}

	// Remove dragged branch from its current position.
	var filtered []string
	for _, n := range names {
		if n != draggedBranch {
			filtered = append(filtered, n)
		}
	}

	// Insert before the target branch.
	var result []string
	for _, n := range filtered {
		if n == targetBranch {
			result = append(result, draggedBranch)
		}
		result = append(result, n)
	}

	s.cfg.BranchOrder = result
	if err := s.cfg.Save(); err != nil {
		slog.Warn("failed to save branch order", "error", err)
	}

	s.RefreshBranches()
}

// formatCount returns a human-readable count string for expander subtitles.
func formatCount(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
