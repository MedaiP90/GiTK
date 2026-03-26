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
	"fmt"
	"log/slog"
	"strings"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/MedaiP90/GiTK/ui/commitdetail"
	"github.com/MedaiP90/GiTK/ui/commitlog"
	"github.com/MedaiP90/GiTK/ui/dialogs"
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
	// gitkApp is a back-reference to the parent app for theme switching etc.
	gitkApp *GiTKApp

	// window is the AdwApplicationWindow — the root GTK window.
	window *adw.ApplicationWindow

	// headerBar is the top header bar with title and action buttons.
	headerBar *adw.HeaderBar

	// toastOverlay wraps the main content and provides a place to show
	// non-blocking toast notifications (e.g., "Pushed to origin/main").
	toastOverlay *adw.ToastOverlay

	// splitPane is the sidebar/content split using a resizable pane.
	splitPane *gtk.Paned

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

	// stagingView is the staging area view.
	stagingView *staging.StagingView

	// mergeView is the three-pane merge editor.
	mergeView *merge.MergeView

	// logBtn and stagingBtn are header bar toggle buttons, kept as fields
	// so we can update their active state and add badges.
	logBtn       *gtk.ToggleButton
	stagingBtn   *gtk.ToggleButton
	stagingBadge *gtk.Label
	stashBtn     *gtk.Button

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
func NewWindow(gitkApp *GiTKApp, app *adw.Application, cfg *config.Config) *Window {
	w := &Window{
		gitkApp: gitkApp,
		cfg:     cfg,
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
//	[Open] [Clone]  |  [Log] [Staging] [Stash]    GiTK    [Fetch] [Pull] [Push]  [≡]
//
// The left side has buttons for opening/cloning repos and view switchers.
// The right side has remote operations and the primary menu.
func (w *Window) buildHeaderBar() *adw.HeaderBar {
	header := adw.NewHeaderBar()

	// --- Left side: Open and Clone buttons ---
	openBtn := gtk.NewButtonFromIconName("folder-open-symbolic")
	openBtn.SetTooltipText("Open Repository (Ctrl+O)")
	openBtn.ConnectClicked(func() {
		w.onOpenRepository()
	})
	header.PackStart(openBtn)

	// Clone button uses a download icon.
	cloneBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	cloneBtn.SetTooltipText("Clone Repository")
	cloneBtn.ConnectClicked(func() {
		w.onCloneRepository()
	})
	header.PackStart(cloneBtn)

	// --- Spacer between clone and view switcher ---
	spacer := gtk.NewSeparator(gtk.OrientationVertical)
	spacer.SetMarginStart(6)
	spacer.SetMarginEnd(6)
	header.PackStart(spacer)

	// --- View switcher buttons ---
	// These toggle between the main views: Log and Staging.
	// They are mutually exclusive — clicking one deactivates the other.
	w.logBtn = gtk.NewToggleButton()
	w.logBtn.SetIconName("view-list-symbolic")
	w.logBtn.SetTooltipText("Commit Log")
	w.logBtn.SetActive(false)
	w.logBtn.SetSensitive(false) // Disabled until a repo is selected.
	w.logBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.switchToView("log")
		}
	})
	header.PackStart(w.logBtn)

	// Staging button with badge support.
	// Staging button with change count badge.
	w.stagingBtn = gtk.NewToggleButton()
	w.stagingBtn.SetTooltipText("Staging Area")
	w.stagingBtn.SetSensitive(false) // Disabled until a repo is selected.

	// Use a box with icon + badge label for the staging button.
	stagingBtnBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	stagingIcon := gtk.NewImageFromIconName("document-edit-symbolic")
	stagingBtnBox.Append(stagingIcon)
	w.stagingBadge = gtk.NewLabel("")
	w.stagingBadge.AddCSSClass("accent")
	w.stagingBadge.AddCSSClass("caption")
	w.stagingBadge.SetVisible(false)
	stagingBtnBox.Append(w.stagingBadge)
	w.stagingBtn.SetChild(stagingBtnBox)

	w.stagingBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.stagingView.SetRepository(w.repo)
			w.switchToView("staging")
		}
	})
	header.PackStart(w.stagingBtn)

	// Stash button — opens the stash management page.
	w.stashBtn = gtk.NewButtonFromIconName("sidebar-show-symbolic")
	w.stashBtn.SetTooltipText("Stash")
	w.stashBtn.SetSensitive(false) // Disabled until a repo is selected.
	w.stashBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.switchToView("stash")
		}
	})
	header.PackStart(w.stashBtn)

	// --- Right side: Primary menu ---
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

	pullBtn := gtk.NewButtonFromIconName("go-down-symbolic")
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

	// Fetch button in the toolbar for quick access.
	fetchBtn := gtk.NewButtonFromIconName("emblem-synchronizing-symbolic")
	fetchBtn.SetTooltipText("Fetch")
	fetchBtn.ConnectClicked(func() {
		if w.repo == nil {
			return
		}
		go func() {
			err := w.repo.Fetch()
			glib.IdleAdd(func() {
				if err != nil {
					w.ShowToast("Fetch failed: " + err.Error())
					return
				}
				w.ShowToast("Fetched from origin")
				w.sidebar.RefreshBranches()
				w.commitLog.SetRepository(w.repo)
			})
		}()
	})
	header.PackEnd(fetchBtn)

	return header
}

// switchToView switches the content stack to the named view and updates
// the toggle button states so only the active view's button is toggled.
func (w *Window) switchToView(name string) {
	w.contentStack.SetVisibleChildName(name)

	// Update toggle button active states to match the visible view.
	w.logBtn.SetActive(name == "log")
	w.stagingBtn.SetActive(name == "staging")
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
	w.sidebar = sidebar.New(w.cfg, w.onRepoSelected, w.onBranchSelected, func(path string) {
		w.ShowToast("Repository removed: " + path + " (no longer exists on disk)")
	})

	// --- Content area ---
	// The content area uses a GtkStack to switch between views:
	// "welcome" (no repo open), "log" (commit log),
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
	w.commitDetail = commitdetail.New(w.cfg)

	// Combine commit log + detail into a horizontal split.
	logDetailSplit := gtk.NewPaned(gtk.OrientationHorizontal)
	logDetailSplit.SetStartChild(w.commitLog.Root)
	logDetailSplit.SetEndChild(w.commitDetail.Root)
	logDetailSplit.SetPosition(700) // Initial split position.
	logDetailSplit.SetShrinkStartChild(false)
	logDetailSplit.SetShrinkEndChild(false)

	w.contentStack.AddNamed(logDetailSplit, "log")

	// --- Staging view ---
	w.stagingView = staging.New(w.cfg, func(hash string) {
		// After a commit, refresh the log.
		if w.repo != nil {
			w.commitLog.SetRepository(w.repo)
		}
		w.ShowToast("Committed " + hash[:7])
	}, func() {
		// Stash button in staging opens the stash dialog.
		dialogs.ShowStashDialog(w.window, func(msg string) {
			w.ShowToast(msg)
		})
	})
	w.contentStack.AddNamed(w.stagingView.Root, "staging")

	// --- Stash management page ---
	stashPage := adw.NewStatusPage()
	stashPage.SetTitle("Stash Management")
	stashPage.SetDescription("Stash operations are not yet supported by the go-git backend.\nThis feature will be available in a future release.")
	stashPage.SetIconName("sidebar-show-symbolic")
	w.contentStack.AddNamed(stashPage, "stash")

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

	// --- Resizable sidebar/content split ---
	// GtkPaned provides a draggable divider between sidebar and content.
	w.splitPane = gtk.NewPaned(gtk.OrientationHorizontal)
	w.splitPane.SetStartChild(w.sidebar.Root)
	w.splitPane.SetEndChild(w.contentStack)
	w.splitPane.SetPosition(280)
	w.splitPane.SetShrinkStartChild(false)
	w.splitPane.SetShrinkEndChild(false)

	// --- Toast overlay ---
	// Wraps everything to allow showing toast notifications.
	w.toastOverlay = adw.NewToastOverlay()
	w.toastOverlay.SetChild(w.splitPane)
}

// onRepoSelected is called by the sidebar when a repository is selected.
func (w *Window) onRepoSelected(repo *git.Repository) {
	w.repo = repo
	w.sidebar.SetRepository(repo)
	w.sidebar.CollapseRecentRepos()
	w.commitLog.SetRepository(repo)
	w.commitDetail.SetRepository(repo)
	w.window.SetTitle("GiTK — " + repo.Name())

	// Enable view switcher buttons now that a repo is open.
	w.logBtn.SetSensitive(true)
	w.stagingBtn.SetSensitive(true)
	w.stashBtn.SetSensitive(true)

	w.switchToView("log")
	w.ShowToast("Opened " + repo.Name())
	w.updateStagingBadge()
	slog.Info("repository selected", "path", repo.Path())
}

// updateStagingBadge checks for uncommitted changes and updates
// the staging button badge to show the change count.
func (w *Window) updateStagingBadge() {
	if w.repo == nil {
		w.stagingBadge.SetVisible(false)
		return
	}

	go func() {
		changes, err := w.repo.Status()
		glib.IdleAdd(func() {
			if err != nil || len(changes) == 0 {
				w.stagingBadge.SetVisible(false)
			} else {
				w.stagingBadge.SetText(fmt.Sprintf("%d", len(changes)))
				w.stagingBadge.SetVisible(true)
			}
		})
	}()
}

// onBranchSelected is called by the sidebar when a branch is clicked.
// It checks out the selected branch and refreshes the commit log.
func (w *Window) onBranchSelected(branchName string, isRemote bool) {
	if w.repo == nil {
		return
	}
	slog.Info("branch selected", "name", branchName, "remote", isRemote)

	go func() {
		err := w.repo.Checkout(branchName)
		glib.IdleAdd(func() {
			if err != nil {
				slog.Warn("checkout failed", "branch", branchName, "error", err)
				w.ShowToast("Checkout failed: " + err.Error())
				return
			}
			w.ShowToast("Switched to " + branchName)
			w.commitLog.SetRepository(w.repo)
			w.sidebar.RefreshBranches()
		})
	}()
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

	// Create Tag action — used by commit log right-click context menu.
	tagAction := gio.NewSimpleAction("create-tag", glib.NewVariantType("s"))
	tagAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil {
			return
		}
		commitHash := ""
		if param != nil {
			commitHash = strings.Trim(param.String(), "'\"")
		}
		dialogs.ShowCreateTagDialog(w.window, w.repo, commitHash, func(msg string) {
			w.ShowToast(msg)
			w.sidebar.RefreshBranches()
		})
	})
	w.window.AddAction(tagAction)

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
