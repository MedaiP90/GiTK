// Package git — diff.go provides diff computation and hunk parsing.
//
// A "diff" shows the changes between two versions of a file. In Git,
// we typically diff between:
//   - HEAD and the staging area (staged changes)
//   - The staging area and the working tree (unstaged changes)
//   - Two commits (commit comparison)
//
// A diff is composed of "hunks" — contiguous blocks of changed lines.
// Each hunk has a header (e.g., "@@ -10,5 +10,7 @@") that tells you
// where in the file the changes are.
//
// This package parses diffs into structured Hunk and DiffLine types
// that the UI can render with proper syntax highlighting.
package git

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffResult represents the complete diff for a single file.
type DiffResult struct {
	// OldPath is the path of the file before the change (may differ from
	// NewPath if the file was renamed).
	OldPath string

	// NewPath is the path of the file after the change.
	NewPath string

	// ChangeType describes what happened to the file.
	ChangeType ChangeType

	// Hunks contains the individual changed sections of the file.
	Hunks []Hunk

	// OldContent is the complete old file content (before changes).
	OldContent string

	// NewContent is the complete new file content (after changes).
	NewContent string

	// IsBinary is true if the file is binary (not diffable as text).
	IsBinary bool

	// Stats holds line count statistics for this file diff.
	Stats DiffStats
}

// ChangeType describes what kind of change happened to a file.
type ChangeType int

const (
	// ChangeModified means the file content was changed.
	ChangeModified ChangeType = iota

	// ChangeAdded means the file is new.
	ChangeAdded

	// ChangeDeleted means the file was removed.
	ChangeDeleted

	// ChangeRenamed means the file was renamed (possibly with content changes).
	ChangeRenamed
)

// String returns a human-readable name for the change type.
func (ct ChangeType) String() string {
	switch ct {
	case ChangeModified:
		return "Modified"
	case ChangeAdded:
		return "Added"
	case ChangeDeleted:
		return "Deleted"
	case ChangeRenamed:
		return "Renamed"
	default:
		return "Unknown"
	}
}

// Hunk represents a contiguous block of changes in a diff.
// A hunk typically has a few lines of context before and after the
// actual changes.
type Hunk struct {
	// OldStart is the starting line number in the old file.
	OldStart int

	// OldLines is the number of lines from the old file in this hunk.
	OldLines int

	// NewStart is the starting line number in the new file.
	NewStart int

	// NewLines is the number of lines from the new file in this hunk.
	NewLines int

	// Header is the hunk header string (e.g., "@@ -10,5 +10,7 @@ func main()").
	Header string

	// Lines contains all the lines in this hunk (context + changes).
	Lines []DiffLine
}

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	// Type indicates whether this line is context, added, or removed.
	Type DiffLineType

	// Content is the line content (without the leading +/-/space).
	Content string

	// OldLineNo is the line number in the old file (0 if this is an added line).
	OldLineNo int

	// NewLineNo is the line number in the new file (0 if this is a removed line).
	NewLineNo int
}

// DiffLineType indicates the type of a diff line.
type DiffLineType int

const (
	// DiffLineContext is an unchanged line shown for context.
	DiffLineContext DiffLineType = iota

	// DiffLineAdd is a newly added line (shown with green background).
	DiffLineAdd

	// DiffLineDelete is a removed line (shown with red background).
	DiffLineDelete
)

// DiffStats holds line count statistics for a file diff.
type DiffStats struct {
	// Additions is the number of added lines.
	Additions int

	// Deletions is the number of removed lines.
	Deletions int
}

// DiffCommit computes the diff between a commit and its parent.
// For merge commits, it diffs against the first parent.
// For the root commit (no parents), it shows all files as added.
func (r *Repository) DiffCommit(commitHash string) ([]DiffResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the commit object.
	commit, err := r.repo.CommitObject(hashFromString(commitHash))
	if err != nil {
		return nil, fmt.Errorf("diff commit: get commit %s: %w", commitHash[:7], err)
	}

	// Get the commit's tree.
	commitTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("diff commit: get tree: %w", err)
	}

	// Get the parent's tree (or nil for root commit).
	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("diff commit: get parent: %w", err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("diff commit: get parent tree: %w", err)
		}
	}

	return diffTrees(parentTree, commitTree)
}

// DiffWorking computes the diff between the staging area and the
// working tree (unstaged changes).
func (r *Repository) DiffWorking() ([]DiffResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get working tree status to find changed files.
	changes, err := r.Status()
	if err != nil {
		return nil, fmt.Errorf("diff working: %w", err)
	}

	var results []DiffResult
	for _, change := range changes {
		if change.Worktree == StatusUnmodified {
			continue
		}
		// For each changed file, compute the diff.
		result, err := r.diffFile(change.Path, false)
		if err != nil {
			// Skip files we can't diff (e.g., binary files).
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// DiffStaged computes the diff between HEAD and the staging area
// (staged changes that will be committed).
func (r *Repository) DiffStaged() ([]DiffResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	changes, err := r.Status()
	if err != nil {
		return nil, fmt.Errorf("diff staged: %w", err)
	}

	var results []DiffResult
	for _, change := range changes {
		if change.Staging == StatusUnmodified && change.Staging != StatusAdded {
			continue
		}
		result, err := r.diffFile(change.Path, true)
		if err != nil {
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// diffFile computes the diff for a single file.
// If staged is true, diffs HEAD vs index. Otherwise, diffs index vs worktree.
func (r *Repository) diffFile(path string, staged bool) (DiffResult, error) {
	// Get the old content (from HEAD for staged, from index for unstaged).
	oldContent, err := r.getFileContent(path, staged)
	if err != nil {
		oldContent = "" // File might be new.
	}

	// Get the new content.
	newContent, err := r.getNewFileContent(path, staged)
	if err != nil {
		newContent = "" // File might be deleted.
	}

	// Determine change type.
	changeType := ChangeModified
	if oldContent == "" && newContent != "" {
		changeType = ChangeAdded
	} else if oldContent != "" && newContent == "" {
		changeType = ChangeDeleted
	}

	// Compute the diff using the diff-match-patch library.
	hunks, stats := computeDiff(oldContent, newContent)

	return DiffResult{
		OldPath:    path,
		NewPath:    path,
		ChangeType: changeType,
		Hunks:      hunks,
		OldContent: oldContent,
		NewContent: newContent,
		Stats:      stats,
	}, nil
}

// getFileContent gets the content of a file from HEAD (for staged diffs)
// or from the index (for working tree diffs).
func (r *Repository) getFileContent(path string, fromHead bool) (string, error) {
	if fromHead {
		// Get content from HEAD commit.
		head, err := r.repo.Head()
		if err != nil {
			return "", err
		}
		commit, err := r.repo.CommitObject(head.Hash())
		if err != nil {
			return "", err
		}
		tree, err := commit.Tree()
		if err != nil {
			return "", err
		}
		file, err := tree.File(path)
		if err != nil {
			return "", err
		}
		return file.Contents()
	}

	// For unstaged diffs, we'd ideally read from the index.
	// go-git makes this complex, so for now we read from HEAD as well.
	// This is a simplification that works for most cases.
	return r.getFileContent(path, true)
}

// getNewFileContent gets the new content of a file — from the index
// (for staged diffs) or the working tree (for unstaged diffs).
func (r *Repository) getNewFileContent(path string, staged bool) (string, error) {
	if staged {
		// For staged diffs, the "new" content is in the index.
		// go-git doesn't expose the index content easily, so we read
		// from the worktree as an approximation.
		return r.readWorktreeFile(path)
	}

	// For unstaged diffs, read from the working tree.
	return r.readWorktreeFile(path)
}

// readWorktreeFile reads a file directly from the working directory.
func (r *Repository) readWorktreeFile(path string) (string, error) {
	fullPath := r.path + "/" + path
	data, err := readFileContent(fullPath)
	if err != nil {
		return "", fmt.Errorf("read worktree file %q: %w", path, err)
	}
	return data, nil
}

// readFileContent reads a file's content as a string.
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// computeDiff computes a unified diff between two strings and returns
// structured hunks with line numbers.
func computeDiff(oldContent, newContent string) ([]Hunk, DiffStats) {
	if oldContent == newContent {
		return nil, DiffStats{}
	}

	// Use diff-match-patch for the actual diffing.
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldContent, newContent, true)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Convert dmp diffs to our hunk/line format.
	return diffsToHunks(diffs)
}

// diffsToHunks converts diff-match-patch diffs into our Hunk structure
// with proper line numbers and context lines.
func diffsToHunks(diffs []diffmatchpatch.Diff) ([]Hunk, DiffStats) {
	var stats DiffStats
	var allLines []DiffLine
	oldLine := 1
	newLine := 1

	// First pass: convert all diffs to DiffLines with line numbers.
	for _, d := range diffs {
		lines := strings.Split(d.Text, "\n")

		// The last element after splitting might be empty if the text
		// ends with a newline. Handle this carefully.
		for i, line := range lines {
			// Skip the empty string after the final newline.
			if i == len(lines)-1 && line == "" {
				continue
			}

			switch d.Type {
			case diffmatchpatch.DiffEqual:
				allLines = append(allLines, DiffLine{
					Type:      DiffLineContext,
					Content:   line,
					OldLineNo: oldLine,
					NewLineNo: newLine,
				})
				oldLine++
				newLine++

			case diffmatchpatch.DiffDelete:
				allLines = append(allLines, DiffLine{
					Type:      DiffLineDelete,
					Content:   line,
					OldLineNo: oldLine,
				})
				oldLine++
				stats.Deletions++

			case diffmatchpatch.DiffInsert:
				allLines = append(allLines, DiffLine{
					Type:      DiffLineAdd,
					Content:   line,
					NewLineNo: newLine,
				})
				newLine++
				stats.Additions++
			}
		}
	}

	// Second pass: group lines into hunks. A hunk starts when we see
	// a changed line and includes up to 3 lines of context before and after.
	const contextLines = 3
	hunks := groupIntoHunks(allLines, contextLines)

	return hunks, stats
}

// changeRange represents a range of indices in a diff line slice.
// Used by groupIntoHunks and mergeRanges to track which lines belong
// to a hunk.
type changeRange struct {
	start, end int // indices into lines slice
}

// groupIntoHunks groups diff lines into hunks with context.
func groupIntoHunks(lines []DiffLine, contextSize int) []Hunk {
	if len(lines) == 0 {
		return nil
	}

	// Find ranges of changed lines.

	var ranges []changeRange
	inChange := false
	var current changeRange

	for i, line := range lines {
		if line.Type != DiffLineContext {
			if !inChange {
				// Start of a new change range, with context before.
				start := i - contextSize
				if start < 0 {
					start = 0
				}
				current = changeRange{start: start}
				inChange = true
			}
			current.end = i
		} else if inChange {
			// Check if we're past the context after a change.
			if i-current.end > contextSize {
				// End the current range with context after.
				current.end = current.end + contextSize
				if current.end >= len(lines) {
					current.end = len(lines) - 1
				}
				ranges = append(ranges, current)
				inChange = false
			}
		}
	}

	// Close any open range.
	if inChange {
		current.end = current.end + contextSize
		if current.end >= len(lines) {
			current.end = len(lines) - 1
		}
		ranges = append(ranges, current)
	}

	// Merge overlapping ranges.
	merged := mergeRanges(ranges)

	// Build hunks from merged ranges.
	var hunks []Hunk
	for _, r := range merged {
		hunk := Hunk{
			Lines: lines[r.start : r.end+1],
		}

		// Set line numbers from the first line.
		if len(hunk.Lines) > 0 {
			first := hunk.Lines[0]
			if first.OldLineNo > 0 {
				hunk.OldStart = first.OldLineNo
			}
			if first.NewLineNo > 0 {
				hunk.NewStart = first.NewLineNo
			}
		}

		// Count old and new lines.
		for _, line := range hunk.Lines {
			if line.Type == DiffLineContext || line.Type == DiffLineDelete {
				hunk.OldLines++
			}
			if line.Type == DiffLineContext || line.Type == DiffLineAdd {
				hunk.NewLines++
			}
		}

		hunk.Header = fmt.Sprintf("@@ -%d,%d +%d,%d @@",
			hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)

		hunks = append(hunks, hunk)
	}

	return hunks
}

// mergeRanges merges overlapping or adjacent change ranges.
func mergeRanges(ranges []changeRange) []changeRange {
	if len(ranges) == 0 {
		return nil
	}

	merged := []changeRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		if ranges[i].start <= last.end+1 {
			// Ranges overlap or are adjacent — merge them.
			if ranges[i].end > last.end {
				last.end = ranges[i].end
			}
		} else {
			merged = append(merged, ranges[i])
		}
	}

	return merged
}

// diffTrees computes the diff between two git trees.
func diffTrees(oldTree, newTree *object.Tree) ([]DiffResult, error) {
	changes, err := object.DiffTree(oldTree, newTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}

	var results []DiffResult
	for _, change := range changes {
		result, err := changeToResult(change)
		if err != nil {
			continue // Skip files we can't process.
		}
		results = append(results, result)
	}

	return results, nil
}

// changeToResult converts a go-git tree change to our DiffResult.
func changeToResult(change *object.Change) (DiffResult, error) {
	// Get the old and new file names.
	action, err := change.Action()
	if err != nil {
		return DiffResult{}, err
	}

	result := DiffResult{}

	switch action {
	case merkletrie.Insert:
		result.ChangeType = ChangeAdded
		result.NewPath = change.To.Name
		result.OldPath = change.To.Name
	case merkletrie.Delete:
		result.ChangeType = ChangeDeleted
		result.OldPath = change.From.Name
		result.NewPath = change.From.Name
	case merkletrie.Modify:
		result.ChangeType = ChangeModified
		result.OldPath = change.From.Name
		result.NewPath = change.To.Name
	}

	// Get file contents for diffing.
	var oldContent, newContent string

	if change.From.Name != "" {
		fromFile, _, err := change.Files()
		if err == nil && fromFile != nil {
			oldContent, _ = fromFile.Contents()
		}
	}
	if change.To.Name != "" {
		_, toFile, err := change.Files()
		if err == nil && toFile != nil {
			newContent, _ = toFile.Contents()
		}
	}

	result.OldContent = oldContent
	result.NewContent = newContent

	// Compute hunks.
	hunks, stats := computeDiff(oldContent, newContent)
	result.Hunks = hunks
	result.Stats = stats

	return result, nil
}

// hashFromString converts a hex string to a plumbing.Hash.
func hashFromString(s string) plumbing.Hash {
	return plumbing.NewHash(s)
}
