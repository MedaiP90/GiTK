// Package app — window.go defines the main application window layout.
//
// The window uses the GNOME HIG NavigationSplitView pattern where each pane
// owns its own header bar.  There is no global header bar spanning both panes.
//
//	AdwApplicationWindow
//	 └─ AdwToastOverlay
//	      └─ AdwNavigationSplitView
//	           ├─ [sidebar] AdwNavigationPage
//	           │    └─ AdwToolbarView
//	           │         ├─ [top] AdwHeaderBar [Open][Clone] … [≡]
//	           │         └─ sidebar content (recent repos, branches)
//	           └─ [content] AdwNavigationPage
//	                └─ AdwToolbarView
//	                     ├─ [top] AdwHeaderBar [Log|Staging|Stash] [⟳][↓][↑]
//	                     └─ AdwViewStack
//	                          ├─ "log"     → AdwToolbarView → GtkPaned
//	                          ├─ "staging" → StagingView (AdwToolbarView → GtkPaned)
//	                          ├─ "stash"   → StashView
//	                          └─ sub-views (merge, blame, filehistory, rebase)
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
	"github.com/MedaiP90/GiTK/ui/staging"
	"github.com/MedaiP90/GiTK/ui/stash"
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

	// sidebarPage is the left NavigationPage that contains the sidebar.
	// We keep a reference to it so we can update the title when a repo is opened.
	sidebarPage *adw.NavigationPage

	// contentPage is the right NavigationPage that contains the main content area.
	// We keep a reference to it so we can update the title when a repo is opened.
	contentPage *adw.NavigationPage

	// toastOverlay wraps the main content and provides a place to show
	// non-blocking toast notifications (e.g., "Pushed to origin/main").
	toastOverlay *adw.ToastOverlay
	currentToast *adw.Toast

	// contentStack is the AdwViewStack that switches between the main views.
	// It is connected to an AdwViewSwitcher in the content header bar.
	contentStack *adw.ViewStack

	// statusPage is the welcome/empty state shown when no repository is open.
	statusPage *adw.StatusPage

	// sidebar is the left sidebar with repositories and branches.
	sidebar *sidebar.Sidebar

	// commitLog is the commit history table view.
	commitLog *commitlog.CommitLog

	// commitDetail is the commit detail panel (right side of the log pane).
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

	// fetchBtn, pullBtn, pushBtn are the remote-operation buttons in the
	// content header bar.  They are disabled until a repository is opened.
	fetchBtn *gtk.Button
	pullBtn  *gtk.Button
	pushBtn  *gtk.Button

	// progressSpinner is shown during remote operations.
	progressSpinner    *gtk.Spinner

	// stagingPage and stashPage are the AdwViewStackPage handles for the
	// staging and stash tabs; used to update their badge numbers.
	stagingPage *adw.ViewStackPage
	stashPage   *adw.ViewStackPage

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
	winW, winH := cfg.WindowWidth, cfg.WindowHeight
	if winW <= 0 {
		winW = 1200
	}
	if winH <= 0 {
		winH = 800
	}
	w.window.SetDefaultSize(winW, winH)

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
.fetching-spinner {
  color: @accent_color;
}
.dragging {
	opacity: 0.5;
}
.drop-target {
	background-color: alpha(@accent_color, 0.1);
}
.current-branch-chip {
	border-radius: 8px;
	padding: 1px 7px;
	background-color: @accent_bg_color;
	color: @accent_fg_color;
	font-weight: bold;
	font-size: 0.8em;
}
/* Remove all vertical padding from ColumnView cells so graph lines connect. */
columnview > listview > row > cell {
	padding-top: 0;
	padding-bottom: 0;
}
.text-cell {
	padding-top: 10px;
	padding-bottom: 10px;
}
columnview > listview > row {
	padding: 0;
	min-height: 0;
}`)
	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(),
		cssProvider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)

	// --- Build all views and the navigation layout ---
	// buildContentArea creates the sidebar, all content views, the
	// AdwNavigationSplitView, and the ToastOverlay.  It also injects the
	// sidebar header bar into sidebar.Root so the header lives inside the
	// NavigationSplitView (GNOME HIG per-pane header pattern).
	w.buildContentArea()

	// The ToastOverlay is the direct window content — no outer AdwToolbarView.
	// Each NavigationPage manages its own AdwToolbarView + AdwHeaderBar.
	w.window.SetContent(w.toastOverlay)

	// Register window-scope actions (e.g., win.open-repo, win.clone).
	w.registerWindowActions()

	// Provide window reference to views that need it for dialogs.
	w.stashView.SetWindow(w.window)

	// Save window size on close so it can be restored on next startup.
	w.window.ConnectCloseRequest(func() bool {
		width := w.window.Width()
		height := w.window.Height()
		cfg.WindowWidth = width
		cfg.WindowHeight = height
		if err := cfg.Save(); err != nil {
			slog.Warn("failed to save window size", "error", err)
		}
		return false // allow the window to close
	})

	slog.Info("main window created", "width", winW, "height", winH)

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
  // If a toast is already visible, dismiss it before showing the new one.
  if w.currentToast != nil {
    w.currentToast.Dismiss()
  }
  // Create and show the new toast.
	w.currentToast = adw.NewToast(message)
	// Toasts auto-dismiss after a few seconds. The default timeout is fine
	// for most messages.
	w.toastOverlay.AddToast(w.currentToast)
}

// buildSidebarHeader creates the AdwHeaderBar that lives inside the sidebar's
// AdwToolbarView (the left NavigationPage of the AdwNavigationSplitView).
//
// GNOME HIG per-pane header pattern: each NavigationPage has its own header bar.
// The sidebar header shows window-level actions (Open, Clone) and the primary
// menu; the content header (built in buildContentArea) shows view-switcher and
// remote-operation buttons.
//
// Layout:
//
//	[Open] [Clone]   Repositories   [≡ Menu]
//
// The sidebar header hides its end title buttons so that, when both panes are
// visible, the window decoration buttons appear only once (on the content side).
func (w *Window) buildSidebarHeader() *adw.HeaderBar {
	header := adw.NewHeaderBar()
	// On desktop with NavigationSplitView, hide the end (right-side) title
	// buttons on the sidebar header so decorations don't appear twice.
	header.SetShowEndTitleButtons(false)

	// --- Left: Open and Clone buttons (always available) ---
	openBtn := gtk.NewButtonFromIconName("folder-open-symbolic")
	openBtn.SetTooltipText("Open Repository (Ctrl+O)")
	openBtn.ConnectClicked(func() { w.onOpenRepository() })
	header.PackStart(openBtn)

	cloneBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	cloneBtn.SetTooltipText("Clone Repository")
	cloneBtn.ConnectClicked(func() { w.onCloneRepository() })
	header.PackStart(cloneBtn)

	// --- Right: Primary hamburger menu ---
	menuBtn := w.buildPrimaryMenu()
	header.PackEnd(menuBtn)

	return header
}

// buildContentHeader creates the AdwHeaderBar that lives inside the content
// NavigationPage's AdwToolbarView.  It uses AdwViewSwitcher as its title
// widget to provide tab-style switching between Log, Staging, and Stash.
// Remote-operation buttons (Fetch / Pull / Push) are packed on the right.
//
// Layout:
//
//	[Log | Staging | Stash]   [⟳ Fetch] [↓ Pull] [↑ Push]
//
// The content header hides its start title buttons (mirror of the sidebar
// header hiding its end title buttons).
func (w *Window) buildContentHeader(viewStack *adw.ViewStack) *adw.HeaderBar {
	bar := adw.NewHeaderBar()
	// Hide start (left-side) title buttons on the content header — they are
	// shown on the sidebar header instead.
	bar.SetShowStartTitleButtons(false)

	// --- Centre: AdwViewSwitcher connected to the view stack ---
	switcher := adw.NewViewSwitcher()
	switcher.SetStack(viewStack)
	switcher.SetPolicy(adw.ViewSwitcherPolicyWide)
	bar.SetTitleWidget(switcher)

	// --- Right: Remote-operation buttons (disabled until a repo is open) ---
	w.fetchBtn = gtk.NewButtonFromIconName("emblem-synchronizing-symbolic")
	w.fetchBtn.SetTooltipText("Fetch")
	w.fetchBtn.SetSensitive(false)
	w.fetchBtn.ConnectClicked(func() {
		w.doFetch()
	})

	w.pullBtn = gtk.NewButtonFromIconName("go-down-symbolic")
	w.pullBtn.SetTooltipText("Pull")
	w.pullBtn.SetSensitive(false)
	w.pullBtn.ConnectClicked(func() {
		if w.repo != nil {
			dialogs.ShowPullDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
				if w.repo != nil {
					w.commitLog.SetRepository(w.repo)
					if !strings.HasPrefix(msg, "Pull failed") {
						w.doFetch()
					}
				}
			})
		}
	})

	w.pushBtn = gtk.NewButtonFromIconName("send-to-symbolic")
	w.pushBtn.SetTooltipText("Push")
	w.pushBtn.SetSensitive(false)
	w.pushBtn.ConnectClicked(func() {
		if w.repo != nil {
			dialogs.ShowPushDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
				if !strings.HasPrefix(msg, "Push failed") {
					w.doFetch()
				}
			})
		}
	})

	// Spinner shown during remote operations (to the left of the buttons).
	w.progressSpinner = gtk.NewSpinner()
	w.progressSpinner.SetVisible(false)
	w.progressSpinner.SetVAlign(gtk.AlignCenter)
	w.progressSpinner.SetMarginEnd(12)
	w.progressSpinner.SetSpinning(false)
	w.progressSpinner.SetCSSClasses([]string{"fetching-spinner"})

	bar.PackEnd(w.pushBtn)
	bar.PackEnd(w.pullBtn)
	bar.PackEnd(w.fetchBtn)
	bar.PackEnd(w.progressSpinner)

	return bar
}

// doFetch runs a git fetch in the background with progress indication.
func (w *Window) doFetch() {
	if w.repo == nil {
		return
	}
	w.ShowToast("Fetching…")
	w.startProgress()
	go func() {
		err := w.repo.Fetch(w.cfg.Git.PruneOnFetch)
		glib.IdleAdd(func() {
			w.stopProgress()
			if err != nil {
				w.ShowToast("Fetch failed: " + err.Error())
				return
			}
			w.ShowToast("Fetched from origin")
			w.sidebar.RefreshBranches()
			w.commitLog.SetRepository(w.repo)
		})
	}()
}

// startProgress shows and begins pulsing the header progress bar.
func (w *Window) startProgress() {
	w.progressSpinner.SetVisible(true)
	w.progressSpinner.SetSpinning(true)
	w.progressSpinner.Start()
}

// stopProgress hides the progress bar and stops the pulse timer.
func (w *Window) stopProgress() {
	w.progressSpinner.Stop()
	w.progressSpinner.SetVisible(false)
	w.progressSpinner.SetSpinning(false)
}

// switchToView switches the AdwViewStack to the named child.
// The AdwViewSwitcher automatically reflects the active page — no manual
// button state management required.
func (w *Window) switchToView(name string) {
	w.contentStack.SetVisibleChildName(name)
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

// buildContentArea constructs the main layout using AdwNavigationSplitView
// (GNOME HIG sidebar pattern).
//
// Each NavigationPage owns its own AdwToolbarView + AdwHeaderBar — the
// header bars live *inside* the split view, not above it.
//
//	AdwNavigationSplitView
//	  ├─ [sidebar] AdwNavigationPage
//	  │    └─ AdwToolbarView
//	  │         ├─ [top] AdwHeaderBar [Open][Clone] … [≡]
//	  │         └─ sidebar content (recent repos, branches)
//	  └─ [content] AdwNavigationPage
//	       └─ AdwToolbarView
//	            ├─ [top] AdwHeaderBar [Log|Staging|Stash] … [⟳][↓][↑]
//	            └─ AdwViewStack
//	                 ├─ "log"     → AdwToolbarView → GtkPaned(table | detail)
//	                 ├─ "staging" → StagingView.Root
//	                 ├─ "stash"   → StashView.Root
//	                 ├─ "merge"   → MergeView.Root   (hidden from switcher)
//	                 ├─ "blame"   → BlameView.Root    (hidden from switcher)
//	                 ├─ "filehistory" → …             (hidden from switcher)
//	                 └─ "rebase"  → …                 (hidden from switcher)
func (w *Window) buildContentArea() {
	// --- Sidebar ---
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
	// Inject the sidebar header bar into the sidebar's AdwToolbarView so it
	// lives inside the NavigationSplitView rather than above it.
	w.sidebar.Root.AddTopBar(w.buildSidebarHeader())

	// --- AdwViewStack: main content switcher ---
	// AdwViewStack + AdwViewSwitcher is the GNOME HIG pattern for tab-style
	// navigation.  Pages added with AddTitledWithIcon appear as tabs in the
	// switcher; pages added with AddNamed (no title) are hidden from it and
	// switched programmatically (sub-views like blame, merge, rebase).
	w.contentStack = adw.NewViewStack()

	// Welcome/empty state — no title → not shown in switcher.
	w.statusPage = adw.NewStatusPage()
	w.statusPage.SetTitle("Welcome to GiTK")
	w.statusPage.SetDescription("Open or clone a Git repository to get started")
	w.statusPage.SetIconName("vcs-branch-symbolic")
	w.contentStack.AddNamed(w.statusPage, "welcome")

	// --- Commit log view ---
	// Build the commit log table widget (search bar + GtkColumnView).
	w.commitLog = commitlog.New(func(commit git.CommitInfo) {
		w.commitDetail.SetCommit(commit)
		refs := w.commitLog.RefsForCommit(commit.Hash)
		w.commitDetail.SetRefs(refs)
	})

	// Build the commit detail panel (right side of the log pane).
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

	// The log page follows the same GtkPaned pattern as the staging area:
	//   AdwToolbarView
	//     └─ GtkPaned (horizontal)
	//          ├─ [start] commit table (more space — position 700)
	//          └─ [end]   commit detail (less space, stays fixed)
	logPane := gtk.NewPaned(gtk.OrientationHorizontal)
	logPane.SetStartChild(w.commitLog.Root)
	logPane.SetEndChild(w.commitDetail.Root)
	logPane.SetResizeStartChild(true)
	logPane.SetResizeEndChild(false)

	logToolbarView := adw.NewToolbarView()
	logToolbarView.SetContent(logPane)

	w.contentStack.AddTitledWithIcon(logToolbarView, "log", "History", "view-list-symbolic")

	// --- Staging view ---
	w.stagingView = staging.New(w.cfg, func(hash string) {
		if w.repo != nil {
			w.commitLog.SetRepository(w.repo)
		}
		w.ShowToast("Committed " + hash[:7])
	}, func() {
		if w.repo != nil {
			dialogs.ShowStashDialog(w.window, w.repo, func(msg string) {
				w.ShowToast(msg)
				w.stashView.RefreshStashes()
				w.updateStagingBadge()
			})
		}
	}, func() {
		w.updateStagingBadge()
	}, func() {
		// Resolve Conflicts button clicked in staging banner.
		w.openMergeForConflicts()
	}, func() {
		// Abort button clicked in staging banner.
		w.abortCurrentOperation()
	}, func() {
		// Skip button clicked in staging banner (rebase only).
		w.skipRebaseConflict()
	}, w.ShowToast)
	w.stagingPage = w.contentStack.AddTitledWithIcon(
		w.stagingView.Root, "staging", "Staging", "document-edit-symbolic",
	)

	// --- Stash management page ---
	w.stashView = stash.New()
	w.stashPage = w.contentStack.AddTitledWithIcon(
		w.stashView.Root, "stash", "Stash", "sidebar-show-symbolic",
	)

	// Hook: when the user switches to staging or stash tabs via the switcher,
	// trigger the lazy-load that was previously in the toggle button handlers.
	w.contentStack.NotifyProperty("visible-child-name", func() {
		name := w.contentStack.VisibleChildName()
		if w.repo == nil {
			return
		}
		switch name {
		case "staging":
			w.stagingView.SetRepository(w.repo)
		case "stash":
			w.stashView.RefreshStashes()
		}
	})

	// --- Sub-views (not shown in AdwViewSwitcher) ---
	// Merge view (opens in its own window).
	w.mergeView = merge.New(
		w.window,
		func(path string, content string) {
			go func() {
				err := w.repo.MarkResolved(path, content)
				glib.IdleAdd(func() {
					if err != nil {
						w.ShowToast("Failed to write resolved file: " + err.Error())
						return
					}
					slog.Info("merge resolved", "path", path)
					w.ShowToast("Resolved " + path)
					// Check for remaining conflicts.
					remaining, _ := w.repo.ConflictedFiles()
					if len(remaining) > 0 {
						w.openMergeForConflicts()
					} else {
						w.mergeView.Close()
						w.onAllConflictsResolved()
					}
				})
			}()
		},
		func() {
			go func() {
				// Abort either merge or rebase depending on repo state.
				state := w.repo.State()
				var err error
				if state == git.StateRebasing {
					err = w.repo.AbortRebase()
				} else {
					err = w.repo.AbortMerge()
				}
				glib.IdleAdd(func() {
					if err != nil {
						w.ShowToast("Abort failed: " + err.Error())
					} else {
						w.ShowToast("Operation aborted")
					}
					w.mergeView.Close()
					w.commitLog.Refresh()
					w.sidebar.RefreshBranches()
					w.stagingView.Refresh()
				})
			}()
		},
	)

	// Blame view.
	w.blameView = blame.New(func() {
		w.contentStack.SetVisibleChildName("log")
	})
	w.contentStack.AddNamed(w.blameView.Root, "blame")

	// File history view.
	w.fileHistoryView = filehistory.New(
		func() {
			w.contentStack.SetVisibleChildName("log")
		},
		func(commit git.CommitInfo) {
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
			// Check if the rebase paused due to conflicts.
			if strings.HasPrefix(msg, "rebase-conflict:") {
				w.ShowToast("Rebase paused — resolve conflicts to continue")
				w.openMergeForConflicts()
				return
			}
			w.ShowToast(msg)
			if w.repo != nil {
				w.commitLog.SetRepository(w.repo)
				w.sidebar.RefreshBranches()
			}
		},
	)
	w.contentStack.AddNamed(w.rebaseView.Root, "rebase")

	// Set the welcome page as the visible child.
	w.contentStack.SetVisibleChildName("welcome")

	// --- Content pane: view-switcher header + view stack ---
	// AdwViewSwitcher is passed the view stack and placed as the title widget
	// of the content header bar. Wrapping in AdwToolbarView gives proper
	// header-bar styling and ensures the stack fills the remaining height.
	contentHeader := w.buildContentHeader(w.contentStack)
	contentToolbarView := adw.NewToolbarView()
	contentToolbarView.AddTopBar(contentHeader)
	contentToolbarView.SetContent(w.contentStack)

	// --- AdwNavigationSplitView (GNOME HIG sidebar pattern) ---
	// Each NavigationPage owns its own AdwToolbarView + AdwHeaderBar, so
	// the header bars live *inside* the split view — not above it.
	navSplit := adw.NewNavigationSplitView()
	navSplit.SetMinSidebarWidth(300)
	navSplit.SetMaxSidebarWidth(400)
	navSplit.SetSidebarWidthFraction(0.25)

	w.sidebarPage = adw.NewNavigationPage(w.sidebar.Root, "Repositories")
	navSplit.SetSidebar(w.sidebarPage)

	w.contentPage = adw.NewNavigationPage(contentToolbarView, "")
	navSplit.SetContent(w.contentPage)

	// --- Toast overlay (direct window content — no outer AdwToolbarView) ---
	w.toastOverlay = adw.NewToastOverlay()
	w.toastOverlay.SetChild(navSplit)
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
	w.sidebarPage.SetTitle(repo.Name())
	w.contentPage.SetTitle(repo.Name())

	// Enable remote-operation buttons now that a repo is open.
	w.fetchBtn.SetSensitive(true)
	w.pullBtn.SetSensitive(true)
	w.pushBtn.SetSensitive(true)

	w.switchToView("log")
	w.ShowToast("Opened " + repo.Name())
	w.updateStagingBadge()
	w.updateStashChip()

	// Start periodic badge polling to detect worktree changes.
	w.badgeStopCh = make(chan struct{})
	go w.badgePollLoop(w.badgeStopCh)

	// Fetch from origin in the background so the view is up-to-date.
	go func() {
		err := repo.Fetch(w.cfg.Git.PruneOnFetch)
		glib.IdleAdd(func() {
			if err != nil {
				slog.Debug("fetch on open failed", "error", err)
				return
			}
			w.sidebar.RefreshBranches()
			w.commitLog.Refresh()
		})
	}()

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

// updateStagingBadge checks for uncommitted changes and updates the badge
// number on the Staging tab in the AdwViewSwitcher.
func (w *Window) updateStagingBadge() {
	if w.repo == nil {
		w.stagingPage.SetBadgeNumber(0)
		return
	}

	go func() {
		changes, err := w.repo.Status()
		glib.IdleAdd(func() {
			if err != nil || len(changes) == 0 {
				w.stagingPage.SetBadgeNumber(0)
			} else {
				w.stagingPage.SetBadgeNumber(uint(len(changes)))
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
			w.switchBranch(branchName, func() error {
				return w.repo.CheckoutTrack(branchName)
			})
		})
		dialog.Present(w.window)
		return
	}

	w.switchBranch(branchName, func() error {
		return w.repo.Checkout(branchName)
	})
}

// switchBranch checks for uncommitted changes before switching branches.
// If the worktree is dirty it asks the user whether to stash, switch, and reapply.
// checkoutFn performs the actual checkout (Checkout or CheckoutTrack).
func (w *Window) switchBranch(branchName string, checkoutFn func() error) {
	go func() {
		changes, _ := w.repo.Status()
		glib.IdleAdd(func() {
			if len(changes) == 0 {
				w.doSwitchBranch(branchName, checkoutFn, false)
				return
			}
			// Uncommitted changes: ask the user.
			dialog := adw.NewAlertDialog(
				"Uncommitted Changes",
				"You have uncommitted changes. Stash them, switch branch, and reapply?",
			)
			dialog.AddResponse("cancel", "Cancel")
			dialog.AddResponse("stash", "Stash & Switch")
			dialog.SetResponseAppearance("stash", adw.ResponseSuggested)
			dialog.SetDefaultResponse("stash")
			dialog.SetCloseResponse("cancel")
			dialog.ConnectResponse(func(response string) {
				if response != "stash" {
					return
				}
				w.doSwitchBranch(branchName, checkoutFn, true)
			})
			dialog.Present(w.window)
		})
	}()
}

// doSwitchBranch performs the branch switch, optionally stashing and reapplying changes.
func (w *Window) doSwitchBranch(branchName string, checkoutFn func() error, stash bool) {
	go func() {
		if stash {
			if err := w.repo.StashSave("Auto-stash before switching to " + branchName); err != nil {
				glib.IdleAdd(func() {
					w.ShowToast("Stash failed: " + err.Error())
				})
				return
			}
		}

		err := checkoutFn()
		if err != nil {
			glib.IdleAdd(func() {
				slog.Warn("checkout failed", "branch", branchName, "error", err)
				w.ShowToast("Checkout failed: " + err.Error())
			})
			return
		}

		var popErr error
		if stash {
			popErr = w.repo.StashPop(0)
		}

		glib.IdleAdd(func() {
			if popErr != nil {
				w.ShowToast("Switched to " + branchName + " (stash conflicts — changes remain in stash)")
			} else {
				w.ShowToast("Switched to " + branchName)
			}
			w.commitLog.SetRepository(w.repo)
			w.sidebar.RefreshBranches()
			w.updateStagingBadge()
			w.updateStashChip()
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
				w.commitLog.Refresh()
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
				w.commitLog.Refresh()
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
					// If it's a conflict, open the merge tool.
					if strings.Contains(msg, "conflict") || strings.Contains(msg, "CONFLICT") {
						w.commitLog.Refresh()
						w.openMergeForConflicts()
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

// onAllConflictsResolved is called when the last conflicted file has been
// resolved. If a rebase is in progress, it continues the rebase; otherwise
// it switches to the staging view for the user to commit the merge.
func (w *Window) onAllConflictsResolved() {
	state := w.repo.State()
	if state == git.StateRebasing {
		// Continue the rebase in background.
		go func() {
			err := w.repo.ContinueRebase()
			glib.IdleAdd(func() {
				if err != nil {
					msg := err.Error()
					// Rebase may pause again on the next commit.
					if w.repo.State() == git.StateRebasing {
						w.ShowToast("Rebase paused again — resolve next conflict")
						w.openMergeForConflicts()
						return
					}
					w.ShowToast("Rebase continue failed: " + msg)
				} else {
					w.ShowToast("Rebase completed")
				}
				w.commitLog.Refresh()
				w.sidebar.RefreshBranches()
				w.switchToView("log")
			})
		}()
		return
	}

	// Normal merge — go to staging so the user can commit.
	w.commitLog.Refresh()
	w.stagingView.SetRepository(w.repo)
	w.switchToView("staging")
}

// openMergeForConflicts detects conflicted files and opens the merge view
// for the first one. If an external merge tool is configured, it launches that
// instead.
func (w *Window) openMergeForConflicts() {
	go func() {
		files, err := w.repo.ConflictedFiles()
		if err != nil || len(files) == 0 {
			glib.IdleAdd(func() {
				// No conflicts detected or error — fall back to staging.
				w.stagingView.SetRepository(w.repo)
				w.switchToView("staging")
			})
			return
		}

		// Check if an external merge tool is configured.
		if w.cfg.MergeTool.UseExternal && w.cfg.MergeTool.ExternalCommand != "" {
			w.launchExternalMergeToolForFiles(files)
			return
		}

		// Use the integrated merge view.
		base, ours, theirs, err := w.repo.ConflictFileVersions(files[0])
		if err != nil {
			glib.IdleAdd(func() {
				w.ShowToast("Cannot read conflict versions: " + err.Error())
				w.stagingView.SetRepository(w.repo)
				w.switchToView("staging")
			})
			return
		}
		result := git.ThreeWayMerge(files[0], base, ours, theirs)
		glib.IdleAdd(func() {
			w.mergeView.SetConflictFiles(files)
			w.mergeView.SetMergeResult(&result)
			w.mergeView.Present()
		})
	}()
}

// abortCurrentOperation aborts the current merge or rebase.
// Can be called from the staging banner or merge view.
func (w *Window) abortCurrentOperation() {
	go func() {
		state := w.repo.State()
		var err error
		if state == git.StateRebasing {
			err = w.repo.AbortRebase()
		} else {
			err = w.repo.AbortMerge()
		}
		glib.IdleAdd(func() {
			if err != nil {
				w.ShowToast("Abort failed: " + err.Error())
			} else {
				w.ShowToast("Operation aborted")
			}
			w.commitLog.Refresh()
			w.sidebar.RefreshBranches()
			w.stagingView.SetRepository(w.repo)
			w.switchToView("log")
		})
	}()
}

// skipRebaseConflict skips the current conflicting commit during a rebase.
func (w *Window) skipRebaseConflict() {
	go func() {
		err := w.repo.SkipRebase()
		glib.IdleAdd(func() {
			if err != nil {
				msg := err.Error()
				// Skip may cause another conflict.
				if w.repo.State() == git.StateRebasing {
					files, _ := w.repo.ConflictedFiles()
					if len(files) > 0 {
						w.ShowToast("Rebase paused again on next commit")
						w.stagingView.SetRepository(w.repo)
						w.stagingView.Refresh()
						return
					}
				}
				w.ShowToast("Skip failed: " + msg)
			} else {
				w.ShowToast("Commit skipped")
			}
			w.commitLog.Refresh()
			w.sidebar.RefreshBranches()
			// Check if rebase is still in progress (more commits to apply).
			if w.repo.State() == git.StateRebasing {
				files, _ := w.repo.ConflictedFiles()
				if len(files) > 0 {
					w.stagingView.SetRepository(w.repo)
					w.stagingView.Refresh()
					return
				}
			}
			w.switchToView("log")
		})
	}()
}

// launchExternalMergeToolForFiles launches the external merge tool for each
// conflicted file in sequence.
func (w *Window) launchExternalMergeToolForFiles(files []string) {
	for _, file := range files {
		base, ours, theirs, err := w.repo.ConflictFileVersions(file)
		if err != nil {
			glib.IdleAdd(func() {
				w.ShowToast("Cannot read conflict versions: " + err.Error())
			})
			continue
		}
		merged, err := git.LaunchExternalMergeTool(
			w.cfg.MergeTool.ExternalCommand, w.repo.Path(), file, base, ours, theirs,
		)
		if err != nil {
			glib.IdleAdd(func() {
				w.ShowToast("External merge tool failed: " + err.Error())
			})
			continue
		}
		if err := w.repo.MarkResolved(file, merged); err != nil {
			glib.IdleAdd(func() {
				w.ShowToast("Failed to stage resolved file: " + err.Error())
			})
		}
	}
	glib.IdleAdd(func() {
		w.ShowToast("All conflicts resolved with external tool")
		w.commitLog.Refresh()
		w.sidebar.RefreshBranches()
		w.stagingView.SetRepository(w.repo)
		w.switchToView("staging")
	})
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
		w.commitLog.Refresh()
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
		w.commitLog.Refresh()
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
				w.commitLog.Refresh()
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
			w.commitLog.Refresh()
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
			w.commitLog.Refresh()
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
			w.commitLog.Refresh()
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

// updateStashChip refreshes the badge number on the Stash tab in the
// AdwViewSwitcher to reflect the current stash count.
func (w *Window) updateStashChip() {
	if w.repo == nil {
		w.stashPage.SetBadgeNumber(0)
		return
	}
	go func() {
		stashes, err := w.repo.StashList()
		glib.IdleAdd(func() {
			if err != nil || len(stashes) == 0 {
				w.stashPage.SetBadgeNumber(0)
			} else {
				w.stashPage.SetBadgeNumber(uint(len(stashes)))
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
