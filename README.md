# GiTK

A comprehensive, native GTK4 Git GUI client for GNOME, written in Go.

GiTK uses [gotk4](https://github.com/diamondburned/gotk4) for GTK4/libadwaita bindings and [go-git](https://github.com/go-git/go-git) for a pure-Go Git backend. It follows the [GNOME Human Interface Guidelines](https://developer.gnome.org/hig/) and is designed primarily for Linux/GNOME, with cross-platform potential.

---

## Features

### Repository Management
- Open local repositories via native file chooser (xdg-desktop-portal)
- Clone remote repositories with progress tracking (auto-creates subdirectory named after the project)
- Recent repositories list with quick access from the sidebar (auto-collapses on selection)
- Change indicator badge (with count) on the staging button and recent repos list
- Deleted repositories automatically detected and removed from recents
- `AdwNavigationSplitView` sidebar following GNOME HIG — collapsible on narrow screens, single header bar spanning full width
- Header bar with Open/Clone buttons on the left and hamburger menu on the right; view-switcher and remote ops move into the content area's secondary header
- Toolbar buttons disabled when no repository is selected
- XDG-compliant JSON configuration (`~/.config/gitk/config.json`)

### Commit History
- Commit log table using `GtkColumnView` with columns: Graph, Hash, Subject, Author, Date, Refs
- Inline graph column: Cairo `GtkDrawingArea` per row renders branch/merge topology — colored lane lines, bezier curves for lane changes, circles for regular commits, diamonds for merges, double-ring for HEAD
- Column reordering disabled — column order is fixed and intentional
- Commit detail panel showing full metadata, message, and changed file list with +/- stats
- Inline expandable diffs under each file in the commit detail panel with syntax-colored added/deleted/context lines
- Commit table resizes synchronously with the detail panel when the window is resized or the divider is dragged
- Actions menu button in commit detail for tag creation
- First commit auto-selected when entering the page
- Search filtering by message, author, or hash

### Staging Area
- Split view: unstaged files first, then staged files, with hunk-level diff
- Stage/unstage individual files or all at once
- Per-hunk staging/unstaging with colored diff display
- Discard individual file changes
- Commit message editor with subject line character counter (orange at 50, red at 72)
- AI-powered commit message generation button (when enabled in Preferences)
- Stash button for managing stashed changes
- Async commit creation with toast notifications

### Visual DAG Graph View
- Full commit graph drawn on a Cairo `GtkDrawingArea` inside a scrollable viewport
- Bezier curves for lane changes, diamonds for merge commits, double-ring for HEAD
- 8-color GNOME palette for lane coloring
- Zoom in/out (Ctrl+scroll or buttons) with clamped range (20-48px row height)
- Click-to-select commits with highlight ring
- Ref pills (branches, tags, remotes) with hit-testing

### Branch Management
- Sidebar branch tree with local, remote, tag, submodule, and remote sections using `AdwExpanderRow`
- Click-to-checkout branches
- Current branch indicator
- Delete local branches and tags with confirmation
- Merge any branch into the current branch with confirmation
- Drag-and-drop reordering for local branches (order saved in config)
- Add/remove remotes
- Submodule management: add, update all, remove (with confirmation)

### Remote Operations
- Push dialog with remote selection and force-push toggle (with destructive action confirmation via `AdwAlertDialog`)
- Pull dialog with remote and branch selection

### Commit Actions
- Create tag on any commit (lightweight or annotated)
- Create branch from any commit
- Cherry-pick any commit onto the current branch
- Reset current branch (soft/mixed/hard) to any commit
- View per-file blame/annotate for any commit
- View full file history for any file in a commit

### Tag & Stash
- Create tag dialog (lightweight or annotated) — tags are automatically pushed to the remote after creation
- Full stash management: save with message, apply, pop, drop
- Stash count chip in toolbar showing number of stashed entries

### Blame & File History
- Blame view: annotates each source line with the commit hash, author, date, and message
- File history view: shows the full commit log filtered to a single file (follows renames)

### Interactive Rebase
- Rebase UI: select a base commit, then reorder/squash/fixup/reword/drop commits
- Drag-and-drop or Up/Down buttons to reorder commits in the todo list
- Kicks off `git rebase -i` with a pre-built todo — no terminal editor needed

### Three-Way Merge Editor
- Three-pane layout: OURS (read-only) | RESULT (editable) | THEIRS (read-only)
- Conflict navigation (Previous/Next)
- Resolution strategies: Use Ours, Use Theirs, Both (Ours First), Both (Theirs First)
- Conflict counter with "Mark as Resolved" button
- Abort Merge with destructive action confirmation

### Preferences
- `AdwPreferencesWindow` with pages for Git identity/fetch settings and AI configuration
- Git author name/email with per-repo override toggle
- Dark mode enforced (no theme switching)
- Settings persist to JSON config on window close

### AI Commit Message Generation (Optional)
- Three providers: **Claude (Anthropic)**, **Google Gemini**, and **OpenCode Go**
- API keys configured directly in **Preferences > AI** (no environment variable required; env vars are used as a fallback)
- Provider and model selectable per-provider in Preferences
- Generates conventional commit messages from staged diffs
- Enable/disable from Preferences

### Keyboard Shortcuts
- `Ctrl+O` Open repository
- `Ctrl+Q` Quit
- `Ctrl+,` Preferences
- `Ctrl+?` Keyboard shortcuts window

---

## Prerequisites

### System Dependencies

**Ubuntu/Debian:**
```bash
sudo apt install -y \
  libgtk-4-dev \
  libadwaita-1-dev \
  gobject-introspection \
  libgirepository1.0-dev \
  libgraphene-1.0-dev \
  pkg-config \
  gcc
```

**Fedora:**
```bash
sudo dnf install -y \
  gtk4-devel \
  libadwaita-devel \
  gobject-introspection-devel \
  graphene-devel \
  pkg-config \
  gcc
```

**Arch Linux:**
```bash
sudo pacman -S gtk4 libadwaita gobject-introspection graphene pkg-config gcc
```

### Go

Go 1.24+ is required. Install from [go.dev/dl](https://go.dev/dl/) or via your package manager.

> **Note:** Ubuntu 24.04 ships libadwaita 1.5. GiTK is pinned to a gotk4-adwaita version compatible with libadwaita 1.5. If you have libadwaita 1.6+, you can update the dependency in `go.mod`.

---

## Getting Started

### Clone and Build

```bash
git clone https://github.com/MedaiP90/GiTK.git
cd GiTK
go mod download
make build
```

The binary is written to `build/gitk`.

### Run

```bash
make run
```

Or directly:
```bash
./build/gitk
```

### Install Locally

Installs the binary, desktop file, icon, and metainfo to `~/.local`:

```bash
make install
```

After installing, GiTK should appear in your application launcher. You may need to run:
```bash
update-desktop-database ~/.local/share/applications
```

### Uninstall

```bash
make uninstall
```

---

## Testing

### Unit Tests

```bash
make test
```

Or run specific packages:
```bash
go test ./git/... -v       # Git backend tests (merge, graph algorithms)
go test ./ai/... -v        # AI package tests
```

### Static Analysis

```bash
make vet                   # go vet
make lint                  # go vet + staticcheck (if installed)
```

### Manual Testing

1. Run the app with `make run`
2. Click **Open** (folder icon) and select a local Git repository
3. Browse the commit log, click commits to see details; click files to expand inline diffs
4. Use the actions menu (three dots) in commit details to create a tag
5. Switch to **Staging** (pencil icon) to see working tree changes; try per-hunk staging
6. Use **Push/Pull** buttons in the header bar for remote operations
7. Open the hamburger menu for Preferences and About

---

## Project Structure

```
GiTK/
├── main.go                          # Entry point
├── go.mod / go.sum                  # Go module definition
├── Makefile                         # Build, run, test, install targets
│
├── app/                             # Application lifecycle
│   ├── app.go                       #   AdwApplication setup, global actions
│   ├── window.go                    #   Main window layout, content wiring
│   └── shortcuts.go                 #   Keyboard shortcuts window (GtkBuilder XML)
│
├── config/
│   └── config.go                    # XDG-compliant JSON config (thread-safe)
│
├── git/                             # Git backend (ZERO GTK imports)
│   ├── repo.go                      #   Repository wrapper, state machine
│   ├── operations.go                #   commit, push, pull, branch, tag, etc.
│   ├── diff.go                      #   Diff computation with hunks
│   ├── merge.go                     #   Three-way merge + conflict resolution
│   ├── graph.go                     #   DAG layout + lane assignment
│   ├── watcher.go                   #   Filesystem polling watcher
│   ├── graph_test.go                #   Graph algorithm tests
│   └── merge_test.go                #   Merge + conflict resolution tests
│
├── ui/
│   ├── sidebar/
│   │   ├── sidebar.go               #   Recent repos list, branch tree
│   │   └── branchrow.go             #   Branch/tag row widgets
│   │
│   ├── commitlog/
│   │   ├── log.go                   #   GtkColumnView commit table
│   │   └── graphrenderer.go         #   Cairo graph column renderer
│   │
│   ├── commitdetail/
│   │   └── detail.go                #   Commit metadata + file list with inline diffs
│   │
│   ├── staging/
│   │   ├── staging.go               #   Staged/unstaged file lists + commit
│   │   └── hunkview.go              #   Per-hunk diff cards
│   │
│   ├── graphview/
│   │   ├── graphview.go             #   Full Cairo DAG view
│   │   ├── layout.go                #   Coordinate conversion + hit testing
│   │   └── node.go                  #   GraphNode UI model
│   │
│   ├── merge/
│   │   └── mergeview.go             #   Three-pane merge editor
│   │
│   ├── dialogs/
│   │   ├── clone.go                 #   Clone repository dialog
│   │   ├── remote.go                #   Push/Pull/Add-remote dialogs
│   │   ├── tag.go                   #   Create tag dialog
│   │   ├── stash.go                 #   Stash save dialog
│   │   ├── branch.go                #   Create branch dialog
│   │   └── submodule.go             #   Add/remove submodule dialogs
│   │
│   ├── stash/
│   │   └── stash.go                 #   Stash management page
│   │
│   ├── blame/
│   │   └── blameview.go             #   Blame/annotate view
│   │
│   ├── filehistory/
│   │   └── filehistory.go           #   Per-file commit history
│   │
│   ├── rebase/
│   │   └── rebaseview.go            #   Interactive rebase UI
│   │
│   └── prefs/
│       └── prefs.go                 #   AdwPreferencesWindow
│
├── ai/
│   ├── claude.go                    # Claude API client
│   ├── opencode.go                  # OpenCode Go API client
│   ├── provider.go                  # AIProvider interface + factory
│   └── prompts.go                   # Prompt templates
│
└── data/
    ├── io.github.MedaiP90.GiTK.desktop      # Desktop entry
    ├── io.github.MedaiP90.GiTK.metainfo.xml # AppStream metadata
    ├── io.github.MedaiP90.GiTK.svg          # Application icon
    └── io.github.MedaiP90.GiTK.json         # Flatpak manifest
```

---

## Architecture Notes

### Design Principles

- **No global state** - Dependencies are passed via constructors.
- **Thread safety** - The `git.Repository` uses `sync.RWMutex`. UI updates from goroutines go through `glib.IdleAdd()`.
- **Clean separation** - The `git/` package has zero GTK imports. It is a pure backend library that can be tested independently.
- **GNOME HIG compliance** - Uses `AdwApplicationWindow`, `AdwHeaderBar`, `AdwNavigationSplitView`, `AdwToastOverlay`, `AdwPreferencesWindow`, `AdwAlertDialog`, and other libadwaita widgets.

### Key gotk4 API Patterns

```go
// SignalListItemFactory callbacks require Object → ListItem cast:
factory.ConnectSetup(func(obj *coreglib.Object) {
    item := obj.Cast().(*gtk.ListItem)
    // ...
})

// ColumnViewColumn needs *gtk.ListItemFactory, not *SignalListItemFactory:
col := gtk.NewColumnViewColumn("Title", &factory.ListItemFactory)

// FileDialog.SelectFolder takes context.Context + *gtk.Window:
dialog.SelectFolder(context.Background(), &w.window.Window, callback)

// UI updates from goroutines must use glib.IdleAdd:
go func() {
    result, err := expensiveOperation()
    glib.IdleAdd(func() {
        updateUI(result, err)
    })
}()
```

### Repository State Machine

The `git.Repository` tracks its state as one of:
`Clean` | `Dirty` | `Merging` | `Rebasing` | `CherryPicking` | `Detached`

The UI adapts based on the current state (e.g., showing the merge editor when in `Merging` state).

---

## AI Commit Messages (Optional)

GiTK can generate commit messages using AI. Three providers are supported: **Claude (Anthropic)**, **Google Gemini**, and **OpenCode Go**.

### Setup

1. Open **Preferences** (`Ctrl+,`) and go to the **AI** tab
2. Toggle **Enable AI Features** on
3. Select a **Provider** and enter your API key for that provider
4. Optionally pick a different model

The API key is stored in `~/.config/gitk/config.json` (user-owned, restricted permissions). Alternatively, the key can be supplied via environment variable — GiTK will fall back to the environment variable if the config key is empty.

### Claude (Anthropic)

- API key: enter in Preferences, or set `ANTHROPIC_API_KEY` in the environment
- Models: `claude-sonnet-4-20250514` (default), `claude-opus-4-20250514`, `claude-haiku-4-5-20251001`
- Get a key at [console.anthropic.com](https://console.anthropic.com)

### Google Gemini

- API key: enter in Preferences, or set `GEMINI_API_KEY` in the environment
- Models: `gemini-2.5-flash` (default), `gemini-2.5-pro`, `gemini-2.0-flash`
- Get a key at [aistudio.google.com](https://aistudio.google.com)

### OpenCode Go

- API key: enter in Preferences, or set `OPENCODE_API_KEY` in the environment
- Models: `opencode-go/kimi-k2.5` (default), `opencode-go/glm-5`, `opencode-go/minimax-m2.5`
- Requires an [OpenCode Go subscription](https://opencode.ai/docs/go/) at opencode.ai
- Uses an OpenAI-compatible endpoint at `https://opencode.ai/zen/go/v1/chat/completions`

### How it works

Clicking the ✨ button in the staging area sends your staged diff to the selected AI provider and inserts the generated [conventional commit](https://www.conventionalcommits.org/) message (type, scope, subject + optional body) into the commit message editor.

---

## Flatpak

A Flatpak manifest is provided at `data/io.github.MedaiP90.GiTK.json`.

To build locally (requires `flatpak-builder`):
```bash
flatpak-builder --force-clean build/flatpak data/io.github.MedaiP90.GiTK.json
```

The app ID `io.github.MedaiP90.GiTK` follows the Flathub verification format.

---

## Makefile Reference

| Target          | Description                                    |
|-----------------|------------------------------------------------|
| `make build`    | Compile the binary to `build/gitk`             |
| `make run`      | Build and run                                  |
| `make test`     | Run all unit tests                             |
| `make vet`      | Run `go vet`                                   |
| `make lint`     | Run `go vet` + `staticcheck`                   |
| `make install`  | Install to `~/.local` (binary, desktop, icon)  |
| `make uninstall`| Remove installed files                         |
| `make clean`    | Remove build artifacts                         |
| `make help`     | Show all targets                               |

---

## Roadmap

### v0.1.0 (Current)
- [x] Application skeleton with GTK4/libadwaita
- [x] Git backend with go-git (repo, operations, diff, merge, graph)
- [x] Sidebar with recent repos and branch tree
- [x] Commit log with graph column and detail panel
- [x] Staging area with hunk-level staging
- [x] Visual DAG graph view
- [x] Three-way merge editor
- [x] Remote operations (push, pull)
- [x] Tag creation with auto-push to remote
- [x] Inline expandable diffs in commit detail
- [x] AI commit message button in staging area
- [x] Per-hunk staging/unstaging
- [x] Discard individual file changes
- [x] Resizable sidebar with drag handle
- [x] Staging button badge with change count
- [x] Deleted repo detection in recents
- [x] Preferences window
- [x] Flatpak packaging files

### v0.2.0 (Current)
- [x] Stash operations (full UI + git CLI backend)
- [x] Stash count chip in toolbar
- [x] Submodule management (add, update, remove in sidebar)
- [x] Branch creation from commit detail action menu
- [x] Branch deletion and merge from sidebar
- [x] Tag deletion from sidebar
- [x] Remove items from recent repositories list
- [x] Reset-to-commit (soft/mixed/hard) from commit detail
- [x] Add remotes from sidebar
- [x] Commit graph rendering fixed (hash-based O(1) lookup)
- [x] OpenCode Go as second AI provider
- [x] Google Gemini as third AI provider
- [x] `AdwNavigationSplitView` sidebar layout (GNOME HIG compliant)
- [x] Commit table resize fixed (synchronous with detail panel on window resize/divider drag)
- [x] Column reordering removed from commit log table
- [x] Cairo graph column rendering in `GtkColumnView` (lane lines, bezier curves, node shapes)
- [x] Cherry-pick dialog (from commit detail action menu)
- [x] File history view (per-file commit log)
- [x] Blame/annotate view (per-line commit info)
- [x] Interactive rebase UI (reorder, squash, fixup, drop)
- [x] Drag-and-drop branch reordering in sidebar

### v0.3.0 (Planned)
- [ ] Built-in image diff viewer
- [ ] Commit signing (GPG/SSH)
- [ ] Multi-repository workspace
- [ ] Search across all branches
- [ ] Custom themes / graph color schemes
- [ ] Localization (i18n via gettext)

### Future
- [ ] macOS and Windows support via GTK4 cross-compilation
- [ ] Plugin system for custom actions
- [ ] GitHub/GitLab integration (PR creation, issue linking)
- [ ] Performance optimizations for repositories with 100k+ commits
- [ ] Accessibility audit and screen reader support

---

## Contributing

Contributions are welcome! The codebase is heavily commented to be approachable for Go beginners.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes and ensure `make test` and `make vet` pass
4. Commit with a conventional commit message
5. Push and open a pull request

### Code Style
- All Go source files include package-level and function-level doc comments
- The `git/` package must never import GTK packages
- UI updates from goroutines must use `glib.IdleAdd()`
- Prefer `AdwDialog` over `GtkDialog` (deprecated in GTK4)
- Follow [GNOME HIG](https://developer.gnome.org/hig/) for new UI components

---

## License

GPL-3.0-or-later. See [LICENSE](LICENSE) for details.
