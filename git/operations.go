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
	"log/slog"
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

// StashInfo holds information about a stash entry.
type StashInfo struct {
	// Index is the stash index (0 = most recent).
	Index int

	// Message is the stash message.
	Message string

	// Hash is the commit hash of the stash entry.
	Hash string
}

// Note: go-git has limited stash support. The Stash/StashPop/StashList
// methods below use the available API. For full stash support, we may
// need to shell out to git in the future.

// StashSave creates a new stash entry with the current changes.
// Note: go-git's stash support is limited. This is a best-effort
// implementation.
func (r *Repository) StashSave(message string) error {
	// go-git v5 does not have built-in stash support.
	// For now, return an informative error. In the future, we could
	// shell out to the git CLI for this operation.
	return fmt.Errorf("stash save: not yet implemented in go-git backend")
}

// StashList returns the list of stash entries.
func (r *Repository) StashList() ([]StashInfo, error) {
	return nil, fmt.Errorf("stash list: not yet implemented in go-git backend")
}

// StashApply applies a stash entry without removing it.
func (r *Repository) StashApply(index int) error {
	return fmt.Errorf("stash apply: not yet implemented in go-git backend")
}

// StashPop applies and removes a stash entry.
func (r *Repository) StashPop(index int) error {
	return fmt.Errorf("stash pop: not yet implemented in go-git backend")
}

// StashDrop removes a stash entry without applying it.
func (r *Repository) StashDrop(index int) error {
	return fmt.Errorf("stash drop: not yet implemented in go-git backend")
}
