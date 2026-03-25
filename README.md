# GiTK

A comprehensive, native GTK4 Git GUI client for GNOME, written in Go.

GiTK uses [gotk4](https://github.com/diamondburned/gotk4) for GTK4/libadwaita bindings and [go-git](https://github.com/go-git/go-git) for a pure-Go Git backend. It follows the [GNOME Human Interface Guidelines](https://developer.gnome.org/hig/) and is designed primarily for Linux/GNOME, with cross-platform potential.

**App ID:** `io.github.MedaiP90.GiTK` (Flathub-verifiable GitHub URI format)

---

## Features

### Repository Management
- Open local repositories via native file chooser (xdg-desktop-portal)
- Clone remote repositories with progress tracking
- Recent repositories list with quick access from the sidebar
- XDG-compliant JSON configuration (`~/.config/gitk/config.json`)

### Commit History
- Commit log table using `GtkColumnView` with columns: Graph, Hash, Subject, Author, Date, Refs
- Inline graph column rendering branch/merge topology with colored lanes
- Commit detail panel showing full metadata, message, and changed file list with +/- stats
- Unified diff viewer with syntax-colored added/deleted/context lines
- Search filtering by message, author, or hash

### Staging Area
- Split view: staged files, unstaged files, and hunk-level diff
- Stage/unstage individual files or all at once
- Per-hunk staging with colored diff display
- Commit message editor with subject line character counter (orange at 50, red at 72)
- Async commit creation with toast notifications

### Visual DAG Graph View
- Full commit graph drawn on a Cairo `GtkDrawingArea` inside a scrollable viewport
- Bezier curves for lane changes, diamonds for merge commits, double-ring for HEAD
- 8-color GNOME palette for lane coloring
- Zoom in/out (Ctrl+scroll or buttons) with clamped range (20-48px row height)
- Click-to-select commits with highlight ring
- Ref pills (branches, tags, remotes) with hit-testing

### Branch Management
- Sidebar branch tree with local, remote, and tag sections using `AdwExpanderRow`
- Click-to-checkout branches
- Current branch indicator

### Remote Operations
- Push dialog with remote selection and force-push toggle (with destructive action confirmation via `AdwAlertDialog`)
- Pull dialog with remote and branch selection
- Fetch all remotes from the menu

### Tag & Stash
- Create tag dialog (lightweight or annotated) with target commit field
- Stash dialog (UI ready; stash operations pending go-git backend support)

### Three-Way Merge Editor
- Three-pane layout: OURS (read-only) | RESULT (editable) | THEIRS (read-only)
- Conflict navigation (Previous/Next)
- Resolution strategies: Use Ours, Use Theirs, Both (Ours First), Both (Theirs First)
- Conflict counter with "Mark as Resolved" button
- Abort Merge with destructive action confirmation

### Preferences
- `AdwPreferencesWindow` with pages for Identity, Graph, and AI settings
- Git author name/email with per-repo override toggle
- Graph settings: max commits, show tags, show remotes
- Settings persist to JSON config on window close

### AI Commit Message Generation (Optional)
- Claude API integration via the Anthropic Messages API
- Reads API key from `ANTHROPIC_API_KEY` environment variable
- Configurable model selection (default: `claude-sonnet-4-20250514`)
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
3. Browse the commit log, click commits to see details and diffs
4. Switch to **Staging** (pencil icon) to see working tree changes
5. Switch to **Graph** (grid icon) for the visual DAG view
6. Use **Push/Pull** buttons in the header bar for remote operations
7. Open the hamburger menu for Tag, Stash, Fetch, Preferences, and About

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
│   │   ├── detail.go                #   Commit metadata + file list
│   │   └── diffview.go              #   Unified diff viewer (GtkTextView)
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
│   │   ├── remote.go                #   Push/Pull dialogs
│   │   ├── tag.go                   #   Create tag dialog
│   │   └── stash.go                 #   Stash dialog (UI only)
│   │
│   └── prefs/
│       └── prefs.go                 #   AdwPreferencesWindow
│
├── ai/
│   ├── claude.go                    # Claude API client
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

GiTK can generate commit messages using the Claude API. To enable:

1. Set the `ANTHROPIC_API_KEY` environment variable:
   ```bash
   export ANTHROPIC_API_KEY="sk-ant-..."
   ```
2. Open **Preferences** > **AI** and enable "AI Features"
3. Optionally change the model (default: `claude-sonnet-4-20250514`)

The AI reads your staged diff and generates a conventional commit message (type, scope, description). The API key is never stored in the config file.

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
- [x] Remote operations (push, pull, fetch)
- [x] Tag and stash dialogs
- [x] Preferences window
- [x] AI commit message generation
- [x] Flatpak packaging files

### v0.2.0 (Planned)
- [ ] Interactive rebase UI
- [ ] Cherry-pick dialog
- [ ] Blame/annotate view
- [ ] File history view
- [ ] Submodule management
- [ ] Stash operations (pending go-git support or git CLI fallback)
- [ ] Drag-and-drop branch reordering in sidebar

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
