package git

import (
	"testing"
	"time"
)

func TestAssignLanes_SingleBranch(t *testing.T) {
	commits := []CommitInfo{
		{Hash: "aaa", ShortHash: "aaa", Subject: "C3", AuthorTime: time.Now(), ParentHashes: []string{"bbb"}},
		{Hash: "bbb", ShortHash: "bbb", Subject: "C2", AuthorTime: time.Now(), ParentHashes: []string{"ccc"}},
		{Hash: "ccc", ShortHash: "ccc", Subject: "C1", AuthorTime: time.Now(), ParentHashes: []string{}},
	}

	refMap := make(map[string][]GraphRef)
	result := assignLanes(commits, refMap, "aaa")

	if len(result) != 3 {
		t.Fatalf("expected 3 graph commits, got %d", len(result))
	}

	// All commits should be in lane 0 for a linear history.
	for i, gc := range result {
		if gc.Lane != 0 {
			t.Errorf("commit %d: expected lane 0, got %d", i, gc.Lane)
		}
	}

	// First commit should be HEAD.
	if !result[0].IsHead {
		t.Error("first commit should be HEAD")
	}
}

func TestAssignLanes_MergeCommit(t *testing.T) {
	commits := []CommitInfo{
		{Hash: "merge", ShortHash: "merge", Subject: "Merge", AuthorTime: time.Now(), ParentHashes: []string{"aaa", "bbb"}, IsMerge: true},
		{Hash: "aaa", ShortHash: "aaa", Subject: "A", AuthorTime: time.Now(), ParentHashes: []string{"root"}},
		{Hash: "bbb", ShortHash: "bbb", Subject: "B", AuthorTime: time.Now(), ParentHashes: []string{"root"}},
		{Hash: "root", ShortHash: "root", Subject: "Root", AuthorTime: time.Now(), ParentHashes: []string{}},
	}

	refMap := make(map[string][]GraphRef)
	result := assignLanes(commits, refMap, "merge")

	if !result[0].IsMerge {
		t.Error("first commit should be a merge")
	}

	if len(result[0].Edges) != 2 {
		t.Errorf("merge commit should have 2 edges, got %d", len(result[0].Edges))
	}
}

func TestFindFreeLane(t *testing.T) {
	tests := []struct {
		lanes    []string
		expected int
	}{
		{[]string{}, -1},
		{[]string{"abc"}, -1},
		{[]string{""}, 0},
		{[]string{"abc", "", "def"}, 1},
		{[]string{"abc", "def", ""}, 2},
	}

	for _, tt := range tests {
		got := findFreeLane(tt.lanes)
		if got != tt.expected {
			t.Errorf("findFreeLane(%v): got %d, want %d", tt.lanes, got, tt.expected)
		}
	}
}

func TestSortGraphByTimestamp(t *testing.T) {
	now := time.Now()
	commits := []GraphCommit{
		{Hash: "old", Timestamp: now.Add(-2 * time.Hour)},
		{Hash: "new", Timestamp: now},
		{Hash: "mid", Timestamp: now.Add(-1 * time.Hour)},
	}

	SortGraphByTimestamp(commits)

	if commits[0].Hash != "new" {
		t.Error("expected newest commit first")
	}
	if commits[2].Hash != "old" {
		t.Error("expected oldest commit last")
	}
}

func TestDefaultGraphOptions(t *testing.T) {
	opts := DefaultGraphOptions()
	if !opts.AllBranches {
		t.Error("AllBranches should be true by default")
	}
	if opts.MaxCommits != 2000 {
		t.Errorf("MaxCommits should be 2000, got %d", opts.MaxCommits)
	}
}
