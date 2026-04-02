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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MedaiP90/GiTK/config"
	"github.com/MedaiP90/GiTK/git"
	"github.com/MedaiP90/GiTK/ui/blame"
	"github.com/MedaiP90/GiTK/ui/commitdetail"
	"github.com/MedaiP90/GiTK/ui/commitlog"
	"github.com/MedaiP90/GiTK/ui/dialogs"
	"github.com/MedaiP90/GiTK/ui/filehistory"
	"github.com/MedaiP90/GiTK/ui/merge"
	"github.com/MedaiP90/GiTK/ui/rebase"
	"github.com/MedaiP90/GiTK/ui/sidebar"
	"github.com/MedaiP90/GiTK/ui/stash"
	"github.com/MedaiP90/GiTK/ui/staging"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
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

	// stashView is the stash management page.
	stashView *stash.StashView

	// blameView is the blame/annotate view.
	blameView *blame.BlameView

	// fileHistoryView is the per-file commit history view.
	fileHistoryView *filehistory.FileHistoryView

	// rebaseView is the interactive rebase UI.
	rebaseView *rebase.RebaseView

	// logBtn and stagingBtn are header bar toggle buttons, kept as fields
	// so we can update their active state.
	logBtn     *gtk.ToggleButton
	stagingBtn *gtk.ToggleButton
	stashBtn   *gtk.ToggleButton

	// stagingChip is the external pill label showing "N changes" next to stagingBtn.
	stagingChip *gtk.Label

	// stashChip is the external pill label showing "N stashed" next to stashBtn.
	stashChip *gtk.Label

	// repo is the currently open git repository (nil if none).
	repo *git.Repository

	// cfg is a reference to the app configuration for reading/writing prefs.
	cfg *config.Config

	// badgeStopCh signals the badge poll goroutine to stop.
	badgeStopCh chan struct{}
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

	// --- Load app icon ---
	// The icon lives under resources/hicolor/256x256/apps/<AppID>.png,
	// following the XDG icon theme hierarchy so GTK can resolve it by AppID.
	// We register the resources/ directory (and an exe-relative copy) as an
	// icon theme search path, then set the window icon by name.
	iconTheme := gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault())
	if exe, err := os.Executable(); err == nil {
		iconTheme.AddSearchPath(filepath.Join(filepath.Dir(exe), "resources"))
	}
	if cwd, err := os.Getwd(); err == nil {
		iconTheme.AddSearchPath(filepath.Join(cwd, "resources"))
	}
	w.window.SetIconName(AppID)

	// --- Load custom CSS ---
	cssProvider := gtk.NewCSSProvider()
	cssProvider.LoadFromString(`
.staging-btn-wrap { background: transparent; }
.changes-chip {
	border-radius: 12px;
	padding: 2px 8px;
	background-color: alpha(@accent_bg_color, 0.15);
	color: @accent_color;
	font-size: 0.8em;
}
.current-branch-chip {
	border-radius: 8px;
	padding: 1px 7px;
	background-color: @accent_bg_color;
	color: @accent_fg_color;
	font-weight: bold;
	font-size: 0.8em;
}`)
	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(),
		cssProvider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)

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

	// Provide window reference to views that need it for dialogs.
	w.stashView.SetWindow(w.window)

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
	// They are mutually exclusive — we use regular buttons styled as flat
	// to avoid the ToggleButton auto-toggle behavior that conflicts with
	// our manual active state management.
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
	// Group with staging button so GTK manages mutual exclusivity.
	header.PackStart(w.logBtn)

	// Staging button — plain icon-only toggle button.
	w.stagingBtn = gtk.NewToggleButton()
	w.stagingBtn.SetIconName("document-edit-symbolic")
	w.stagingBtn.SetTooltipText("Staging Area")
	w.stagingBtn.SetActive(false)
	w.stagingBtn.SetSensitive(false) // Disabled until a repo is selected.
	w.stagingBtn.SetGroup(w.logBtn) // Mutual exclusivity with log button.
	w.stagingBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.stagingView.SetRepository(w.repo)
			w.switchToView("staging")
		}
	})

	// External chip label showing "N changes" next to the staging button.
	w.stagingChip = gtk.NewLabel("")
	w.stagingChip.AddCSSClass("changes-chip")
	w.stagingChip.SetVisible(false)
	w.stagingChip.SetVAlign(gtk.AlignCenter)

	// Wrap button + chip in a horizontal box so they sit side by side.
	stagingWrap := gtk.NewBox(gtk.OrientationHorizontal, 4)
	stagingWrap.AddCSSClass("staging-btn-wrap")
	stagingWrap.SetVAlign(gtk.AlignCenter)
	stagingWrap.Append(w.stagingBtn)
	stagingWrap.Append(w.stagingChip)
	header.PackStart(stagingWrap)

	// Stash button — opens the stash management page.
	w.stashBtn = gtk.NewToggleButton()
	w.stashBtn.SetIconName("sidebar-show-symbolic")
	w.stashBtn.SetTooltipText("Stash")
	w.stashBtn.SetActive(false)
	w.stashBtn.SetSensitive(false) // Disabled until a repo is selected.
	w.stashBtn.SetGroup(w.logBtn) // Mutual exclusivity with log button.
	w.stashBtn.ConnectClicked(func() {
		if w.repo != nil {
			w.stashView.RefreshStashes()
			w.switchToView("stash")
		}
	})

	// External chip showing how many stash entries exist.
	w.stashChip = gtk.NewLabel("")
	w.stashChip.AddCSSClass("changes-chip")
	w.stashChip.SetVisible(false)
	w.stashChip.SetVAlign(gtk.AlignCenter)

	stashWrap := gtk.NewBox(gtk.OrientationHorizontal, 4)
	stashWrap.SetVAlign(gtk.AlignCenter)
	stashWrap.Append(w.stashBtn)
	stashWrap.Append(w.stashChip)
	header.PackStart(stashWrap)

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

	// For grouped toggle buttons, setting one active automatically
	// deactivates the other. Only set active if switching to log/staging.
	switch name {
	case "log":
		w.logBtn.SetActive(true)
	case "staging":
		w.stagingBtn.SetActive(true)
	default:
		// For other views (stash, merge, etc.), deactivate both.
		w.logBtn.SetActive(false)
		w.stagingBtn.SetActive(false)
	}
}

// buildPrimaryMenu creates the hamburger menu button (≡) with the app menu.
func (w *Window) buildPrimaryMenu() *gtk.MenuButton {
	// Build the menu model. GMenu is GTK's way of defining menus
	// declaratively — each item references a GAction by name.
	menu := newMenu()

	// Section 1: View / Git actions
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
	w.sidebar = sidebar.New(w.cfg, sidebar.SidebarCallbacks{
		OnRepoSelected:   w.onRepoSelected,
		OnBranchSelected: w.onBranchSelected,
		OnRepoRemoved: func(path string) {
			w.ShowToast("Repository removed: " + path + " (no longer exists on disk)")
		},
		OnBranchDelete:    w.onBranchDelete,
		OnTagDelete:       w.onTagDelete,
		OnBranchMerge:     w.onBranchMerge,
		OnBranchRebase:    w.onBranchRebase,
		OnAddRemote:       w.onAddRemote,
		OnSubmoduleAdd:    w.onSubmoduleAdd,
		OnSubmoduleRemove: w.onSubmoduleRemove,
		OnSubmoduleUpdate: w.onSubmoduleUpdate,
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
		// Show refs (branches/tags) for this commit.
		refs := w.commitLog.RefsForCommit(commit.Hash)
		w.commitDetail.SetRefs(refs)
	})

	// --- Commit detail panel ---
	w.commitDetail = commitdetail.New(w.cfg)
	w.commitDetail.SetFileCallbacks(
		func(path, hash string) {
			w.blameView.Load(path, hash)
			w.contentStack.SetVisibleChildName("blame")
		},
		func(path string) {
			w.fileHistoryView.Load(path)
			w.contentStack.SetVisibleChildName("filehistory")
		},
	)

	// Combine commit log + detail into a horizontal split.
	logDetailSplit := gtk.NewPaned(gtk.OrientationHorizontal)
	logDetailSplit.SetStartChild(w.commitLog.Root)
	logDetailSplit.SetEndChild(w.commitDetail.Root)
	logDetailSplit.SetPosition(700) // Initial split position.
	logDetailSplit.SetShrinkStartChild(false)
	logDetailSplit.SetShrinkEndChild(false)
	logDetailSplit.SetResizeStartChild(true)
	logDetailSplit.SetResizeEndChild(false)

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
		if w.repo != nil {
			dialogs.ShowStashDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
				w.stashView.RefreshStashes()
				w.updateStagingBadge()
			})
		}
	}, func() {
		// Changes updated — refresh the badge counter.
		w.updateStagingBadge()
	})
	w.contentStack.AddNamed(w.stagingView.Root, "staging")

	// --- Stash management page ---
	w.stashView = stash.New()
	w.contentStack.AddNamed(w.stashView.Root, "stash")

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

	// --- Blame view ---
	w.blameView = blame.New(func() {
		w.contentStack.SetVisibleChildName("log")
	})
	w.contentStack.AddNamed(w.blameView.Root, "blame")

	// --- File history view ---
	w.fileHistoryView = filehistory.New(
		func() {
			w.contentStack.SetVisibleChildName("log")
		},
		func(commit git.CommitInfo) {
			// Show the selected commit in the detail panel, highlight it in
			// the main commits table, and switch back to the log view.
			w.commitDetail.SetCommit(commit)
			refs := w.commitLog.RefsForCommit(commit.Hash)
			w.commitDetail.SetRefs(refs)
			w.commitLog.SelectByHash(commit.Hash)
			w.contentStack.SetVisibleChildName("log")
		},
	)
	w.contentStack.AddNamed(w.fileHistoryView.Root, "filehistory")

	// --- Interactive rebase view ---
	w.rebaseView = rebase.New(
		func() {
			w.contentStack.SetVisibleChildName("log")
		},
		func(msg string) {
			w.ShowToast(msg)
			if w.repo != nil {
				w.commitLog.SetRepository(w.repo)
			}
		},
	)
	w.contentStack.AddNamed(w.rebaseView.Root, "rebase")

	// Set the welcome page as the visible child.
	w.contentStack.SetVisibleChildName("welcome")

	// --- Resizable sidebar/content split ---
	// GtkPaned provides a draggable divider between sidebar and content.
	w.splitPane = gtk.NewPaned(gtk.OrientationHorizontal)
	w.sidebar.Root.SetSizeRequest(200, -1) // Minimum sidebar width.
	w.splitPane.SetStartChild(w.sidebar.Root)
	w.splitPane.SetEndChild(w.contentStack)
	w.splitPane.SetPosition(280)
	w.splitPane.SetShrinkStartChild(false)
	w.splitPane.SetShrinkEndChild(false)
	// Sidebar stays fixed; all extra space from window resize goes to content.
	w.splitPane.SetResizeStartChild(false)
	w.splitPane.SetResizeEndChild(true)

	// --- Toast overlay ---
	// Wraps everything to allow showing toast notifications.
	w.toastOverlay = adw.NewToastOverlay()
	w.toastOverlay.SetChild(w.splitPane)
}

// onRepoSelected is called by the sidebar when a repository is selected.
func (w *Window) onRepoSelected(repo *git.Repository) {
	// Stop existing badge poll.
	if w.badgeStopCh != nil {
		close(w.badgeStopCh)
	}

	w.repo = repo
	w.sidebar.SetRepository(repo)
	w.sidebar.CollapseRecentRepos()
	w.commitLog.SetRepository(repo)
	w.commitDetail.SetRepository(repo)
	w.stashView.SetRepository(repo)
	w.blameView.SetRepository(repo)
	w.fileHistoryView.SetRepository(repo)
	w.rebaseView.SetRepository(repo)
	w.window.SetTitle(repo.Name())

	// Enable view switcher buttons now that a repo is open.
	w.logBtn.SetSensitive(true)
	w.stagingBtn.SetSensitive(true)
	w.stashBtn.SetSensitive(true)

	w.switchToView("log")
	w.ShowToast("Opened " + repo.Name())
	w.updateStagingBadge()
	w.updateStashChip()

	// Start periodic badge polling to detect worktree changes.
	w.badgeStopCh = make(chan struct{})
	go w.badgePollLoop(w.badgeStopCh)

	slog.Info("repository selected", "path", repo.Path())
}

// badgePollLoop periodically polls repo.Status() and updates the badge.
// This detects external file edits that the git watcher can't see.
func (w *Window) badgePollLoop(stopCh chan struct{}) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if w.repo == nil {
				return
			}
			glib.IdleAdd(func() {
				w.updateStagingBadge()
				w.updateStashChip()
			})
		}
	}
}

// updateStagingBadge checks for uncommitted changes and updates
// the external chip label to show the change count.
func (w *Window) updateStagingBadge() {
	if w.repo == nil {
		w.stagingChip.SetVisible(false)
		return
	}

	go func() {
		changes, err := w.repo.Status()
		glib.IdleAdd(func() {
			if err != nil || len(changes) == 0 {
				w.stagingChip.SetVisible(false)
			} else {
				w.stagingChip.SetText(fmt.Sprintf("%d changes", len(changes)))
				w.stagingChip.SetVisible(true)
			}
		})
	}()
}

// onBranchSelected is called by the sidebar when a branch is clicked.
// For remote branches not yet available locally, it offers to create a tracking branch.
func (w *Window) onBranchSelected(branchName string, isRemote bool) {
	if w.repo == nil {
		return
	}
	slog.Info("branch selected", "name", branchName, "remote", isRemote)

	if isRemote {
		// Offer to create a local tracking branch instead of trying a raw checkout.
		dialog := adw.NewAlertDialog(
			"Create Local Branch",
			fmt.Sprintf("Create a local tracking branch for '%s' and check it out?", branchName),
		)
		dialog.AddResponse("cancel", "Cancel")
		dialog.AddResponse("track", "Create & Checkout")
		dialog.SetResponseAppearance("track", adw.ResponseSuggested)
		dialog.SetDefaultResponse("track")
		dialog.SetCloseResponse("cancel")
		dialog.ConnectResponse(func(response string) {
			if response != "track" {
				return
			}
			go func() {
				err := w.repo.CheckoutTrack(branchName)
				glib.IdleAdd(func() {
					if err != nil {
						slog.Warn("checkout track failed", "branch", branchName, "error", err)
						w.ShowToast("Checkout failed: " + err.Error())
						return
					}
					w.ShowToast("Switched to " + branchName)
					w.commitLog.SetRepository(w.repo)
					w.sidebar.RefreshBranches()
				})
			}()
		})
		dialog.Present(w.window)
		return
	}

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

// onBranchDelete is called by the sidebar when the user clicks the delete button on a branch.
func (w *Window) onBranchDelete(branchName string) {
	if w.repo == nil {
		return
	}

	deleteRemoteCheck := gtk.NewCheckButton()
	deleteRemoteCheck.SetLabel("Also delete remote ref (origin/" + branchName + ")")

	dialog := adw.NewAlertDialog(
		"Delete Branch",
		fmt.Sprintf("Delete branch '%s'? This cannot be undone.", branchName),
	)
	dialog.SetExtraChild(deleteRemoteCheck)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete")
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "delete" {
			return
		}
		alsoRemote := deleteRemoteCheck.Active()
		go func() {
			err := w.repo.DeleteBranch(branchName)
			glib.IdleAdd(func() {
				if err != nil {
					w.ShowToast("Delete failed: " + err.Error())
					return
				}
				w.ShowToast("Deleted branch " + branchName)
				w.sidebar.RefreshBranches()
			})
			if alsoRemote {
				remoteErr := w.repo.DeleteRemoteBranch("origin", branchName)
				glib.IdleAdd(func() {
					if remoteErr != nil {
						w.ShowToast("Remote delete failed: " + remoteErr.Error())
					} else {
						w.ShowToast("Deleted remote ref origin/" + branchName)
					}
				})
			}
		}()
	})
	dialog.Present(w.window)
}

// onTagDelete is called by the sidebar when the user clicks the delete button on a tag.
func (w *Window) onTagDelete(tagName string) {
	if w.repo == nil {
		return
	}

	deleteRemoteCheck := gtk.NewCheckButton()
	deleteRemoteCheck.SetLabel("Also delete from remote (origin)")

	dialog := adw.NewAlertDialog(
		"Delete Tag",
		fmt.Sprintf("Delete tag '%s'? This cannot be undone.", tagName),
	)
	dialog.SetExtraChild(deleteRemoteCheck)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete")
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "delete" {
			return
		}
		alsoRemote := deleteRemoteCheck.Active()
		go func() {
			err := w.repo.DeleteTag(tagName)
			glib.IdleAdd(func() {
				if err != nil {
					w.ShowToast("Delete failed: " + err.Error())
					return
				}
				w.ShowToast("Deleted tag " + tagName)
				w.sidebar.RefreshBranches()
			})
			if alsoRemote {
				remoteErr := w.repo.DeleteRemoteTag("origin", tagName)
				glib.IdleAdd(func() {
					if remoteErr != nil {
						w.ShowToast("Remote delete failed: " + remoteErr.Error())
					} else {
						w.ShowToast("Deleted remote tag origin/" + tagName)
					}
				})
			}
		}()
	})
	dialog.Present(w.window)
}

// onBranchMerge is called by the sidebar when the user requests merging a branch into the current one.
func (w *Window) onBranchMerge(branchName string) {
	if w.repo == nil {
		return
	}
	current := w.repo.CurrentBranch()

	dialog := adw.NewAlertDialog(
		"Merge Branch",
		fmt.Sprintf("Merge '%s' into '%s'?", branchName, current),
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("merge", "Merge")
	dialog.SetResponseAppearance("merge", adw.ResponseSuggested)
	dialog.SetDefaultResponse("merge")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "merge" {
			return
		}
		go func() {
			err := w.repo.MergeBranch(branchName)
			glib.IdleAdd(func() {
				if err != nil {
					msg := err.Error()
					w.ShowToast("Merge: " + msg)
					// If it's a conflict, switch to staging so user can resolve.
					if strings.Contains(msg, "conflict") || strings.Contains(msg, "CONFLICT") {
						w.stagingView.SetRepository(w.repo)
						w.switchToView("staging")
					}
					return
				}
				w.ShowToast("Merged " + branchName + " into " + current)
				w.commitLog.SetRepository(w.repo)
				w.sidebar.RefreshBranches()
			})
		}()
	})
	dialog.Present(w.window)
}

// onBranchRebase is called by the sidebar when the user wants to rebase onto a branch.
// It opens the rebase view and pre-fills the base ref with the tip of the selected branch.
func (w *Window) onBranchRebase(branchName string) {
	if w.repo == nil {
		return
	}
	w.rebaseView.SetRepository(w.repo)
	w.rebaseView.PrepareFromBranch(branchName)
	w.switchToView("rebase")
}

// onAddRemote opens the Add Remote dialog.
func (w *Window) onAddRemote() {
	if w.repo == nil {
		return
	}
	dialogs.ShowAddRemoteDialog(w.window, w.repo, func(msg string) {
		w.ShowToast(msg)
		w.sidebar.RefreshBranches()
	})
}

// onSubmoduleAdd opens the Add Submodule dialog.
func (w *Window) onSubmoduleAdd() {
	if w.repo == nil {
		return
	}
	dialogs.ShowAddSubmoduleDialog(w.window, w.repo, func(msg string) {
		w.ShowToast(msg)
		w.sidebar.RefreshBranches()
	})
}

// onSubmoduleRemove asks for confirmation and removes a submodule.
func (w *Window) onSubmoduleRemove(path string) {
	if w.repo == nil {
		return
	}
	dialogs.ShowRemoveSubmoduleConfirmDialog(w.window, path, func() {
		go func() {
			err := w.repo.RemoveSubmodule(path)
			glib.IdleAdd(func() {
				if err != nil {
					w.ShowToast("Remove submodule failed: " + err.Error())
					return
				}
				w.ShowToast("Submodule '" + path + "' removed")
				w.sidebar.RefreshBranches()
			})
		}()
	})
}

// onSubmoduleUpdate updates all submodules.
func (w *Window) onSubmoduleUpdate() {
	if w.repo == nil {
		return
	}
	go func() {
		err := w.repo.UpdateSubmodules()
		glib.IdleAdd(func() {
			if err != nil {
				w.ShowToast("Update submodules failed: " + err.Error())
				return
			}
			w.ShowToast("Submodules updated")
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

	// Create Branch action — used by commit detail action menu.
	createBranchAction := gio.NewSimpleAction("create-branch", glib.NewVariantType("s"))
	createBranchAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil {
			return
		}
		commitHash := ""
		if param != nil {
			commitHash = strings.Trim(param.String(), "'\"")
		}
		dialogs.ShowCreateBranchDialog(w.window, w.repo, commitHash, func(msg string) {
			w.ShowToast(msg)
			w.sidebar.RefreshBranches()
		})
	})
	w.window.AddAction(createBranchAction)

	// Reset Soft action — moves HEAD, keeps changes staged.
	resetSoftAction := gio.NewSimpleAction("reset-soft", glib.NewVariantType("s"))
	resetSoftAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil || param == nil {
			return
		}
		commitHash := strings.Trim(param.String(), "'\"")
		w.doReset(commitHash, git.ResetSoft, "Soft reset to "+commitHash[:7])
	})
	w.window.AddAction(resetSoftAction)

	// Reset Mixed action — moves HEAD, unstages changes.
	resetMixedAction := gio.NewSimpleAction("reset-mixed", glib.NewVariantType("s"))
	resetMixedAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil || param == nil {
			return
		}
		commitHash := strings.Trim(param.String(), "'\"")
		w.doReset(commitHash, git.ResetMixed, "Mixed reset to "+commitHash[:7])
	})
	w.window.AddAction(resetMixedAction)

	// Reset Hard action — moves HEAD and discards all changes. Shows confirmation.
	resetHardAction := gio.NewSimpleAction("reset-hard", glib.NewVariantType("s"))
	resetHardAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil || param == nil {
			return
		}
		commitHash := strings.Trim(param.String(), "'\"")
		dialog := adw.NewAlertDialog(
			"Hard Reset",
			"This will discard all uncommitted changes and reset the working tree to commit "+commitHash[:7]+". This cannot be undone.",
		)
		dialog.AddResponse("cancel", "Cancel")
		dialog.AddResponse("reset", "Reset Hard")
		dialog.SetResponseAppearance("reset", adw.ResponseDestructive)
		dialog.SetDefaultResponse("cancel")
		dialog.SetCloseResponse("cancel")
		dialog.ConnectResponse(func(response string) {
			if response == "reset" {
				w.doReset(commitHash, git.ResetHard, "Hard reset to "+commitHash[:7])
			}
		})
		dialog.Present(w.window)
	})
	w.window.AddAction(resetHardAction)

	// Cherry-pick action — applies a commit onto the current branch.
	cherryPickAction := gio.NewSimpleAction("cherry-pick", glib.NewVariantType("s"))
	cherryPickAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil || param == nil {
			return
		}
		commitHash := strings.Trim(param.String(), "'\"")
		dialog := adw.NewAlertDialog(
			"Cherry-pick commit?",
			"Apply the changes from "+commitHash[:7]+" onto the current branch.",
		)
		dialog.AddResponse("cancel", "Cancel")
		dialog.AddResponse("apply", "Cherry-pick")
		dialog.SetResponseAppearance("apply", adw.ResponseSuggested)
		dialog.SetDefaultResponse("apply")
		dialog.SetCloseResponse("cancel")
		dialog.ConnectResponse(func(response string) {
			if response != "apply" {
				return
			}
			go func() {
				err := w.repo.CherryPick(commitHash)
				glib.IdleAdd(func() {
					if err != nil {
						w.ShowToast("Cherry-pick failed: " + err.Error())
						return
					}
					w.ShowToast("Cherry-picked " + commitHash[:7])
					w.commitLog.SetRepository(w.repo)
					w.updateStagingBadge()
				})
			}()
		})
		dialog.Present(w.window)
	})
	w.window.AddAction(cherryPickAction)

	// Rebase action — opens the interactive rebase view pre-filled from a commit.
	rebaseAction := gio.NewSimpleAction("rebase", glib.NewVariantType("s"))
	rebaseAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil || param == nil {
			return
		}
		commitHash := strings.Trim(param.String(), "'\"")
		// Use the commit hash as the base — user can refine in the rebase UI.
		w.rebaseView.SetRepository(w.repo)
		w.rebaseView.PrepareFromHash(commitHash)
		w.contentStack.SetVisibleChildName("rebase")
	})
	w.window.AddAction(rebaseAction)

	// Open rebase view action — for toolbar/menu access.
	openRebaseAction := gio.NewSimpleAction("open-rebase", nil)
	openRebaseAction.ConnectActivate(func(param *glib.Variant) {
		if w.repo == nil {
			return
		}
		w.rebaseView.SetRepository(w.repo)
		w.contentStack.SetVisibleChildName("rebase")
	})
	w.window.AddAction(openRebaseAction)

}

// updateStashChip refreshes the stash count chip in the toolbar.
func (w *Window) updateStashChip() {
	if w.repo == nil {
		w.stashChip.SetVisible(false)
		return
	}
	go func() {
		stashes, err := w.repo.StashList()
		glib.IdleAdd(func() {
			if err != nil || len(stashes) == 0 {
				w.stashChip.SetVisible(false)
			} else {
				w.stashChip.SetText(fmt.Sprintf("%d stashed", len(stashes)))
				w.stashChip.SetVisible(true)
			}
		})
	}()
}

// doReset performs a git reset to the given commit hash with the specified mode.
func (w *Window) doReset(commitHash string, mode git.ResetMode, successMsg string) {
	go func() {
		err := w.repo.Reset(commitHash, mode)
		glib.IdleAdd(func() {
			if err != nil {
				w.ShowToast("Reset failed: " + err.Error())
				return
			}
			w.ShowToast(successMsg)
			w.commitLog.SetRepository(w.repo)
			w.sidebar.RefreshBranches()
			w.updateStagingBadge()
		})
	}()
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
