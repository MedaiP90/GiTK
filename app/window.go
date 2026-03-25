// Package app — window.go defines the main application window layout.
//
// The window uses the standard GNOME HIG layout:
//
//	┌──────────────────────────────────────────────────────┐
//	│  AdwHeaderBar  [Open] [Clone]    GiTK    [≡ Menu]   │
//	├────────────┬─────────────────────────────────────────┤
//	│  Sidebar   │  Content area                          │
//	│  (repos,   │  (commit log, staging, graph, etc.)    │
//	│  branches) │                                         │
//	│            │                                         │
//	└────────────┴─────────────────────────────────────────┘
//
// The root widget hierarchy is:
//
//	AdwApplicationWindow
//	 └─ AdwToolbarView
//	     ├─ [top] AdwHeaderBar
//	     └─ [content] AdwToastOverlay
//	          └─ AdwNavigationSplitView
//	               ├─ [sidebar] AdwNavigationPage → sidebar content
//	               └─ [content] AdwNavigationPage → main content stack
package app

import (
	"context"
	"log/slog"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/MedaiP90/GiTK/ui/commitdetail"
	"github.com/MedaiP90/GiTK/ui/commitlog"
	"github.com/MedaiP90/GiTK/ui/dialogs"
	"github.com/MedaiP90/GiTK/ui/graphview"
	"github.com/MedaiP90/GiTK/ui/merge"
	"github.com/MedaiP90/GiTK/ui/sidebar"
	"github.com/MedaiP90/GiTK/ui/staging"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Window is the main application window. It holds references to all major
// layout widgets so that other parts of the UI can update them.
type Window struct {
	// window is the AdwApplicationWindow — the root GTK window.
	window *adw.ApplicationWindow

	// headerBar is the top header bar with title and action buttons.
	headerBar *adw.HeaderBar

	// toastOverlay wraps the main content and provides a place to show
	// non-blocking toast notifications (e.g., "Pushed to origin/main").
	toastOverlay *adw.ToastOverlay

	// splitView is the sidebar/content split. On narrow screens, libadwaita
	// automatically collapses the sidebar into a navigation stack.
	splitView *adw.NavigationSplitView

	// contentStack switches between different views in the main content
	// area: commit log, staging area, graph view, etc.
	contentStack *gtk.Stack

	// statusPage is the welcome/empty state shown when no repository is open.
	statusPage *adw.StatusPage

	// sidebar is the left sidebar with repositories and branches.
	sidebar *sidebar.Sidebar

	// commitLog is the commit history table view.
	commitLog *commitlog.CommitLog

	// commitDetail is the commit detail panel (right side).
	commitDetail *commitdetail.CommitDetail

	// diffView is the inline diff viewer.
	diffView *commitdetail.DiffView

	// stagingView is the staging area view.
	stagingView *staging.StagingView

	// graphView is the visual DAG graph view.
	graphView *graphview.GraphView

	// mergeView is the three-pane merge editor.
	mergeView *merge.MergeView

	// repo is the currently open git repository (nil if none).
	repo *git.Repository

	// cfg is a reference to the app configuration for reading/writing prefs.
	cfg *config.Config
}

// NewWindow creates the main application window with the full GNOME HIG
// layout. It sets up the header bar, sidebar, content area, and toast
// overlay.
//
// Parameters:
//   - app: the adw.Application that owns this window.
//   - cfg: the user configuration (recent repos, preferences, etc.).
func NewWindow(app *adw.Application, cfg *config.Config) *Window {
	w := &Window{
		cfg: cfg,
	}

	// --- Create the AdwApplicationWindow ---
	// AdwApplicationWindow is required by GNOME HIG. Never use GtkWindow
	// directly in a libadwaita app.
	w.window = adw.NewApplicationWindow(&app.Application)
	w.window.SetTitle("GiTK")
	w.window.SetDefaultSize(1200, 800)

	// --- Build the header bar ---
	w.headerBar = w.buildHeaderBar()

	// --- Build the main content area ---
	w.buildContentArea()

	// --- Assemble the layout ---
	// AdwToolbarView hosts the header bar at the top and content below.
	// This is the recommended way to combine AdwHeaderBar with content
	// in GNOME HIG.
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(w.headerBar)
	toolbarView.SetContent(w.toastOverlay)

	// Set the toolbar view as the window's content.
	w.window.SetContent(toolbarView)

	// Register window-scope actions (e.g., win.open-repo, win.clone).
	w.registerWindowActions()

	slog.Info("main window created", "width", 1200, "height", 800)

	return w
}

// Present brings the window to the foreground. If the window is on another
// workspace or minimized, the window manager will make it visible.
func (w *Window) Present() {
	w.window.Present()
}

// ShowToast displays a non-blocking toast notification at the bottom of the
// window. Use this for success messages, warnings, and non-critical errors.
//
// Example: w.ShowToast("Pushed 3 commits to origin/main")
func (w *Window) ShowToast(message string) {
	toast := adw.NewToast(message)
	// Toasts auto-dismiss after a few seconds. The default timeout is fine
	// for most messages.
	w.toastOverlay.AddToast(toast)
}

// buildHeaderBar creates the AdwHeaderBar with action buttons.
//
// Layout:
//
//	[Open] [Clone]    GiTK    [View Switcher]    [≡]
//
// The left side has buttons for opening/cloning repos.
// The center shows the app title (or a view switcher when a repo is open).
// The right side has the primary menu.
func (w *Window) buildHeaderBar() *adw.HeaderBar {
	header := adw.NewHeaderBar()

	// --- Left side: Open and Clone buttons ---
	openBtn := gtk.NewButtonFromIconName("folder-open-symbolic")
	openBtn.SetTooltipText("Open Repository (Ctrl+O)")
	openBtn.ConnectClicked(func() {
		w.onOpenRepository()
	})
	header.PackStart(openBtn)

	cloneBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	cloneBtn.SetTooltipText("Clone Repository")
	cloneBtn.ConnectClicked(func() {
		w.onCloneRepository()
	})
	header.PackStart(cloneBtn)

	// --- View switcher buttons ---
	// These toggle between the main views: Log, Staging, Graph.
	logBtn := gtk.NewToggleButton()
	logBtn.SetIconName("view-list-symbolic")
	logBtn.SetTooltipText("Commit Log")
	logBtn.SetActive(true)
	logBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.contentStack.SetVisibleChildName("log")
		}
	})
	header.PackStart(logBtn)

	stagingBtn := gtk.NewToggleButton()
	stagingBtn.SetIconName("document-edit-symbolic")
	stagingBtn.SetTooltipText("Staging Area")
	stagingBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.stagingView.SetRepository(w.repo)
			w.contentStack.SetVisibleChildName("staging")
		}
	})
	header.PackStart(stagingBtn)

	graphBtn := gtk.NewToggleButton()
	graphBtn.SetIconName("view-app-grid-symbolic")
	graphBtn.SetTooltipText("Graph View")
	graphBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.graphView.LoadFromRepo(w.repo)
			w.contentStack.SetVisibleChildName("graph")
		}
	})
	header.PackStart(graphBtn)

	// --- Right side: Primary menu ---
	// The primary menu is the hamburger menu (≡) in the top-right corner.
	// It contains actions like Preferences, Keyboard Shortcuts, About.
	menuBtn := w.buildPrimaryMenu()
	header.PackEnd(menuBtn)

	// --- Right side: Remote operations ---
	pushBtn := gtk.NewButtonFromIconName("send-to-symbolic")
	pushBtn.SetTooltipText("Push")
	pushBtn.ConnectClicked(func() {
		if w.repo != nil {
			dialogs.ShowPushDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
			})
		}
	})
	header.PackEnd(pushBtn)

	pullBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	pullBtn.SetTooltipText("Pull")
	pullBtn.ConnectClicked(func() {
		if w.repo != nil {
			dialogs.ShowPullDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
				if w.repo != nil {
					w.commitLog.SetRepository(w.repo)
				}
			})
		}
	})
	header.PackEnd(pullBtn)

	return header
}

// buildPrimaryMenu creates the hamburger menu button (≡) with the app menu.
func (w *Window) buildPrimaryMenu() *gtk.MenuButton {
	// Build the menu model. GMenu is GTK's way of defining menus
	// declaratively — each item references a GAction by name.
	menu := newMenu()

	// Section 1: View actions
	viewSection := newMenu()
	viewSection.Append("Keyboard Shortcuts", "app.shortcuts")
	menu.AppendSection("", viewSection)

	// Section 2: Application actions
	appSection := newMenu()
	appSection.Append("Preferences", "app.preferences")
	appSection.Append("About GiTK", "app.about")
	appSection.Append("Quit", "app.quit")
	menu.AppendSection("", appSection)

	// Create the menu button with a hamburger icon.
	menuBtn := gtk.NewMenuButton()
	menuBtn.SetIconName("open-menu-symbolic")
	menuBtn.SetMenuModel(menu)
	menuBtn.SetTooltipText("Main Menu")
	// Use a popover (not a popup window) per GNOME HIG.
	menuBtn.SetPrimary(true)

	return menuBtn
}

// newMenu is a helper that creates a new GMenu (GIO menu model).
// GMenu is used by GTK to define menus declaratively — you add items
// that reference GAction names, and GTK handles rendering the menu.
func newMenu() *gio.Menu {
	return gio.NewMenu()
}

// buildContentArea constructs the main content area: a NavigationSplitView
// with a sidebar on the left and a content stack on the right, all wrapped
// in a ToastOverlay for notifications.
func (w *Window) buildContentArea() {
	// --- Sidebar ---
	// The sidebar shows recent repositories and branch tree.
	w.sidebar = sidebar.New(w.cfg, w.onRepoSelected, w.onBranchSelected)
	sidebarPage := adw.NewNavigationPage(w.sidebar.Root, "Repositories")

	// --- Content area ---
	// The content area uses a GtkStack to switch between views:
	// "welcome" (no repo open), "log" (commit log), "graph" (DAG view),
	// "staging" (staging area).
	w.contentStack = gtk.NewStack()
	w.contentStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

	// Welcome/empty state shown when no repository is loaded.
	w.statusPage = adw.NewStatusPage()
	w.statusPage.SetTitle("Welcome to GiTK")
	w.statusPage.SetDescription("Open or clone a Git repository to get started")
	w.statusPage.SetIconName("vcs-branch-symbolic")
	w.contentStack.AddNamed(w.statusPage, "welcome")

	// --- Commit log view (main view when a repo is open) ---
	// The log view shows a split between the commit table and detail panel.
	w.commitLog = commitlog.New(func(commit git.CommitInfo) {
		// When a commit is selected, show its details.
		w.commitDetail.SetCommit(commit)
	})

	// --- Commit detail panel ---
	w.commitDetail = commitdetail.New(func(diff git.DiffResult) {
		// When a file is selected, show the diff.
		w.diffView.SetDiff(diff)
		w.contentStack.SetVisibleChildName("diff")
	})

	// --- Diff view ---
	w.diffView = commitdetail.NewDiffView()

	// Combine commit log + detail into a horizontal split.
	logDetailSplit := gtk.NewPaned(gtk.OrientationHorizontal)
	logDetailSplit.SetStartChild(w.commitLog.Root)
	logDetailSplit.SetEndChild(w.commitDetail.Root)
	logDetailSplit.SetPosition(700) // Initial split position.
	logDetailSplit.SetShrinkStartChild(false)
	logDetailSplit.SetShrinkEndChild(false)

	w.contentStack.AddNamed(logDetailSplit, "log")
	w.contentStack.AddNamed(w.diffView.Root, "diff")

	// --- Staging view ---
	w.stagingView = staging.New(func(hash string) {
		// After a commit, refresh the log.
		if w.repo != nil {
			w.commitLog.SetRepository(w.repo)
		}
		w.ShowToast("Committed " + hash[:7])
	})
	w.contentStack.AddNamed(w.stagingView.Root, "staging")

	// --- Graph view ---
	w.graphView = graphview.New(func(gc git.GraphCommit) {
		slog.Info("graph commit selected", "hash", gc.ShortHash)
	})
	w.contentStack.AddNamed(w.graphView.Root, "graph")

	// --- Merge view ---
	w.mergeView = merge.New(
		func(path string, content string) {
			slog.Info("merge resolved", "path", path)
			w.ShowToast("Resolved " + path)
			w.contentStack.SetVisibleChildName("staging")
		},
		func() {
			slog.Info("merge aborted")
			w.ShowToast("Merge aborted")
			w.contentStack.SetVisibleChildName("log")
		},
	)
	w.contentStack.AddNamed(w.mergeView.Root, "merge")

	// Set the welcome page as the visible child.
	w.contentStack.SetVisibleChildName("welcome")

	contentPage := adw.NewNavigationPage(w.contentStack, "Content")

	// --- NavigationSplitView ---
	// This is the GNOME HIG way to do sidebar + content. On narrow screens
	// (e.g., phones or narrow windows), it automatically collapses into a
	// navigation stack where the sidebar slides away.
	w.splitView = adw.NewNavigationSplitView()
	w.splitView.SetSidebar(sidebarPage)
	w.splitView.SetContent(contentPage)
	w.splitView.SetMinSidebarWidth(200)
	w.splitView.SetMaxSidebarWidth(400)

	// --- Toast overlay ---
	// Wraps everything to allow showing toast notifications.
	w.toastOverlay = adw.NewToastOverlay()
	w.toastOverlay.SetChild(w.splitView)
}

// onRepoSelected is called by the sidebar when a repository is selected.
func (w *Window) onRepoSelected(repo *git.Repository) {
	w.repo = repo
	w.sidebar.SetRepository(repo)
	w.commitLog.SetRepository(repo)
	w.commitDetail.SetRepository(repo)
	w.window.SetTitle("GiTK — " + repo.Name())
	w.contentStack.SetVisibleChildName("log")
	w.ShowToast("Opened " + repo.Name())
	slog.Info("repository selected", "path", repo.Path())
}

// onBranchSelected is called by the sidebar when a branch is clicked.
func (w *Window) onBranchSelected(branchName string, isRemote bool) {
	if w.repo == nil {
		return
	}
	// TODO (Phase 7): Handle branch checkout/actions.
	slog.Info("branch selected", "name", branchName, "remote", isRemote)
}

// registerWindowActions registers GActions scoped to this window.
// These are triggered by keyboard shortcuts or menu items.
func (w *Window) registerWindowActions() {
	app := w.window.Application()

	// Ctrl+O: Open repository
	app.SetAccelsForAction("win.open-repo", []string{"<Control>o"})
	openAction := gio.NewSimpleAction("open-repo", nil)
	openAction.ConnectActivate(func(param *glib.Variant) {
		w.onOpenRepository()
	})
	w.window.AddAction(openAction)
}

// onOpenRepository handles the "Open Repository" action. It shows a native
// file chooser dialog (via xdg-desktop-portal on Linux) and opens the
// selected directory as a git repository.
func (w *Window) onOpenRepository() {
	// GtkFileDialog is the modern GTK4 way to show file choosers.
	// On Linux with xdg-desktop-portal, this automatically uses the
	// native GNOME file chooser.
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Open Git Repository")

	// We want to select a folder (git repos are directories).
	// SelectFolder takes a context.Context (for cancellation), the parent
	// window, and an async callback.
	dialog.SelectFolder(context.Background(), &w.window.Window, func(result gio.AsyncResulter) {
		file, err := dialog.SelectFolderFinish(result)
		if err != nil {
			// User cancelled the dialog — not an error.
			slog.Debug("file dialog cancelled", "error", err)
			return
		}

		path := file.Path()
		slog.Info("opening repository", "path", path)

		// Open the repository using the git backend.
		repo, err := git.OpenRepository(path)
		if err != nil {
			w.ShowToast("Not a Git repository: " + path)
			slog.Warn("failed to open repository", "path", path, "error", err)
			return
		}

		// Update config and UI.
		w.cfg.AddRecentRepository(path)
		if saveErr := w.cfg.Save(); saveErr != nil {
			slog.Warn("failed to save config", "error", saveErr)
		}
		w.onRepoSelected(repo)
	})
}

// onCloneRepository handles the "Clone Repository" action.
// Shows the clone dialog for entering the URL and destination.
func (w *Window) onCloneRepository() {
	dialogs.ShowCloneDialog(w.window, func(repo *git.Repository) {
		// Clone completed — treat it like opening a repo.
		w.cfg.AddRecentRepository(repo.Path())
		if err := w.cfg.Save(); err != nil {
			slog.Warn("failed to save config after clone", "error", err)
		}
		w.onRepoSelected(repo)
	})
}
