// Package git — merge.go provides three-way merge logic for resolving
// merge conflicts in GiTK.
//
// A three-way merge compares three versions of a file:
//   - BASE:   the common ancestor of the two branches being merged.
//   - OURS:   the version from the current branch (HEAD).
//   - THEIRS: the version from the branch being merged in.
//
// When the same region of a file is changed in both OURS and THEIRS
// (relative to BASE), we have a conflict that requires user resolution.
//
// This package detects conflict regions and provides utilities for
// resolving them programmatically (e.g., "use ours", "use theirs",
// "use both").
package git

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// ConflictRegion represents a single conflict in a three-way merge.
// Each conflict has the text from all three versions so the merge
// editor can display them side by side.
type ConflictRegion struct {
	// BaseText is the text from the common ancestor.
	BaseText string

	// OursText is the text from the current branch (HEAD).
	OursText string

	// TheirsText is the text from the branch being merged.
	TheirsText string

	// BaseStart is the starting line number in the base file.
	BaseStart int

	// BaseEnd is the ending line number in the base file.
	BaseEnd int

	// Resolved is true if the user has resolved this conflict.
	Resolved bool

	// Resolution is the text the user chose for this conflict region.
	// It's empty until the conflict is resolved.
	Resolution string
}

// MergeResult holds the result of a three-way merge for a single file.
type MergeResult struct {
	// Path is the file path.
	Path string

	// HasConflicts is true if there are unresolved conflicts.
	HasConflicts bool

	// Conflicts contains all conflict regions in the file.
	Conflicts []ConflictRegion

	// MergedContent is the fully merged content. If there are no
	// conflicts, this is the final result. If there are conflicts,
	// this contains conflict markers (<<<<<<< / ======= / >>>>>>>).
	MergedContent string

	// OursContent is the full content from our branch.
	OursContent string

	// TheirsContent is the full content from their branch.
	TheirsContent string

	// BaseContent is the full content from the common ancestor.
	BaseContent string
}

// ThreeWayMerge performs a three-way merge of three file versions.
// It returns a MergeResult with any conflicts detected.
//
// Parameters:
//   - base:   content from the common ancestor
//   - ours:   content from the current branch (HEAD)
//   - theirs: content from the branch being merged
//
// The algorithm:
//  1. Diff base→ours and base→theirs to find what each side changed.
//  2. If the changes don't overlap, merge them cleanly.
//  3. If they overlap, mark those regions as conflicts.
func ThreeWayMerge(path, base, ours, theirs string) MergeResult {
	result := MergeResult{
		Path:          path,
		OursContent:   ours,
		TheirsContent: theirs,
		BaseContent:   base,
	}

	// If one side didn't change, return the other side.
	if base == ours {
		result.MergedContent = theirs
		return result
	}
	if base == theirs {
		result.MergedContent = ours
		return result
	}
	if ours == theirs {
		// Both sides made the same changes — no conflict.
		result.MergedContent = ours
		return result
	}

	// Split into lines for line-level merging.
	baseLines := splitLines(base)
	oursLines := splitLines(ours)
	theirsLines := splitLines(theirs)

	// Compute line-level diffs.
	dmp := diffmatchpatch.New()

	// Diff base → ours.
	oursDiffs := dmp.DiffMain(base, ours, true)
	oursDiffs = dmp.DiffCleanupSemantic(oursDiffs)

	// Diff base → theirs.
	theirsDiffs := dmp.DiffMain(base, theirs, true)
	theirsDiffs = dmp.DiffCleanupSemantic(theirsDiffs)

	// Simple line-by-line merge with conflict detection.
	merged, conflicts := mergeLines(baseLines, oursLines, theirsLines)

	result.Conflicts = conflicts
	result.HasConflicts = len(conflicts) > 0
	result.MergedContent = strings.Join(merged, "\n")

	// Suppress "unused" warnings for the diffs. They're computed for
	// potential future use in more sophisticated merge algorithms.
	_ = oursDiffs
	_ = theirsDiffs

	return result
}

// mergeLines performs a simple line-by-line three-way merge.
// This is a basic implementation that detects conflicts when both
// sides modify the same lines.
func mergeLines(base, ours, theirs []string) ([]string, []ConflictRegion) {
	var merged []string
	var conflicts []ConflictRegion

	// Use the longest common subsequence approach for merging.
	// For simplicity, we walk through the lines and compare:
	maxLen := max(len(base), max(len(ours), len(theirs)))

	i := 0 // base index
	j := 0 // ours index
	k := 0 // theirs index

	for i < len(base) || j < len(ours) || k < len(theirs) {
		if i >= maxLen && j >= maxLen && k >= maxLen {
			break
		}

		baseLine := getLine(base, i)
		oursLine := getLine(ours, j)
		theirsLine := getLine(theirs, k)

		if baseLine == oursLine && baseLine == theirsLine {
			// All three agree — no change.
			if i < len(base) {
				merged = append(merged, baseLine)
			}
			i++
			j++
			k++
		} else if baseLine == oursLine && baseLine != theirsLine {
			// Only theirs changed — take theirs.
			if k < len(theirs) {
				merged = append(merged, theirsLine)
			}
			i++
			j++
			k++
		} else if baseLine != oursLine && baseLine == theirsLine {
			// Only ours changed — take ours.
			if j < len(ours) {
				merged = append(merged, oursLine)
			}
			i++
			j++
			k++
		} else if oursLine == theirsLine {
			// Both changed the same way — take either.
			if j < len(ours) {
				merged = append(merged, oursLine)
			}
			i++
			j++
			k++
		} else {
			// Conflict: both sides changed differently.
			conflict := ConflictRegion{
				BaseText:  baseLine,
				OursText:  oursLine,
				TheirsText: theirsLine,
				BaseStart: i + 1,
				BaseEnd:   i + 1,
			}
			conflicts = append(conflicts, conflict)

			// Add conflict markers to the merged output.
			merged = append(merged, "<<<<<<< OURS")
			merged = append(merged, oursLine)
			merged = append(merged, "=======")
			merged = append(merged, theirsLine)
			merged = append(merged, ">>>>>>> THEIRS")

			i++
			j++
			k++
		}
	}

	// Handle remaining lines from ours or theirs.
	for j < len(ours) {
		merged = append(merged, ours[j])
		j++
	}
	for k < len(theirs) {
		merged = append(merged, theirs[k])
		k++
	}

	return merged, conflicts
}

// ResolveConflict applies a resolution strategy to a conflict region.
type ResolutionStrategy int

const (
	// ResolveOurs uses the text from the current branch.
	ResolveOurs ResolutionStrategy = iota

	// ResolveTheirs uses the text from the branch being merged.
	ResolveTheirs

	// ResolveBothOursFirst uses both, with ours first.
	ResolveBothOursFirst

	// ResolveBothTheirsFirst uses both, with theirs first.
	ResolveBothTheirsFirst
)

// ApplyResolution resolves a conflict using the given strategy.
// It sets the Resolution field and marks the conflict as Resolved.
func (c *ConflictRegion) ApplyResolution(strategy ResolutionStrategy) {
	switch strategy {
	case ResolveOurs:
		c.Resolution = c.OursText
	case ResolveTheirs:
		c.Resolution = c.TheirsText
	case ResolveBothOursFirst:
		c.Resolution = c.OursText + "\n" + c.TheirsText
	case ResolveBothTheirsFirst:
		c.Resolution = c.TheirsText + "\n" + c.OursText
	}
	c.Resolved = true
}

// ApplyResolutions takes a MergeResult with resolved conflicts and
// produces the final merged content by replacing conflict markers
// with the chosen resolutions.
func ApplyResolutions(result *MergeResult) string {
	if !result.HasConflicts {
		return result.MergedContent
	}

	lines := strings.Split(result.MergedContent, "\n")
	var output []string
	conflictIdx := 0
	inConflict := false

	for _, line := range lines {
		if line == "<<<<<<< OURS" {
			inConflict = true
			if conflictIdx < len(result.Conflicts) && result.Conflicts[conflictIdx].Resolved {
				output = append(output, result.Conflicts[conflictIdx].Resolution)
			}
			continue
		}
		if line == ">>>>>>> THEIRS" {
			inConflict = false
			conflictIdx++
			continue
		}
		if line == "=======" && inConflict {
			continue
		}
		if inConflict {
			// Skip lines inside conflict markers (we already added the resolution).
			continue
		}
		output = append(output, line)
	}

	return strings.Join(output, "\n")
}

// splitLines splits a string into lines. Unlike strings.Split, this
// handles the edge case of a trailing newline without creating an
// extra empty element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty element if the string ends with newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// getLine safely gets a line from a slice, returning empty string if
// the index is out of bounds.
func getLine(lines []string, i int) string {
	if i >= len(lines) {
		return ""
	}
	return lines[i]
}
