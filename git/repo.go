// Package git provides a high-level abstraction over the go-git library
// for all Git operations that GiTK needs.
//
// IMPORTANT: This package has ZERO GTK imports. It is purely a backend
// library that can be tested independently. All communication with the
// UI happens through return values, error returns, and channels — never
// by calling GTK functions directly.
//
// The central type is Repository, which wraps a go-git repository and
// provides methods for all Git operations. The repository tracks its
// current state (Clean, Dirty, Merging, etc.) so the UI can adapt.
package git

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// RepoState represents the current state of a Git repository.
// The UI uses this to show appropriate banners, enable/disable actions,
// and adapt the layout.
type RepoState int

const (
	// StateClean means the working tree matches HEAD with no staged changes.
	StateClean RepoState = iota

	// StateDirty means there are uncommitted changes (staged or unstaged).
	StateDirty

	// StateMerging means a merge is in progress (MERGE_HEAD exists).
	StateMerging

	// StateRebasing means a rebase is in progress (.git/rebase-merge or
	// .git/rebase-apply exists).
	StateRebasing

	// StateCherryPicking means a cherry-pick is in progress
	// (CHERRY_PICK_HEAD exists).
	StateCherryPicking

	// StateDetached means HEAD is not pointing at a branch.
	StateDetached
)

// String returns a human-readable name for the repository state.
func (s RepoState) String() string {
	switch s {
	case StateClean:
		return "Clean"
	case StateDirty:
		return "Dirty"
	case StateMerging:
		return "Merging"
	case StateRebasing:
		return "Rebasing"
	case StateCherryPicking:
		return "Cherry-picking"
	case StateDetached:
		return "Detached HEAD"
	default:
		return "Unknown"
	}
}

// Repository is the central type that wraps a go-git repository and
// provides all Git operations needed by the GiTK UI.
//
// All methods are safe to call from goroutines. The repository uses a
// read-write mutex to prevent concurrent modifications while allowing
// concurrent reads.
type Repository struct {
	// mu protects concurrent access to the repository.
	mu sync.RWMutex

	// repo is the underlying go-git repository.
	repo *gogit.Repository

	// path is the absolute path to the repository root (the directory
	// containing the .git folder).
	path string

	// gitDir is the path to the .git directory itself.
	gitDir string
}

// OpenRepository opens an existing Git repository at the given path.
// The path can point to either the repository root or the .git directory.
//
// Returns an error if the path is not a valid Git repository.
//
// Example:
//
//	repo, err := git.OpenRepository("/home/user/myproject")
//	if err != nil {
//	    log.Fatal("not a git repo:", err)
//	}
func OpenRepository(path string) (*Repository, error) {
	// go-git's PlainOpen handles both cases: if path is the repo root,
	// it looks for .git inside; if it's a bare repo, it uses path directly.
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open repository %q: %w", path, err)
	}

	// Resolve the absolute path for consistency.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	r := &Repository{
		repo:   repo,
		path:   absPath,
		gitDir: filepath.Join(absPath, ".git"),
	}

	slog.Info("repository opened", "path", absPath)
	return r, nil
}

// CloneRepository clones a remote repository to the given local path.
// The progress callback is called periodically with a status message
// so the UI can show clone progress.
//
// Parameters:
//   - url:       the remote URL to clone from (HTTPS or SSH).
//   - destPath:  the local directory to clone into.
//   - progress:  optional callback for progress updates. Pass nil to ignore.
//
// Example:
//
//	repo, err := git.CloneRepository("https://github.com/user/repo.git", "/tmp/repo", nil)
func CloneRepository(url, destPath string, progress func(string)) (*Repository, error) {
	slog.Info("cloning repository", "url", url, "dest", destPath)

	// Set up progress reporting if a callback was provided.
	var progressWriter *progressReporter
	if progress != nil {
		progressWriter = &progressReporter{callback: progress}
	}

	opts := &gogit.CloneOptions{
		URL: url,
	}
	if progressWriter != nil {
		opts.Progress = progressWriter
	}

	repo, err := gogit.PlainClone(destPath, false, opts)
	if err != nil {
		return nil, fmt.Errorf("clone %q: %w", url, err)
	}

	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	r := &Repository{
		repo:   repo,
		path:   absPath,
		gitDir: filepath.Join(absPath, ".git"),
	}

	slog.Info("repository cloned", "url", url, "path", absPath)
	return r, nil
}

// Path returns the absolute path to the repository root directory.
func (r *Repository) Path() string {
	return r.path
}

// Name returns the repository name (the last component of the path).
// For "/home/user/myproject", this returns "myproject".
func (r *Repository) Name() string {
	return filepath.Base(r.path)
}

// State determines the current state of the repository by checking
// for merge/rebase/cherry-pick markers and working tree changes.
func (r *Repository) State() RepoState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for in-progress operations by looking for marker files.
	// These files are created by Git when an operation is paused.

	// MERGE_HEAD exists during a merge conflict.
	if fileExists(filepath.Join(r.gitDir, "MERGE_HEAD")) {
		return StateMerging
	}

	// rebase-merge or rebase-apply directories exist during a rebase.
	if dirExists(filepath.Join(r.gitDir, "rebase-merge")) ||
		dirExists(filepath.Join(r.gitDir, "rebase-apply")) {
		return StateRebasing
	}

	// CHERRY_PICK_HEAD exists during a cherry-pick conflict.
	if fileExists(filepath.Join(r.gitDir, "CHERRY_PICK_HEAD")) {
		return StateCherryPicking
	}

	// Check if HEAD is detached (not pointing at a branch).
	head, err := r.repo.Head()
	if err != nil {
		slog.Warn("failed to read HEAD", "error", err)
		return StateDetached
	}
	if !head.Name().IsBranch() {
		return StateDetached
	}

	// Check for uncommitted changes.
	wt, err := r.repo.Worktree()
	if err != nil {
		slog.Warn("failed to get worktree", "error", err)
		return StateClean
	}
	status, err := wt.Status()
	if err != nil {
		slog.Warn("failed to get status", "error", err)
		return StateClean
	}
	if !status.IsClean() {
		return StateDirty
	}

	return StateClean
}

// Head returns the current HEAD reference. This tells you which branch
// is checked out (or the detached commit hash).
func (r *Repository) Head() (*plumbing.Reference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}
	return head, nil
}

// CurrentBranch returns the name of the currently checked-out branch.
// Returns an empty string if HEAD is detached.
func (r *Repository) CurrentBranch() string {
	head, err := r.Head()
	if err != nil {
		return ""
	}
	if !head.Name().IsBranch() {
		return ""
	}
	return head.Name().Short()
}

// CommitInfo holds the metadata for a single Git commit. This struct is
// used throughout the application to display commit information without
// needing direct access to the go-git commit object.
type CommitInfo struct {
	// Hash is the full SHA-1 hash of the commit.
	Hash string

	// ShortHash is the first 7 characters of the hash.
	ShortHash string

	// Subject is the first line of the commit message.
	Subject string

	// Body is the full commit message (including the subject line).
	Body string

	// Author is the name of the commit author.
	Author string

	// AuthorEmail is the email of the commit author.
	AuthorEmail string

	// AuthorTime is when the commit was authored.
	AuthorTime time.Time

	// Committer is the name of the committer (may differ from author).
	Committer string

	// CommitterEmail is the email of the committer.
	CommitterEmail string

	// CommitTime is when the commit was made.
	CommitTime time.Time

	// ParentHashes is the list of parent commit hashes.
	// A regular commit has 1 parent; a merge commit has 2+; the root has 0.
	ParentHashes []string

	// IsMerge is true if this commit has more than one parent.
	IsMerge bool
}

// commitToInfo converts a go-git commit object to our CommitInfo struct.
// This is a helper used internally to bridge between go-git types and
// our application types.
func commitToInfo(c *object.Commit) CommitInfo {
	hash := c.Hash.String()
	parentHashes := make([]string, len(c.ParentHashes))
	for i, ph := range c.ParentHashes {
		parentHashes[i] = ph.String()
	}

	// The subject is the first line of the commit message.
	subject := c.Message
	for i, ch := range c.Message {
		if ch == '\n' {
			subject = c.Message[:i]
			break
		}
	}

	return CommitInfo{
		Hash:           hash,
		ShortHash:      hash[:7],
		Subject:        subject,
		Body:           c.Message,
		Author:         c.Author.Name,
		AuthorEmail:    c.Author.Email,
		AuthorTime:     c.Author.When,
		Committer:      c.Committer.Name,
		CommitterEmail: c.Committer.Email,
		CommitTime:     c.Committer.When,
		ParentHashes:   parentHashes,
		IsMerge:        len(c.ParentHashes) > 1,
	}
}

// Log returns a list of commits starting from HEAD (or the given options).
// The maxCount parameter limits how many commits to return (0 = all).
//
// This is the primary method for populating the commit log view.
func (r *Repository) Log(maxCount int) ([]CommitInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	opts := &gogit.LogOptions{
		Order: gogit.LogOrderCommitterTime,
	}

	iter, err := r.repo.Log(opts)
	if err != nil {
		return nil, fmt.Errorf("get log: %w", err)
	}
	defer iter.Close()

	var commits []CommitInfo
	count := 0

	err = iter.ForEach(func(c *object.Commit) error {
		if maxCount > 0 && count >= maxCount {
			// We've collected enough commits. Return a sentinel error
			// to stop iteration. go-git uses this pattern.
			return fmt.Errorf("limit reached")
		}
		commits = append(commits, commitToInfo(c))
		count++
		return nil
	})

	// The "limit reached" error is expected — ignore it.
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("iterate log: %w", err)
	}

	return commits, nil
}

// LogAll returns commits from all branches (not just HEAD).
// This is used by the graph view to show the full repository history.
func (r *Repository) LogAll(maxCount int) ([]CommitInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	opts := &gogit.LogOptions{
		Order: gogit.LogOrderCommitterTime,
		All:   true,
	}

	iter, err := r.repo.Log(opts)
	if err != nil {
		return nil, fmt.Errorf("get log (all): %w", err)
	}
	defer iter.Close()

	var commits []CommitInfo
	count := 0

	err = iter.ForEach(func(c *object.Commit) error {
		if maxCount > 0 && count >= maxCount {
			return fmt.Errorf("limit reached")
		}
		commits = append(commits, commitToInfo(c))
		count++
		return nil
	})

	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("iterate log (all): %w", err)
	}

	return commits, nil
}

// GoGitRepo returns the underlying go-git repository for operations
// that need direct access. Use sparingly — prefer the typed methods above.
func (r *Repository) GoGitRepo() *gogit.Repository {
	return r.repo
}

// progressReporter adapts a callback function to io.Writer so we can
// pass it to go-git's Progress field.
type progressReporter struct {
	callback func(string)
}

// Write implements io.Writer. It converts the byte slice to a string
// and calls the progress callback.
func (p *progressReporter) Write(data []byte) (int, error) {
	p.callback(string(data))
	return len(data), nil
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists checks if a directory exists at the given path.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
