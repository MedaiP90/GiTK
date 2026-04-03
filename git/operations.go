// Package git — operations.go provides all Git operations that modify
// the repository: commit, push, pull, fetch, branch management, tag
// management, stash, checkout, merge, rebase, cherry-pick, and reset.
//
// Each operation:
//  1. Acquires the repository write lock.
//  2. Performs the operation using go-git.
//  3. Returns a result or error.
//
// The UI calls these methods from goroutines and receives results via
// channels or callbacks. All errors are wrapped with context using
// fmt.Errorf("operation: %w", err) for clear error messages.
package git

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// FileStatusCode represents the status of a file in the working tree
// or staging area.
type FileStatusCode byte

const (
	// StatusUnmodified means the file has not changed.
	StatusUnmodified FileStatusCode = ' '

	// StatusModified means the file has been modified.
	StatusModified FileStatusCode = 'M'

	// StatusAdded means the file is new (tracked for the first time).
	StatusAdded FileStatusCode = 'A'

	// StatusDeleted means the file has been deleted.
	StatusDeleted FileStatusCode = 'D'

	// StatusRenamed means the file has been renamed.
	StatusRenamed FileStatusCode = 'R'

	// StatusCopied means the file has been copied.
	StatusCopied FileStatusCode = 'C'

	// StatusUntracked means the file is not tracked by Git.
	StatusUntracked FileStatusCode = '?'
)

// FileChange represents a single file's status in the working tree.
// It shows both the staging status and the working tree status.
type FileChange struct {
	// Path is the file path relative to the repository root.
	Path string

	// Staging is the file's status in the staging area (index).
	Staging FileStatusCode

	// Worktree is the file's status in the working tree.
	Worktree FileStatusCode
}

// Status returns the current working tree status — all modified, added,
// deleted, and untracked files.
func (r *Repository) Status() ([]FileChange, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}

	var changes []FileChange
	for path, fs := range status {
		changes = append(changes, FileChange{
			Path:     path,
			Staging:  FileStatusCode(fs.Staging),
			Worktree: FileStatusCode(fs.Worktree),
		})
	}

	return changes, nil
}

// StageFile adds a file to the staging area (git add).
// The path is relative to the repository root.
func (r *Repository) StageFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("stage file: get worktree: %w", err)
	}

	_, err = wt.Add(path)
	if err != nil {
		return fmt.Errorf("stage file %q: %w", path, err)
	}

	slog.Debug("staged file", "path", path)
	return nil
}

// UnstageFile removes a file from the staging area (git reset HEAD -- file).
// The file remains modified in the working tree.
func (r *Repository) UnstageFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("unstage file: get worktree: %w", err)
	}

	// go-git doesn't have a direct "unstage" — we reset the file in the
	// index to match HEAD.
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("unstage file: get HEAD: %w", err)
	}

	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("unstage file: get HEAD commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("unstage file: get tree: %w", err)
	}

	// Check if the file exists in HEAD. If not, we need to remove it
	// from the index entirely (it was a new file).
	_, err = tree.File(path)
	if err != nil {
		// File doesn't exist in HEAD — remove from index.
		_, err = wt.Remove(path)
		if err != nil {
			return fmt.Errorf("unstage new file %q: %w", path, err)
		}
	} else {
		// File exists in HEAD — reset it in the index.
		err = wt.Reset(&gogit.ResetOptions{
			Mode: gogit.MixedReset,
		})
		if err != nil {
			return fmt.Errorf("unstage file %q: %w", path, err)
		}
	}

	slog.Debug("unstaged file", "path", path)
	return nil
}

// DiscardFile discards unstaged changes to a file by restoring the
// version from the index (equivalent to git checkout -- file).
func (r *Repository) DiscardFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullPath := filepath.Join(r.path, path)

	// Try to read the file from the index first.
	content, err := r.readIndexFileUnlocked(path)
	if err != nil {
		// Not in index, try HEAD.
		head, herr := r.repo.Head()
		if herr != nil {
			return fmt.Errorf("discard file: no index or HEAD: %w", err)
		}
		commit, cerr := r.repo.CommitObject(head.Hash())
		if cerr != nil {
			return fmt.Errorf("discard file: get HEAD commit: %w", cerr)
		}
		tree, terr := commit.Tree()
		if terr != nil {
			return fmt.Errorf("discard file: get tree: %w", terr)
		}
		file, ferr := tree.File(path)
		if ferr != nil {
			// File doesn't exist in HEAD — it's untracked. Delete it.
			if removeErr := os.Remove(fullPath); removeErr != nil {
				return fmt.Errorf("discard untracked file %q: %w", path, removeErr)
			}
			slog.Debug("discarded untracked file", "path", path)
			return nil
		}
		content, err = file.Contents()
		if err != nil {
			return fmt.Errorf("discard file: read HEAD content: %w", err)
		}
	}

	// Write the index/HEAD content to the working tree.
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("discard file %q: write: %w", path, err)
	}

	slog.Debug("discarded file", "path", path)
	return nil
}

// StageAll stages all changes in the working tree (git add -A).
func (r *Repository) StageAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("stage all: get worktree: %w", err)
	}

	// Add all files to the index.
	err = wt.AddWithOptions(&gogit.AddOptions{All: true})
	if err != nil {
		return fmt.Errorf("stage all: %w", err)
	}

	slog.Debug("staged all changes")
	return nil
}

// CommitOptions configures how a commit is created.
type CommitOptions struct {
	// Message is the commit message (required).
	Message string

	// AuthorName overrides the author name. If empty, uses git config.
	AuthorName string

	// AuthorEmail overrides the author email. If empty, uses git config.
	AuthorEmail string

	// Amend, when true, amends the last commit instead of creating a new one.
	Amend bool

	// SignOff, when true, adds a "Signed-off-by" line to the message.
	SignOff bool
}

// Commit creates a new commit with the staged changes.
// Returns the hash of the new commit.
func (r *Repository) Commit(opts CommitOptions) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if opts.Message == "" {
		return "", fmt.Errorf("commit: message is required")
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("commit: get worktree: %w", err)
	}

	// Build the commit options for go-git.
	now := time.Now()
	commitOpts := &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  opts.AuthorName,
			Email: opts.AuthorEmail,
			When:  now,
		},
	}

	// If author info is not provided, go-git will try to read it from
	// the git config file.
	if opts.AuthorName == "" || opts.AuthorEmail == "" {
		commitOpts.Author = nil
	}

	if opts.Amend {
		commitOpts.Amend = true
	}

	hash, err := wt.Commit(opts.Message, commitOpts)
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	slog.Info("committed", "hash", hash.String()[:7], "message", opts.Message)
	return hash.String(), nil
}

// BranchInfo holds information about a Git branch.
type BranchInfo struct {
	// Name is the short branch name (e.g., "main", "feature/login").
	Name string

	// FullName is the full reference name (e.g., "refs/heads/main").
	FullName string

	// Hash is the commit hash that the branch points to.
	Hash string

	// IsRemote is true if this is a remote-tracking branch.
	IsRemote bool

	// IsCurrent is true if this is the currently checked-out branch.
	IsCurrent bool

	// Upstream is the upstream tracking branch name (e.g., "origin/main").
	Upstream string
}

// Branches returns a list of all local and remote branches.
func (r *Repository) Branches() ([]BranchInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	currentBranch := r.currentBranchUnlocked()

	var branches []BranchInfo

	// Local branches.
	localIter, err := r.repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer localIter.Close()

	err = localIter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		branches = append(branches, BranchInfo{
			Name:      name,
			FullName:  ref.Name().String(),
			Hash:      ref.Hash().String(),
			IsRemote:  false,
			IsCurrent: name == currentBranch,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate local branches: %w", err)
	}

	// Remote branches.
	remoteIter, err := r.repo.References()
	if err != nil {
		return nil, fmt.Errorf("list references: %w", err)
	}
	defer remoteIter.Close()

	err = remoteIter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsRemote() {
			branches = append(branches, BranchInfo{
				Name:     ref.Name().Short(),
				FullName: ref.Name().String(),
				Hash:     ref.Hash().String(),
				IsRemote: true,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate remote refs: %w", err)
	}

	return branches, nil
}

// currentBranchUnlocked returns the current branch name without locking.
// The caller must hold at least a read lock.
func (r *Repository) currentBranchUnlocked() string {
	head, err := r.repo.Head()
	if err != nil {
		return ""
	}
	if !head.Name().IsBranch() {
		return ""
	}
	return head.Name().Short()
}

// CreateBranch creates a new local branch pointing at the given commit.
// If commitHash is empty, the branch is created from HEAD.
func (r *Repository) CreateBranch(name string, commitHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Resolve the target commit.
	var targetHash plumbing.Hash
	if commitHash == "" {
		head, err := r.repo.Head()
		if err != nil {
			return fmt.Errorf("create branch: get HEAD: %w", err)
		}
		targetHash = head.Hash()
	} else {
		targetHash = plumbing.NewHash(commitHash)
	}

	// Create the branch reference.
	refName := plumbing.NewBranchReferenceName(name)
	ref := plumbing.NewHashReference(refName, targetHash)
	if err := r.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("create branch %q: %w", name, err)
	}

	slog.Info("branch created", "name", name, "target", targetHash.String()[:7])
	return nil
}

// DeleteBranch deletes a local branch. Returns an error if the branch
// is currently checked out.
func (r *Repository) DeleteBranch(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Safety check: don't delete the current branch.
	current := r.currentBranchUnlocked()
	if name == current {
		return fmt.Errorf("delete branch: cannot delete the current branch %q", name)
	}

	refName := plumbing.NewBranchReferenceName(name)
	if err := r.repo.Storer.RemoveReference(refName); err != nil {
		return fmt.Errorf("delete branch %q: %w", name, err)
	}

	slog.Info("branch deleted", "name", name)
	return nil
}

// Checkout switches to the given branch.
func (r *Repository) Checkout(branchName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("checkout: get worktree: %w", err)
	}

	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		return fmt.Errorf("checkout %q: %w", branchName, err)
	}

	slog.Info("checked out", "branch", branchName)
	return nil
}

// CheckoutTrack creates a local tracking branch from a remote branch and checks it out.
// remoteBranch should be in the form "origin/feature-x".
func (r *Repository) CheckoutTrack(remoteBranch string) error {
	cmd := exec.Command("git", "checkout", "--track", remoteBranch)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("checkout --track %q: %s", remoteBranch, strings.TrimSpace(string(out)))
	}
	slog.Info("checked out tracking branch", "remote", remoteBranch)
	return nil
}

// CheckoutCommit checks out a specific commit (detached HEAD).
func (r *Repository) CheckoutCommit(hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("checkout commit: get worktree: %w", err)
	}

	err = wt.Checkout(&gogit.CheckoutOptions{
		Hash: plumbing.NewHash(hash),
	})
	if err != nil {
		return fmt.Errorf("checkout commit %s: %w", hash[:7], err)
	}

	slog.Info("checked out commit (detached)", "hash", hash[:7])
	return nil
}

// TagInfo holds information about a Git tag.
type TagInfo struct {
	// Name is the tag name (e.g., "v1.0.0").
	Name string

	// Hash is the commit hash the tag points to.
	Hash string

	// Message is the tag message (empty for lightweight tags).
	Message string

	// IsAnnotated is true if this is an annotated tag (has a message).
	IsAnnotated bool

	// Tagger is the name of the person who created the tag.
	Tagger string

	// TagTime is when the tag was created.
	TagTime time.Time
}

// Tags returns a list of all tags in the repository.
func (r *Repository) Tags() ([]TagInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tags []TagInfo

	iter, err := r.repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer iter.Close()

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		tag := TagInfo{
			Name: ref.Name().Short(),
			Hash: ref.Hash().String(),
		}

		// Try to get annotated tag info.
		tagObj, err := r.repo.TagObject(ref.Hash())
		if err == nil {
			// This is an annotated tag.
			tag.IsAnnotated = true
			tag.Message = tagObj.Message
			tag.Tagger = tagObj.Tagger.Name
			tag.TagTime = tagObj.Tagger.When
			tag.Hash = tagObj.Target.String()
		}

		tags = append(tags, tag)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return tags, nil
}

// CreateTag creates a new tag pointing at the given commit.
// If commitHash is empty, the tag is created at HEAD.
// If message is non-empty, an annotated tag is created.
func (r *Repository) CreateTag(name, commitHash, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Resolve target.
	var targetHash plumbing.Hash
	if commitHash == "" {
		head, err := r.repo.Head()
		if err != nil {
			return fmt.Errorf("create tag: get HEAD: %w", err)
		}
		targetHash = head.Hash()
	} else {
		targetHash = plumbing.NewHash(commitHash)
	}

	if message != "" {
		// Annotated tag.
		_, err := r.repo.CreateTag(name, targetHash, &gogit.CreateTagOptions{
			Message: message,
			Tagger: &object.Signature{
				Name:  "GiTK User",
				Email: "user@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			return fmt.Errorf("create annotated tag %q: %w", name, err)
		}
	} else {
		// Lightweight tag.
		refName := plumbing.NewTagReferenceName(name)
		ref := plumbing.NewHashReference(refName, targetHash)
		if err := r.repo.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("create lightweight tag %q: %w", name, err)
		}
	}

	slog.Info("tag created", "name", name, "target", targetHash.String()[:7])
	return nil
}

// PushTag pushes a single tag to the specified remote.
func (r *Repository) PushTag(name, remoteName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if remoteName == "" {
		remoteName = "origin"
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", name, name))
	err := r.repo.Push(&gogit.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refSpec},
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("push tag %q to %s: %w", name, remoteName, err)
	}

	slog.Info("tag pushed", "name", name, "remote", remoteName)
	return nil
}

// DeleteTag deletes a local tag.
func (r *Repository) DeleteTag(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.repo.DeleteTag(name); err != nil {
		return fmt.Errorf("delete tag %q: %w", name, err)
	}

	slog.Info("tag deleted", "name", name)
	return nil
}

// DeleteRemoteBranch deletes a branch from the given remote using git push --delete.
func (r *Repository) DeleteRemoteBranch(remote, name string) error {
	cmd := exec.Command("git", "push", remote, "--delete", name)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete remote branch: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("remote branch deleted", "remote", remote, "name", name)
	return nil
}

// DeleteRemoteTag deletes a tag from the given remote using git push --delete.
func (r *Repository) DeleteRemoteTag(remote, name string) error {
	cmd := exec.Command("git", "push", remote, "--delete", "refs/tags/"+name)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete remote tag: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("remote tag deleted", "remote", remote, "name", name)
	return nil
}

// RemoteInfo holds information about a Git remote.
type RemoteInfo struct {
	// Name is the remote name (e.g., "origin").
	Name string

	// URLs are the remote URLs (fetch and push).
	URLs []string
}

// Remotes returns a list of all configured remotes.
func (r *Repository) Remotes() ([]RemoteInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	remotes, err := r.repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("list remotes: %w", err)
	}

	var result []RemoteInfo
	for _, remote := range remotes {
		cfg := remote.Config()
		result = append(result, RemoteInfo{
			Name: cfg.Name,
			URLs: cfg.URLs,
		})
	}

	return result, nil
}

// Fetch fetches from all remotes.
func (r *Repository) Fetch() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	err := r.repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch: %w", err)
	}

	slog.Info("fetched from origin")
	return nil
}

// Pull pulls from the specified remote and branch.
func (r *Repository) Pull(remoteName, branchName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("pull: get worktree: %w", err)
	}

	opts := &gogit.PullOptions{
		RemoteName:    remoteName,
		ReferenceName: plumbing.NewBranchReferenceName(branchName),
	}

	err = wt.Pull(opts)
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("pull %s/%s: %w", remoteName, branchName, err)
	}

	slog.Info("pulled", "remote", remoteName, "branch", branchName)
	return nil
}

// Push pushes the current branch to the specified remote.
func (r *Repository) Push(remoteName string, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	opts := &gogit.PushOptions{
		RemoteName: remoteName,
		Force:      force,
	}

	err := r.repo.Push(opts)
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("push to %s: %w", remoteName, err)
	}

	slog.Info("pushed", "remote", remoteName, "force", force)
	return nil
}

// ResetMode defines how a git reset behaves.
type ResetMode int

const (
	// ResetSoft moves HEAD but keeps changes staged.
	ResetSoft ResetMode = iota

	// ResetMixed moves HEAD and unstages changes (default git reset behavior).
	ResetMixed

	// ResetHard moves HEAD and discards all changes. DESTRUCTIVE!
	ResetHard
)

// Reset resets the current branch to the given commit.
// WARNING: ResetHard is destructive — it discards uncommitted changes.
func (r *Repository) Reset(commitHash string, mode ResetMode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("reset: get worktree: %w", err)
	}

	var goGitMode gogit.ResetMode
	switch mode {
	case ResetSoft:
		goGitMode = gogit.SoftReset
	case ResetMixed:
		goGitMode = gogit.MixedReset
	case ResetHard:
		goGitMode = gogit.HardReset
	}

	err = wt.Reset(&gogit.ResetOptions{
		Commit: plumbing.NewHash(commitHash),
		Mode:   goGitMode,
	})
	if err != nil {
		return fmt.Errorf("reset to %s: %w", commitHash[:7], err)
	}

	slog.Info("reset", "hash", commitHash[:7], "mode", mode)
	return nil
}

// MergeBranch merges the given branch into the current branch.
// It shells out to the git CLI because go-git's merge support is limited.
func (r *Repository) MergeBranch(branchName string) error {
	cmd := exec.Command("git", "merge", "--no-edit", branchName)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("merge %s: %s", branchName, strings.TrimSpace(string(out)))
	}
	slog.Info("merged branch", "branch", branchName)
	return nil
}

// AddRemote adds a new remote to the repository.
func (r *Repository) AddRemote(name, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("add remote %q: %w", name, err)
	}

	slog.Info("remote added", "name", name, "url", url)
	return nil
}

// RemoveRemote removes a remote from the repository.
func (r *Repository) RemoveRemote(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.repo.DeleteRemote(name); err != nil {
		return fmt.Errorf("remove remote %q: %w", name, err)
	}

	slog.Info("remote removed", "name", name)
	return nil
}

// StageHunk stages a single hunk of a file by temporarily swapping the
// worktree file content, staging it, and restoring the original content.
func (r *Repository) StageHunk(path string, hunk Hunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("stage hunk: get worktree: %w", err)
	}

	fullPath := filepath.Join(r.path, path)

	// Save current worktree content.
	worktreeContent, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("stage hunk: read worktree file: %w", err)
	}

	// Read current index content (what's already staged for this file).
	indexContent, err := r.readIndexFileUnlocked(path)
	if err != nil {
		// File not in index yet — start from empty.
		indexContent = ""
	}

	// Apply the hunk forward to the index content.
	newContent := ApplyHunk(indexContent, hunk, false)

	// Write new content to worktree temporarily.
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("stage hunk: write temp content: %w", err)
	}

	// Stage the file.
	_, addErr := wt.Add(path)

	// Restore worktree content regardless of staging result.
	if restoreErr := os.WriteFile(fullPath, worktreeContent, 0644); restoreErr != nil {
		slog.Warn("stage hunk: failed to restore worktree file", "path", path, "error", restoreErr)
	}

	if addErr != nil {
		return fmt.Errorf("stage hunk: add to index: %w", addErr)
	}

	slog.Debug("staged hunk", "path", path)
	return nil
}

// UnstageHunk unstages a single hunk of a file by reversing the hunk
// in the index content.
func (r *Repository) UnstageHunk(path string, hunk Hunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("unstage hunk: get worktree: %w", err)
	}

	fullPath := filepath.Join(r.path, path)

	// Save current worktree content.
	worktreeContent, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("unstage hunk: read worktree file: %w", err)
	}

	// Read current index content.
	indexContent, err := r.readIndexFileUnlocked(path)
	if err != nil {
		return fmt.Errorf("unstage hunk: read index file: %w", err)
	}

	// Apply the hunk in reverse to remove it from the index.
	newContent := ApplyHunk(indexContent, hunk, true)

	// Write new content to worktree temporarily.
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("unstage hunk: write temp content: %w", err)
	}

	// Stage the file (updates the index to match the new content).
	_, addErr := wt.Add(path)

	// Restore worktree content.
	if restoreErr := os.WriteFile(fullPath, worktreeContent, 0644); restoreErr != nil {
		slog.Warn("unstage hunk: failed to restore worktree file", "path", path, "error", restoreErr)
	}

	if addErr != nil {
		return fmt.Errorf("unstage hunk: add to index: %w", addErr)
	}

	slog.Debug("unstaged hunk", "path", path)
	return nil
}

// DiscardHunk discards a single unstaged hunk by reversing it in the
// worktree file content.
func (r *Repository) DiscardHunk(path string, hunk Hunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullPath := filepath.Join(r.path, path)

	worktreeContent, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("discard hunk: read worktree file: %w", err)
	}

	// Reverse the hunk in the worktree content.
	newContent := ApplyHunk(string(worktreeContent), hunk, true)

	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("discard hunk: write file: %w", err)
	}

	slog.Debug("discarded hunk", "path", path)
	return nil
}

// readIndexFileUnlocked reads a file's content from the git index.
// The caller must hold at least a read lock.
func (r *Repository) readIndexFileUnlocked(path string) (string, error) {
	idx, err := r.repo.Storer.Index()
	if err != nil {
		return "", fmt.Errorf("read index: %w", err)
	}

	for _, entry := range idx.Entries {
		if entry.Name == path {
			blob, err := r.repo.BlobObject(entry.Hash)
			if err != nil {
				return "", fmt.Errorf("read blob %s: %w", entry.Hash, err)
			}
			reader, err := blob.Reader()
			if err != nil {
				return "", fmt.Errorf("open blob reader: %w", err)
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				return "", fmt.Errorf("read blob data: %w", err)
			}
			return string(data), nil
		}
	}

	return "", fmt.Errorf("file %q not found in index", path)
}

// ApplyHunk applies or reverses a single hunk on the given content.
// When reverse is false, it applies the hunk (adds additions, removes deletions).
// When reverse is true, it reverses the hunk (removes additions, restores deletions).
func ApplyHunk(content string, hunk Hunk, reverse bool) string {
	lines := strings.Split(content, "\n")

	// Handle trailing newline.
	hasTrailingNewline := len(content) > 0 && content[len(content)-1] == '\n'
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Determine start position in the current content (0-indexed).
	var startLine int
	if reverse {
		startLine = hunk.NewStart - 1
	} else {
		startLine = hunk.OldStart - 1
	}
	if startLine < 0 {
		startLine = 0
	}

	var result []string

	// Copy lines before the hunk.
	for i := 0; i < startLine && i < len(lines); i++ {
		result = append(result, lines[i])
	}

	// Apply hunk lines.
	idx := startLine
	for _, dl := range hunk.Lines {
		if reverse {
			switch dl.Type {
			case DiffLineContext:
				if idx < len(lines) {
					result = append(result, lines[idx])
					idx++
				}
			case DiffLineAdd:
				// In reverse: added lines are in the current content — skip.
				idx++
			case DiffLineDelete:
				// In reverse: deleted lines need to be restored.
				result = append(result, dl.Content)
			}
		} else {
			switch dl.Type {
			case DiffLineContext:
				if idx < len(lines) {
					result = append(result, lines[idx])
					idx++
				}
			case DiffLineDelete:
				// Forward: skip deleted lines from old content.
				idx++
			case DiffLineAdd:
				// Forward: insert new lines.
				result = append(result, dl.Content)
			}
		}
	}

	// Copy remaining lines after the hunk.
	for i := idx; i < len(lines); i++ {
		result = append(result, lines[i])
	}

	resultStr := strings.Join(result, "\n")
	if hasTrailingNewline || len(resultStr) > 0 {
		resultStr += "\n"
	}

	return resultStr
}

// StashInfo holds information about a stash entry.
type StashInfo struct {
	// Index is the stash index (0 = most recent).
	Index int

	// Message is the stash message.
	Message string

	// Hash is the commit hash of the stash entry.
	Hash string
}

// StashSave creates a new stash entry with the current changes.
// It shells out to the git CLI because go-git v5 has no built-in stash support.
func (r *Repository) StashSave(message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stash save: %s", strings.TrimSpace(string(out)))
	}
	output := strings.TrimSpace(string(out))
	if output == "No local changes to save" {
		return fmt.Errorf("stash save: no local changes to save")
	}
	slog.Info("stash saved", "message", message)
	return nil
}

// StashList returns the list of stash entries.
// It parses the output of "git stash list --format=%H %s".
func (r *Repository) StashList() ([]StashInfo, error) {
	cmd := exec.Command("git", "stash", "list", "--format=%H\t%s")
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("stash list: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var stashes []StashInfo
	for i, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		hash := ""
		msg := line
		if len(parts) == 2 {
			hash = parts[0]
			msg = parts[1]
		}
		stashes = append(stashes, StashInfo{Index: i, Message: msg, Hash: hash})
	}
	return stashes, nil
}

// StashApply applies a stash entry without removing it from the stash list.
func (r *Repository) StashApply(index int) error {
	ref := "stash@{" + strconv.Itoa(index) + "}"
	cmd := exec.Command("git", "stash", "apply", ref)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stash apply: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("stash applied", "index", index)
	return nil
}

// StashPop applies and removes a stash entry.
func (r *Repository) StashPop(index int) error {
	ref := "stash@{" + strconv.Itoa(index) + "}"
	cmd := exec.Command("git", "stash", "pop", ref)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stash pop: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("stash popped", "index", index)
	return nil
}

// StashDrop removes a stash entry without applying it.
func (r *Repository) StashDrop(index int) error {
	ref := "stash@{" + strconv.Itoa(index) + "}"
	cmd := exec.Command("git", "stash", "drop", ref)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stash drop: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("stash dropped", "index", index)
	return nil
}

// StashShow returns the list of changed files and their structured diffs for a stash entry.
func (r *Repository) StashShow(index int) ([]DiffResult, error) {
	ref := "stash@{" + strconv.Itoa(index) + "}"
	// Use only -p to get the raw patch; parse file paths from diff --git headers.
	cmd := exec.Command("git", "stash", "show", "-p", ref)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("stash show: %s", strings.TrimSpace(string(out)))
	}

	raw := string(out)

	// Split the raw patch at "diff --git " boundaries to get per-file sections.
	sections := strings.Split(raw, "diff --git ")
	// sections[0] is any output before the first diff (summary stats) — skip it.

	var results []DiffResult
	for _, section := range sections[1:] {
		if strings.TrimSpace(section) == "" {
			continue
		}
		// Extract the file path from the first line: "a/<path> b/<path>"
		firstNewline := strings.IndexByte(section, '\n')
		header := section
		if firstNewline >= 0 {
			header = section[:firstNewline]
		}
		// header is like "a/path/to/file b/path/to/file"
		path := extractDiffPath(header)

		dr := ParseRawDiff("diff --git " + section)
		dr.OldPath = path
		dr.NewPath = path
		if dr.ChangeType == 0 {
			dr.ChangeType = ChangeModified
		}
		results = append(results, dr)
	}

	return results, nil
}

// extractDiffPath extracts the new file path from a "diff --git" first-line
// header of the form "a/<path> b/<path>".
func extractDiffPath(header string) string {
	// The format is "a/<path> b/<path>".
	// Find the last " b/" which marks the start of the new path.
	idx := strings.LastIndex(header, " b/")
	if idx >= 0 {
		return header[idx+3:] // strip " b/"
	}
	// Fallback: try stripping the "b/" prefix from the right half.
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 {
		p := parts[1]
		if strings.HasPrefix(p, "b/") {
			return p[2:]
		}
		return p
	}
	return header
}

// StashClear removes all stash entries from the repository.
func (r *Repository) StashClear() error {
	cmd := exec.Command("git", "stash", "clear")
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stash clear: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("stash cleared")
	return nil
}

// SubmoduleInfo holds information about a git submodule.
type SubmoduleInfo struct {
	// Name is the submodule name (usually the path).
	Name string

	// Path is the submodule path relative to the repository root.
	Path string

	// URL is the remote URL of the submodule.
	URL string

	// Hash is the currently checked-out commit hash in the submodule.
	Hash string
}

// Submodules returns information about all submodules in the repository.
func (r *Repository) Submodules() ([]SubmoduleInfo, error) {
	cmd := exec.Command("git", "submodule", "status")
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		// Not an error if there are simply no submodules.
		return nil, nil
	}

	// Parse the URL for each submodule from .gitmodules via config.
	urlCmd := exec.Command("git", "config", "--file", ".gitmodules", "--get-regexp", "submodule\\..*\\.url")
	urlCmd.Dir = r.path
	urlOut, _ := urlCmd.Output()
	urlMap := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(urlOut)), "\n") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			// key: "submodule.<name>.url"
			key := parts[0]
			url := parts[1]
			// Extract <name> from key.
			keyParts := strings.Split(key, ".")
			if len(keyParts) >= 3 {
				name := strings.Join(keyParts[1:len(keyParts)-1], ".")
				urlMap[name] = url
			}
		}
	}

	var submodules []SubmoduleInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: [ +-U]<hash> <path> [(<describe>)]
		line = strings.TrimLeft(line, " +-U")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		path := parts[1]
		name := path
		url := urlMap[name]
		submodules = append(submodules, SubmoduleInfo{
			Name: name,
			Path: path,
			URL:  url,
			Hash: hash,
		})
	}
	return submodules, nil
}

// AddSubmodule adds a new submodule to the repository.
func (r *Repository) AddSubmodule(url, path string) error {
	cmd := exec.Command("git", "submodule", "add", url, path)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add submodule: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("submodule added", "url", url, "path", path)
	return nil
}

// UpdateSubmodules runs `git submodule update --init --recursive` to
// initialise and update all submodules to the committed state.
func (r *Repository) UpdateSubmodules() error {
	cmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update submodules: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("submodules updated")
	return nil
}

// RemoveSubmodule removes a submodule from the repository by deinitialising,
// removing the worktree directory and unregistering from .gitmodules / config.
func (r *Repository) RemoveSubmodule(path string) error {
	// 1. Deinit.
	cmd := exec.Command("git", "submodule", "deinit", "-f", path)
	cmd.Dir = r.path
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deinit submodule: %s", strings.TrimSpace(string(out)))
	}
	// 2. Remove from index and working tree.
	cmd = exec.Command("git", "rm", "-f", path)
	cmd.Dir = r.path
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rm submodule: %s", strings.TrimSpace(string(out)))
	}
	// 3. Remove leftover .git/modules/<path> directory.
	modulesDir := filepath.Join(r.path, ".git", "modules", path)
	_ = os.RemoveAll(modulesDir)
	slog.Info("submodule removed", "path", path)
	return nil
}

// CherryPick applies the changes introduced by a commit onto the current branch.
// It shells out to git CLI because go-git's cherry-pick support is limited.
func (r *Repository) CherryPick(hash string) error {
	cmd := exec.Command("git", "cherry-pick", hash)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cherry-pick %s: %s", hash[:7], strings.TrimSpace(string(out)))
	}
	slog.Info("cherry-picked", "hash", hash[:7])
	return nil
}

// BlameLine holds annotation data for a single line in a file.
type BlameLine struct {
	// LineNo is the 1-based line number in the file.
	LineNo int

	// Hash is the commit hash that last modified this line.
	Hash string

	// ShortHash is the first 7 characters of Hash.
	ShortHash string

	// Author is the name of the commit author.
	Author string

	// Date is the author date of the commit.
	Date time.Time

	// Message is the commit subject line.
	Message string

	// Text is the actual line content.
	Text string
}

// BlameFile returns blame annotation for every line in the file at path
// as it existed at commitHash. An empty commitHash uses HEAD.
func (r *Repository) BlameFile(path, commitHash string) ([]BlameLine, error) {
	args := []string{"blame", "--porcelain"}
	if commitHash != "" {
		args = append(args, commitHash)
	}
	args = append(args, "--", path)

	cmd := exec.Command("git", args...)
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("blame %s: %w", path, err)
	}

	return parseBlameOutput(string(out)), nil
}

// parseBlameOutput parses the output of `git blame --porcelain`.
func parseBlameOutput(output string) []BlameLine {
	lines := strings.Split(output, "\n")
	// Porcelain format: each hunk starts with "<40-hex-hash> <orig> <final> [<lines>]"
	// followed by key-value header lines, then a line starting with TAB (the actual content).
	commitInfo := make(map[string]struct {
		Author  string
		Date    time.Time
		Message string
	})

	var result []BlameLine
	lineNo := 0

	for i := 0; i < len(lines); {
		line := lines[i]
		if len(line) < 40 {
			i++
			continue
		}
		// Check if this is a hash line (40 hex chars followed by space).
		hash := line[:40]
		allHex := true
		for _, c := range hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				allHex = false
				break
			}
		}
		if !allHex || len(line) < 41 || line[40] != ' ' {
			i++
			continue
		}

		lineNo++
		i++

		// Read header key-value pairs until the TAB-prefixed content line.
		ci := commitInfo[hash]
		for i < len(lines) && len(lines[i]) > 0 && lines[i][0] != '\t' {
			kv := lines[i]
			i++
			if strings.HasPrefix(kv, "author ") {
				ci.Author = strings.TrimPrefix(kv, "author ")
			} else if strings.HasPrefix(kv, "author-time ") {
				ts, err := strconv.ParseInt(strings.TrimPrefix(kv, "author-time "), 10, 64)
				if err == nil {
					ci.Date = time.Unix(ts, 0)
				}
			} else if strings.HasPrefix(kv, "summary ") {
				ci.Message = strings.TrimPrefix(kv, "summary ")
			}
		}
		commitInfo[hash] = ci

		// Content line (tab-prefixed).
		text := ""
		if i < len(lines) && len(lines[i]) > 0 && lines[i][0] == '\t' {
			text = lines[i][1:]
			i++
		}

		shortHash := hash
		if len(hash) >= 7 {
			shortHash = hash[:7]
		}

		result = append(result, BlameLine{
			LineNo:    lineNo,
			Hash:      hash,
			ShortHash: shortHash,
			Author:    commitInfo[hash].Author,
			Date:      commitInfo[hash].Date,
			Message:   commitInfo[hash].Message,
			Text:      text,
		})
	}

	return result
}

// FileLog returns the list of commits that touched the given file path,
// following renames (using git log --follow). Limit 0 means no limit.
func (r *Repository) FileLog(path string, limit int) ([]CommitInfo, error) {
	args := []string{"log", "--format=%H\t%s\t%an\t%ae\t%ai", "--follow"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	args = append(args, "--", path)

	cmd := exec.Command("git", args...)
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("file log %s: %w", path, err)
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			continue
		}
		hash := parts[0]
		subject := parts[1]
		author := parts[2]
		authorEmail := parts[3]
		dateStr := parts[4]

		shortHash := hash
		if len(hash) >= 7 {
			shortHash = hash[:7]
		}

		t, _ := time.Parse("2006-01-02 15:04:05 -0700", dateStr)

		commits = append(commits, CommitInfo{
			Hash:        hash,
			ShortHash:   shortHash,
			Subject:     subject,
			Body:        subject,
			Author:      author,
			AuthorEmail: authorEmail,
			AuthorTime:  t,
		})
	}
	return commits, nil
}

// RebaseTodoAction represents a rebase todo action.
type RebaseTodoAction string

const (
	RebasePick   RebaseTodoAction = "pick"
	RebaseReword RebaseTodoAction = "reword"
	RebaseEdit   RebaseTodoAction = "edit"
	RebaseSquash RebaseTodoAction = "squash"
	RebaseFixup  RebaseTodoAction = "fixup"
	RebaseDrop   RebaseTodoAction = "drop"
)

// RebaseTodo is a single entry in the rebase todo list.
type RebaseTodo struct {
	Action  RebaseTodoAction
	Hash    string
	Subject string
}

// ListRebaseCommits returns commits reachable from HEAD but not from base,
// in reverse order (oldest first), suitable for populating a rebase todo list.
func (r *Repository) ListRebaseCommits(base string) ([]CommitInfo, error) {
	cmd := exec.Command("git", "log", "--format=%H\t%s\t%an\t%ai", "--reverse", base+"..HEAD")
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list rebase commits: %w", err)
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		hash := parts[0]
		shortHash := hash
		if len(hash) >= 7 {
			shortHash = hash[:7]
		}
		t, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[3])
		commits = append(commits, CommitInfo{
			Hash:       hash,
			ShortHash:  shortHash,
			Subject:    parts[1],
			Body:       parts[1],
			Author:     parts[2],
			AuthorTime: t,
		})
	}
	return commits, nil
}

// InteractiveRebase starts an interactive rebase using the given todo list.
// It writes a custom GIT_SEQUENCE_EDITOR script to avoid opening a terminal editor.
func (r *Repository) InteractiveRebase(base string, todos []RebaseTodo) error {
	// Build the todo file content.
	var sb strings.Builder
	for _, t := range todos {
		sb.WriteString(string(t.Action))
		sb.WriteByte(' ')
		sb.WriteString(t.Hash)
		sb.WriteByte(' ')
		sb.WriteString(t.Subject)
		sb.WriteByte('\n')
	}

	// Write todo to temp file.
	todoFile, err := os.CreateTemp("", "gitk-rebase-todo-*")
	if err != nil {
		return fmt.Errorf("create todo temp file: %w", err)
	}
	defer os.Remove(todoFile.Name())
	if _, err := todoFile.WriteString(sb.String()); err != nil {
		todoFile.Close()
		return fmt.Errorf("write todo: %w", err)
	}
	todoFile.Close()

	// Write a tiny shell script that replaces git's sequence editor call.
	scriptFile, err := os.CreateTemp("", "gitk-seqeditor-*")
	if err != nil {
		return fmt.Errorf("create seq-editor temp file: %w", err)
	}
	defer os.Remove(scriptFile.Name())
	script := fmt.Sprintf("#!/bin/sh\ncp %q \"$1\"\n", todoFile.Name())
	if _, err := scriptFile.WriteString(script); err != nil {
		scriptFile.Close()
		return fmt.Errorf("write seq-editor script: %w", err)
	}
	scriptFile.Close()
	if err := os.Chmod(scriptFile.Name(), 0700); err != nil {
		return fmt.Errorf("chmod seq-editor: %w", err)
	}

	cmd := exec.Command("git", "rebase", "-i", base)
	cmd.Dir = r.path
	cmd.Env = append(os.Environ(), "GIT_SEQUENCE_EDITOR="+scriptFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rebase: %s", strings.TrimSpace(string(out)))
	}

	slog.Info("interactive rebase completed", "base", base, "todos", len(todos))
	return nil
}

// AbortRebase aborts an in-progress interactive rebase.
func (r *Repository) AbortRebase() error {
	cmd := exec.Command("git", "rebase", "--abort")
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rebase --abort: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("rebase aborted")
	return nil
}
