package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRunWithMockClaude tests the full Run pipeline using a mock claude script
// that outputs a known .dot file.
func TestRunWithMockClaude(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// Read a known-good .dot file to use as mock output.
	dotPath := filepath.Join(repoRoot, "research", "refactor-test-vague.dot")
	dotContent, err := os.ReadFile(dotPath)
	if err != nil {
		t.Skipf("research dot file not found: %v", err)
	}

	// Create a mock claude script that outputs the .dot content.
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "claude")
	err = os.WriteFile(mockScript, []byte("#!/bin/sh\ncat '"+dotPath+"'\n"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	// Create a minimal skill file.
	skillPath := filepath.Join(tmpDir, "SKILL.md")
	err = os.WriteFile(skillPath, []byte("# Test Skill\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Point KILROY_CLAUDE_PATH at the mock.
	t.Setenv("KILROY_CLAUDE_PATH", mockScript)

	result, err := Run(context.Background(), Options{
		Requirements: "solitaire plz",
		SkillPath:    skillPath,
		Model:        "claude-sonnet-4-5",
		RepoPath:     repoRoot,
		Validate:     true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.DotContent == "" {
		t.Fatal("DotContent is empty")
	}
	if result.DotContent[:7] != "digraph" {
		t.Errorf("DotContent should start with 'digraph', got %q", result.DotContent[:20])
	}

	// RawOutput should contain the full content.
	if len(result.RawOutput) < len(result.DotContent) {
		t.Error("RawOutput should be >= DotContent length")
	}

	// Check that the extracted content matches what we'd get from direct extraction.
	directExtract, err := ExtractDigraph(string(dotContent))
	if err != nil {
		t.Fatalf("direct ExtractDigraph failed: %v", err)
	}
	if result.DotContent != directExtract {
		t.Error("Run extraction differs from direct ExtractDigraph")
	}

	t.Logf("DotContent length: %d bytes", len(result.DotContent))
	t.Logf("Warnings: %v", result.Warnings)
}

// TestRunWithMockClaudeWrappedOutput tests that Run handles claude output
// that includes commentary around the digraph.
func TestRunWithMockClaudeWrappedOutput(t *testing.T) {
	repoRoot := findRepoRoot(t)

	dotPath := filepath.Join(repoRoot, "research", "refactor-test-moderate.dot")
	dotContent, err := os.ReadFile(dotPath)
	if err != nil {
		t.Skipf("research dot file not found: %v", err)
	}

	// Create a mock that wraps the dot output in commentary.
	tmpDir := t.TempDir()
	wrappedPath := filepath.Join(tmpDir, "wrapped_output.txt")
	wrapped := "Here is the pipeline I generated:\n\n" + string(dotContent) + "\n\nThis pipeline implements the requirements."
	err = os.WriteFile(wrappedPath, []byte(wrapped), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	mockScript := filepath.Join(tmpDir, "claude")
	err = os.WriteFile(mockScript, []byte("#!/bin/sh\ncat '"+wrappedPath+"'\n"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(tmpDir, "SKILL.md")
	err = os.WriteFile(skillPath, []byte("# Test Skill\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("KILROY_CLAUDE_PATH", mockScript)

	result, err := Run(context.Background(), Options{
		Requirements: "Build a link checker CLI",
		SkillPath:    skillPath,
		Model:        "claude-sonnet-4-5",
		RepoPath:     repoRoot,
		Validate:     true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.DotContent[:7] != "digraph" {
		t.Errorf("should start with 'digraph', got %q", result.DotContent[:20])
	}
	if result.DotContent[len(result.DotContent)-1] != '}' {
		t.Errorf("should end with '}', got %q", result.DotContent[len(result.DotContent)-5:])
	}

	t.Logf("Extracted %d bytes from wrapped output of %d bytes", len(result.DotContent), len(result.RawOutput))
}

// TestRunWithMockClaudeFailure tests that Run returns an error when claude fails.
func TestRunWithMockClaudeFailure(t *testing.T) {
	tmpDir := t.TempDir()

	mockScript := filepath.Join(tmpDir, "claude")
	err := os.WriteFile(mockScript, []byte("#!/bin/sh\necho 'API error' >&2\nexit 1\n"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(tmpDir, "SKILL.md")
	err = os.WriteFile(skillPath, []byte("# Test Skill\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("KILROY_CLAUDE_PATH", mockScript)

	_, err = Run(context.Background(), Options{
		Requirements: "Build something",
		SkillPath:    skillPath,
		Model:        "claude-sonnet-4-5",
	})
	if err == nil {
		t.Fatal("expected error when claude fails")
	}
	t.Logf("Got expected error: %v", err)
}
