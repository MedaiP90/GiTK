package git

import (
	"strings"
	"testing"
)

func TestThreeWayMerge_NoConflict_OursOnly(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "line1\nmodified\nline3\n"
	theirs := base

	result := ThreeWayMerge("test.txt", base, ours, theirs)
	if result.HasConflicts {
		t.Errorf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.MergedContent != ours {
		t.Errorf("expected ours content, got %q", result.MergedContent)
	}
}

func TestThreeWayMerge_NoConflict_TheirsOnly(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := base
	theirs := "line1\nchanged\nline3\n"

	result := ThreeWayMerge("test.txt", base, ours, theirs)
	if result.HasConflicts {
		t.Errorf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.MergedContent != theirs {
		t.Errorf("expected theirs content, got %q", result.MergedContent)
	}
}

func TestThreeWayMerge_BothSame(t *testing.T) {
	base := "original\n"
	ours := "changed\n"
	theirs := "changed\n"

	result := ThreeWayMerge("test.txt", base, ours, theirs)
	if result.HasConflicts {
		t.Error("expected no conflicts when both made same change")
	}
}

func TestThreeWayMerge_Conflict(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "line1\nours change\nline3\n"
	theirs := "line1\ntheirs change\nline3\n"

	result := ThreeWayMerge("test.txt", base, ours, theirs)
	if !result.HasConflicts {
		t.Error("expected conflicts")
	}
	if len(result.Conflicts) == 0 {
		t.Error("expected at least one conflict")
	}
	if !strings.Contains(result.MergedContent, "<<<<<<< OURS") {
		t.Error("expected conflict markers in merged content")
	}
}

func TestApplyResolution_Ours(t *testing.T) {
	c := ConflictRegion{
		OursText:   "ours",
		TheirsText: "theirs",
	}
	c.ApplyResolution(ResolveOurs)

	if !c.Resolved {
		t.Error("expected conflict to be resolved")
	}
	if c.Resolution != "ours" {
		t.Errorf("expected 'ours', got %q", c.Resolution)
	}
}

func TestApplyResolution_Theirs(t *testing.T) {
	c := ConflictRegion{
		OursText:   "ours",
		TheirsText: "theirs",
	}
	c.ApplyResolution(ResolveTheirs)

	if c.Resolution != "theirs" {
		t.Errorf("expected 'theirs', got %q", c.Resolution)
	}
}

func TestApplyResolution_BothOursFirst(t *testing.T) {
	c := ConflictRegion{
		OursText:   "ours",
		TheirsText: "theirs",
	}
	c.ApplyResolution(ResolveBothOursFirst)

	if c.Resolution != "ours\ntheirs" {
		t.Errorf("expected 'ours\\ntheirs', got %q", c.Resolution)
	}
}

func TestApplyResolutions(t *testing.T) {
	result := &MergeResult{
		HasConflicts:  true,
		MergedContent: "line1\n<<<<<<< OURS\nours\n=======\ntheirs\n>>>>>>> THEIRS\nline3",
		Conflicts: []ConflictRegion{
			{Resolved: true, Resolution: "resolved"},
		},
	}

	output := ApplyResolutions(result)
	if strings.Contains(output, "<<<<<<<") {
		t.Error("expected conflict markers to be removed")
	}
	if !strings.Contains(output, "resolved") {
		t.Error("expected resolution text in output")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"one\n", 1},
		{"one\ntwo\n", 2},
		{"no trailing newline", 1},
	}

	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.expected {
			t.Errorf("splitLines(%q): got %d lines, want %d", tt.input, len(got), tt.expected)
		}
	}
}
