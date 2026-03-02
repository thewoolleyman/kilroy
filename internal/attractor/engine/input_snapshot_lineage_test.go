package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInputSnapshotLineage_ForkBranchAndAdvanceRevision(t *testing.T) {
	lineage := newInputSnapshotLineage("run-123")
	r0 := lineage.CreateRunRevision("", nil)
	b0, err := lineage.ForkBranch("branch-a", r0)
	if err != nil {
		t.Fatalf("ForkBranch: %v", err)
	}
	b1 := lineage.AdvanceBranch("branch-a", b0, map[string]string{"postmortem_latest.md": "sha256:aaa"})
	if lineage.BranchHeads["branch-a"] != b1 {
		t.Fatalf("branch head mismatch")
	}
}

func TestInputSnapshotLineage_MergePromotionsDetectsConflict(t *testing.T) {
	lineage := newInputSnapshotLineage("run-123")
	r0 := lineage.CreateRunRevision("", map[string]string{})
	a0, _ := lineage.ForkBranch("a", r0)
	b0, _ := lineage.ForkBranch("b", r0)
	lineage.Revisions[a0] = InputSnapshotRev{
		ID:         a0,
		ParentIDs:  []string{r0},
		Scope:      "branch",
		BranchKey:  "a",
		FileDigest: map[string]string{"postmortem_latest.md": "sha256:aaa"},
	}
	lineage.Revisions[b0] = InputSnapshotRev{
		ID:         b0,
		ParentIDs:  []string{r0},
		Scope:      "branch",
		BranchKey:  "b",
		FileDigest: map[string]string{"postmortem_latest.md": "sha256:bbb"},
	}

	_, conflicts, err := lineage.MergePromotedPaths([]string{"postmortem_latest.md"}, map[string]string{"a": a0, "b": b0})
	if err == nil || !strings.Contains(err.Error(), "input_snapshot_conflict") {
		t.Fatalf("expected input_snapshot_conflict, got %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Path != "postmortem_latest.md" {
		t.Fatalf("unexpected conflicts payload: %+v", conflicts)
	}
}

func TestInputSnapshotLineage_BranchAdvanceDoesNotMutateRunHead(t *testing.T) {
	lineage := newInputSnapshotLineage("run-123")
	r0 := lineage.CreateRunRevision("", map[string]string{})
	lineage.RunHead = r0
	b0, _ := lineage.ForkBranch("a", r0)
	_ = lineage.AdvanceBranch("a", b0, map[string]string{"postmortem_latest.md": "sha256:aaa"})
	if lineage.RunHead != r0 {
		t.Fatalf("branch advancement must not mutate run head before fan-in: got %q want %q", lineage.RunHead, r0)
	}
}

func TestInputSnapshotLineage_MergePromotionsConflictOrdering(t *testing.T) {
	lineage := newInputSnapshotLineage("run-123")
	r0 := lineage.CreateRunRevision("", map[string]string{})
	a0, _ := lineage.ForkBranch("a", r0)
	b0, _ := lineage.ForkBranch("b", r0)
	lineage.Revisions[a0] = InputSnapshotRev{
		ID:         a0,
		ParentIDs:  []string{r0},
		Scope:      "branch",
		BranchKey:  "a",
		FileDigest: map[string]string{"z.md": "sha256:aaa", "a.md": "sha256:aaa"},
	}
	lineage.Revisions[b0] = InputSnapshotRev{
		ID:         b0,
		ParentIDs:  []string{r0},
		Scope:      "branch",
		BranchKey:  "b",
		FileDigest: map[string]string{"z.md": "sha256:bbb", "a.md": "sha256:bbb"},
	}
	_, conflicts, err := lineage.MergePromotedPaths([]string{"a.md", "z.md"}, map[string]string{"a": a0, "b": b0})
	if err == nil || !strings.Contains(err.Error(), "input_snapshot_conflict") {
		t.Fatalf("expected input_snapshot_conflict, got %v", err)
	}
	if len(conflicts) != 2 || conflicts[0].Path != "a.md" || conflicts[1].Path != "z.md" {
		t.Fatalf("conflicts must be sorted by path: %+v", conflicts)
	}
}

func TestInputSnapshotLineage_SaveLoadAtomic(t *testing.T) {
	logsRoot := t.TempDir()
	lineage := newInputSnapshotLineage("run-123")
	r0 := lineage.CreateRunRevision("", map[string]string{"review_final.md": "sha256:abc"})
	lineage.RunHead = r0
	if err := lineage.SaveAtomic(logsRoot); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	if _, err := os.Stat(filepath.Join(logsRoot, "input_snapshot", "lineage.json")); err != nil {
		t.Fatalf("lineage.json should exist: %v", err)
	}

	loaded, err := LoadInputSnapshotLineage(logsRoot)
	if err != nil {
		t.Fatalf("LoadInputSnapshotLineage: %v", err)
	}
	if loaded.RunHead != r0 {
		t.Fatalf("run head mismatch: got %q want %q", loaded.RunHead, r0)
	}
}
