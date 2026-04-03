// Package git — graph.go provides the DAG (Directed Acyclic Graph) data
// model and the lane assignment algorithm used by both the commit log's
// graph column and the full visual graph view.
//
// A Git commit history forms a DAG where each commit points to its parent(s).
// To visualize this as a graph, we need to:
//  1. Walk commits in topological order (newest first).
//  2. Assign each commit to a "lane" (horizontal column).
//  3. Draw edges between commits and their parents.
//
// The lane assignment algorithm tries to minimize crossing lines and
// reuse freed lanes to keep the graph compact.
//
// Key types:
//   - GraphCommit: a commit enriched with layout data (lane, edges, refs).
//   - GraphEdge:   an edge to draw from this commit to a parent.
//   - GraphRef:    a branch/tag label attached to a commit.
//   - GraphOptions: controls which commits/refs to include.
//
// This code has ZERO GTK imports. The UI packages read the computed
// []GraphCommit and render it with Cairo.
package git

import (
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// MaxLanes is the maximum number of parallel lanes in the graph.
// Beyond this, additional lanes are compressed into a single overflow
// lane rendered with a dashed line.
const MaxLanes = 16

// GraphCommit is a commit enriched with graph layout data.
// It contains everything the UI needs to render one row of the graph.
type GraphCommit struct {
	// Hash is the full SHA-1 hash.
	Hash string

	// ShortHash is the first 7 characters.
	ShortHash string

	// Subject is the first line of the commit message.
	Subject string

	// Author is the commit author's name.
	Author string

	// AuthorEmail is the commit author's email.
	AuthorEmail string

	// Timestamp is when the commit was authored.
	Timestamp time.Time

	// Lane is the horizontal lane index assigned by the layout algorithm.
	// Lane 0 is the leftmost lane.
	Lane int

	// Edges are the lines to draw from this commit to its parents.
	Edges []GraphEdge

	// Refs are branch/tag/remote labels attached to this commit.
	Refs []GraphRef

	// IsMerge is true if this commit has more than one parent.
	IsMerge bool

	// IsHead is true if this commit is the current HEAD.
	IsHead bool

	// ParentHashes is the list of parent commit hashes.
	ParentHashes []string

	// ActiveLanes lists all lane indices that have a vertical line
	// passing through this row (including the commit's own lane).
	// The renderer uses this to draw pass-through lane lines.
	ActiveLanes []int
}

// GraphEdge describes a line to draw from this commit's lane to a
// parent commit's lane. The UI draws these as vertical/curved lines
// connecting commit nodes.
type GraphEdge struct {
	// FromLane is the lane of this commit (the child).
	FromLane int

	// ToLane is the lane of the parent commit.
	ToLane int

	// Style controls how the edge is rendered.
	Style EdgeStyle
}

// EdgeStyle controls the visual rendering of a graph edge.
type EdgeStyle int

const (
	// EdgeSolid is a normal solid line.
	EdgeSolid EdgeStyle = iota

	// EdgeDashed is used for overflow lanes (beyond MaxLanes).
	EdgeDashed
)

// GraphRef is a branch, tag, or remote ref label attached to a commit.
type GraphRef struct {
	// Name is the display name (e.g., "main", "v1.0", "origin/main").
	Name string

	// Kind indicates the type of reference.
	Kind RefKind
}

// RefKind categorizes a Git reference for display styling.
type RefKind int

const (
	// RefLocalBranch is a local branch (e.g., "main", "feature/login").
	RefLocalBranch RefKind = iota

	// RefRemoteBranch is a remote-tracking branch (e.g., "origin/main").
	RefRemoteBranch

	// RefTag is a tag (e.g., "v1.0.0").
	RefTag

	// RefHEAD indicates the current HEAD (shown with special styling).
	RefHEAD
)

// GraphOptions controls which commits and refs are included in the graph.
type GraphOptions struct {
	// AllBranches includes commits from all branches, not just HEAD.
	AllBranches bool

	// IncludeTags includes tag refs in the graph labels.
	IncludeTags bool

	// IncludeRemotes includes remote-tracking branches.
	IncludeRemotes bool

	// MaxCommits limits how many commits to include (0 = no limit).
	MaxCommits int
}

// DefaultGraphOptions returns sensible defaults for graph building.
func DefaultGraphOptions() GraphOptions {
	return GraphOptions{
		AllBranches:    true,
		IncludeTags:    true,
		IncludeRemotes: true,
		MaxCommits:     2000,
	}
}

// BuildGraph performs the full topological walk and lane assignment.
// It returns a slice of GraphCommits ready for rendering.
//
// This function is safe to call from a goroutine. It reads from the
// repository but does not modify it.
//
// The algorithm:
//  1. Load commits using Repository.LogAll().
//  2. Build a map of hash → refs (branches, tags, remotes).
//  3. Walk commits in order and assign lanes.
//  4. Compute edges between each commit and its parents.
func BuildGraph(repo *Repository, opts GraphOptions) ([]GraphCommit, error) {
	slog.Debug("building graph", "allBranches", opts.AllBranches, "maxCommits", opts.MaxCommits)

	// Step 1: Load commits.
	maxCommits := opts.MaxCommits
	if maxCommits == 0 {
		maxCommits = 2000
	}

	var commits []CommitInfo
	var err error

	if opts.AllBranches {
		commits, err = repo.LogAll(maxCommits)
	} else {
		commits, err = repo.Log(maxCommits)
	}
	if err != nil {
		return nil, fmt.Errorf("build graph: load commits: %w", err)
	}

	if len(commits) == 0 {
		return nil, nil
	}

	// Step 2: Build ref map (hash → list of refs).
	refMap, err := buildRefMap(repo, opts)
	if err != nil {
		slog.Warn("failed to build ref map", "error", err)
		refMap = make(map[string][]GraphRef)
	}

	// Get current HEAD hash for marking.
	headHash := ""
	head, err := repo.Head()
	if err == nil {
		headHash = head.Hash().String()
	}

	// Step 3: Assign lanes using topological walk.
	graphCommits := assignLanes(commits, refMap, headHash)

	slog.Debug("graph built", "commits", len(graphCommits))
	return graphCommits, nil
}

// buildRefMap creates a mapping from commit hash to the refs that point to it.
func buildRefMap(repo *Repository, opts GraphOptions) (map[string][]GraphRef, error) {
	refMap := make(map[string][]GraphRef)

	// Local branches.
	branches, err := repo.Branches()
	if err != nil {
		return refMap, err
	}

	for _, b := range branches {
		if b.IsRemote {
			if opts.IncludeRemotes {
				refMap[b.Hash] = append(refMap[b.Hash], GraphRef{
					Name: b.Name,
					Kind: RefRemoteBranch,
				})
			}
		} else {
			ref := GraphRef{
				Name: b.Name,
				Kind: RefLocalBranch,
			}
			if b.IsCurrent {
				ref.Kind = RefHEAD
			}
			refMap[b.Hash] = append(refMap[b.Hash], ref)
		}
	}

	// Tags.
	if opts.IncludeTags {
		tags, err := repo.Tags()
		if err != nil {
			return refMap, err
		}
		for _, t := range tags {
			refMap[t.Hash] = append(refMap[t.Hash], GraphRef{
				Name: t.Name,
				Kind: RefTag,
			})
		}
	}

	return refMap, nil
}

// assignLanes walks commits in order and assigns each one to a lane.
//
// The algorithm maintains a slice of "active lanes". Each active lane
// is associated with a commit hash that the lane is "waiting for" (the
// next expected parent). When we encounter that commit, it takes the
// lane. Freed lanes are reused.
//
// This is a standard git graph layout algorithm similar to what `git log
// --graph` uses internally.
func assignLanes(commits []CommitInfo, refMap map[string][]GraphRef, headHash string) []GraphCommit {
	// activeLanes tracks which commit hash each lane is waiting for.
	// A nil/empty string means the lane is free.
	activeLanes := make([]string, 0)

	// commitLane maps commit hash → lane index (for commits we've already placed).
	commitLane := make(map[string]int)

	result := make([]GraphCommit, 0, len(commits))

	for _, ci := range commits {
		// Find or assign a lane for this commit.
		lane := -1

		// Check if any active lane is waiting for this commit.
		for l, waitingFor := range activeLanes {
			if waitingFor == ci.Hash {
				lane = l
				break
			}
		}

		if lane == -1 {
			// No lane is waiting for this commit. Find a free lane or create a new one.
			lane = findFreeLane(activeLanes)
			if lane == -1 {
				// All lanes are occupied — add a new one.
				lane = len(activeLanes)
				activeLanes = append(activeLanes, "")
			}
		}

		// Cap at MaxLanes.
		if lane >= MaxLanes {
			lane = MaxLanes - 1
		}

		commitLane[ci.Hash] = lane

		// Compute edges to parents.
		edges := make([]GraphEdge, 0, len(ci.ParentHashes))

		for parentIdx, parentHash := range ci.ParentHashes {
			parentLane := -1

			if parentIdx == 0 {
				// First parent: continue in the same lane.
				activeLanes[lane] = parentHash
				parentLane = lane
			} else {
				// Additional parents (merge): find or create a lane for the parent.
				// Check if the parent already has a lane assignment.
				for l, waitingFor := range activeLanes {
					if waitingFor == parentHash {
						parentLane = l
						break
					}
				}

				if parentLane == -1 {
					// Find a free lane for this parent.
					freeLane := findFreeLane(activeLanes)
					if freeLane == -1 {
						freeLane = len(activeLanes)
						activeLanes = append(activeLanes, "")
					}
					activeLanes[freeLane] = parentHash
					parentLane = freeLane
				}
			}

			if parentLane >= MaxLanes {
				parentLane = MaxLanes - 1
			}

			style := EdgeSolid
			if parentLane >= MaxLanes-1 && lane >= MaxLanes-1 {
				style = EdgeDashed
			}

			edges = append(edges, GraphEdge{
				FromLane: lane,
				ToLane:   parentLane,
				Style:    style,
			})
		}

		// If this commit has no parents (root commit), free its lane.
		if len(ci.ParentHashes) == 0 {
			activeLanes[lane] = ""
		}

		// Compute active lanes — all lanes that have a line running through
		// this row (occupied lanes).
		var active []int
		for l, waitingFor := range activeLanes {
			if waitingFor != "" {
				active = append(active, l)
			}
		}

		// Build the GraphCommit.
		gc := GraphCommit{
			Hash:         ci.Hash,
			ShortHash:    ci.ShortHash,
			Subject:      ci.Subject,
			Author:       ci.Author,
			AuthorEmail:  ci.AuthorEmail,
			Timestamp:    ci.AuthorTime,
			Lane:         lane,
			Edges:        edges,
			Refs:         refMap[ci.Hash],
			IsMerge:      ci.IsMerge,
			IsHead:       ci.Hash == headHash,
			ParentHashes: ci.ParentHashes,
			ActiveLanes:  active,
		}

		result = append(result, gc)
	}

	return result
}

// findFreeLane returns the index of the first free lane (empty string),
// or -1 if all lanes are occupied.
func findFreeLane(lanes []string) int {
	for i, l := range lanes {
		if l == "" {
			return i
		}
	}
	return -1
}

// SortGraphByTimestamp sorts graph commits by timestamp (newest first).
// This is useful when the graph needs to be re-sorted after filtering.
func SortGraphByTimestamp(commits []GraphCommit) {
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Timestamp.After(commits[j].Timestamp)
	})
}
