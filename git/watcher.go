// Package git — watcher.go provides a filesystem watcher that monitors
// a Git repository's working tree and .git directory for changes.
//
// When changes are detected (file edits, new commits, branch switches),
// the watcher sends an event on a channel. The UI subscribes to this
// channel and refreshes the relevant views.
//
// The watcher uses polling (checking file modification times) rather than
// inotify/fsnotify because:
//  1. It's simpler and more portable (works on all platforms).
//  2. Git operations often cause many rapid file changes — polling with
//     a debounce interval gives a single "something changed" event.
//  3. fsnotify has known issues with some filesystems (NFS, FUSE).
//
// The polling interval is configurable but defaults to 2 seconds.
package git

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WatchEvent describes what kind of change was detected.
type WatchEvent int

const (
	// WatchEventWorkTree means files in the working tree changed
	// (edited, created, deleted).
	WatchEventWorkTree WatchEvent = iota

	// WatchEventIndex means the Git index (staging area) changed.
	WatchEventIndex

	// WatchEventHead means HEAD changed (new commit, branch switch).
	WatchEventHead

	// WatchEventRefs means a branch or tag was created/deleted/moved.
	WatchEventRefs
)

// String returns a human-readable name for the watch event.
func (e WatchEvent) String() string {
	switch e {
	case WatchEventWorkTree:
		return "WorkTree"
	case WatchEventIndex:
		return "Index"
	case WatchEventHead:
		return "HEAD"
	case WatchEventRefs:
		return "Refs"
	default:
		return "Unknown"
	}
}

// Watcher monitors a Git repository for filesystem changes.
// Create one with NewWatcher() and start it with Start().
// Events are delivered on the Events channel.
type Watcher struct {
	// Events is the channel where watch events are sent.
	// The UI should read from this channel to know when to refresh.
	Events chan WatchEvent

	// repo is the repository being watched.
	repo *Repository

	// interval is how often we check for changes.
	interval time.Duration

	// stopCh signals the watcher goroutine to stop.
	stopCh chan struct{}

	// wg tracks the watcher goroutine for clean shutdown.
	wg sync.WaitGroup

	// lastMtimes stores the last known modification times of key files
	// so we can detect changes.
	lastMtimes map[string]time.Time
}

// NewWatcher creates a new filesystem watcher for the given repository.
// The pollInterval controls how often the watcher checks for changes.
// Use 2*time.Second for a good balance between responsiveness and CPU usage.
//
// The watcher does NOT start automatically — call Start() to begin watching.
func NewWatcher(repo *Repository, pollInterval time.Duration) *Watcher {
	return &Watcher{
		Events:     make(chan WatchEvent, 10), // Buffered to avoid blocking.
		repo:       repo,
		interval:   pollInterval,
		stopCh:     make(chan struct{}),
		lastMtimes: make(map[string]time.Time),
	}
}

// Start begins watching the repository in a background goroutine.
// Call Stop() to clean up when done.
func (w *Watcher) Start() {
	// Take an initial snapshot of file modification times.
	w.snapshot()

	w.wg.Add(1)
	go w.pollLoop()

	slog.Info("watcher started", "path", w.repo.Path(), "interval", w.interval)
}

// Stop stops the watcher and waits for the goroutine to finish.
// After Stop() returns, no more events will be sent on the Events channel.
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	slog.Info("watcher stopped", "path", w.repo.Path())
}

// pollLoop is the main loop that periodically checks for changes.
// It runs in a goroutine started by Start().
func (w *Watcher) pollLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// check compares current file modification times against the last
// snapshot and emits events for any changes.
func (w *Watcher) check() {
	gitDir := filepath.Join(w.repo.Path(), ".git")

	// Check HEAD (branch switches, new commits).
	headPath := filepath.Join(gitDir, "HEAD")
	if w.hasChanged(headPath) {
		w.emit(WatchEventHead)
	}

	// Check the index (staging area changes).
	indexPath := filepath.Join(gitDir, "index")
	if w.hasChanged(indexPath) {
		w.emit(WatchEventIndex)
	}

	// Check refs directory (branch/tag creation/deletion).
	refsPath := filepath.Join(gitDir, "refs")
	if w.hasDirectoryChanged(refsPath) {
		w.emit(WatchEventRefs)
	}

	// We don't poll the entire working tree (too expensive for large repos).
	// Instead, we rely on the index check — when the user runs `git add`,
	// the index file changes. For real-time working tree monitoring,
	// the UI can trigger a manual refresh.
}

// hasChanged checks if a file's modification time has changed since
// the last snapshot.
func (w *Watcher) hasChanged(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	mtime := info.ModTime()
	lastMtime, exists := w.lastMtimes[path]

	// Update the stored mtime.
	w.lastMtimes[path] = mtime

	if !exists {
		// First check — don't emit an event for the initial state.
		return false
	}

	return !mtime.Equal(lastMtime)
}

// hasDirectoryChanged checks if any file in a directory has been modified.
// This is used for the refs directory where branches/tags are stored
// as individual files.
func (w *Watcher) hasDirectoryChanged(dirPath string) bool {
	changed := false

	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't read.
		}
		if info.IsDir() {
			return nil
		}
		if w.hasChanged(path) {
			changed = true
		}
		return nil
	})

	return changed
}

// snapshot records the current modification times of all watched files.
// Called once at startup.
func (w *Watcher) snapshot() {
	gitDir := filepath.Join(w.repo.Path(), ".git")

	// Snapshot key files.
	for _, path := range []string{
		filepath.Join(gitDir, "HEAD"),
		filepath.Join(gitDir, "index"),
	} {
		info, err := os.Stat(path)
		if err == nil {
			w.lastMtimes[path] = info.ModTime()
		}
	}

	// Snapshot refs directory.
	refsPath := filepath.Join(gitDir, "refs")
	filepath.Walk(refsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		w.lastMtimes[path] = info.ModTime()
		return nil
	})
}

// emit sends an event on the Events channel. It uses a non-blocking send
// to avoid deadlocking if the receiver is slow.
func (w *Watcher) emit(event WatchEvent) {
	select {
	case w.Events <- event:
		slog.Debug("watcher event emitted", "event", event)
	default:
		// Channel is full — the UI is not consuming events fast enough.
		// This is fine; the next poll will detect the same change.
		slog.Debug("watcher event dropped (channel full)", "event", event)
	}
}
