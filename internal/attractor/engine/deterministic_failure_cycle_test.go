package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRun_DeterministicFailureCycle_AbortsInfiniteLoop verifies that when
// every stage in a retry cycle fails with a deterministic failure (e.g.,
// expired auth token), the engine aborts the run instead of looping forever.
//
// The graph has a cycle: implement -> verify -> check -> implement (on fail).
// All tool nodes exit 1 to simulate a persistent provider failure.
// The engine should detect the repeated failure signature and terminate.
func TestRun_DeterministicFailureCycle_AbortsInfiniteLoop(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [default_max_retry=0]
  start [shape=Mdiamond]
  exit [shape=Msquare]

  implement [
    shape=parallelogram,
    tool_command="echo implement_fail >> log.txt; exit 1"
  ]
  verify [
    shape=parallelogram,
    tool_command="echo verify_fail >> log.txt; exit 1"
  ]
  check [shape=diamond]

  start -> implement
  implement -> verify
  verify -> check
  check -> implement [condition="outcome=fail", label="retry"]
  check -> exit [condition="outcome=success"]
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := Run(ctx, dot, RunOptions{RepoPath: repo, RunID: "detfailcycle", LogsRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected run to abort with deterministic failure cycle error, but it succeeded")
	}
	if !strings.Contains(err.Error(), "deterministic failure cycle") {
		t.Fatalf("expected deterministic failure cycle error, got: %v", err)
	}
}

// TestRun_DeterministicFailure_SingleRouteToRecovery_StillWorks verifies
// that a single deterministic failure that routes to a recovery node (not a
// cycle) still works correctly — we don't want the cycle breaker to be too
// aggressive and block legitimate fail-routing.
func TestRun_DeterministicFailure_SingleRouteToRecovery_StillWorks(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [default_max_retry=0]
  start [shape=Mdiamond]
  exit [shape=Msquare]

  attempt [
    shape=parallelogram,
    tool_command="exit 1"
  ]
  recovery [
    shape=parallelogram,
    tool_command="echo recovered > result.txt"
  ]

  start -> attempt -> exit
  attempt -> recovery [condition="outcome=fail"]
  recovery -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo, RunID: "detfailrecovery", LogsRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	resultBytes, err := os.ReadFile(filepath.Join(res.WorktreeDir, "result.txt"))
	if err != nil {
		t.Fatalf("read result.txt: %v", err)
	}
	if got := strings.TrimSpace(string(resultBytes)); got != "recovered" {
		t.Fatalf("result.txt: got %q want %q", got, "recovered")
	}
}

func TestRunSubgraphUntil_DeterministicFailureCycleBreaksAtLimit(t *testing.T) {
	err := runDeterministicSubgraphCycleFixture(t, 2)
	if err == nil || !strings.Contains(err.Error(), "deterministic failure cycle") {
		t.Fatalf("expected deterministic failure cycle error, got %v", err)
	}
}

func TestDeterministicFailureCycleBreaker_IgnoresCanceledClass(t *testing.T) {
	err := runCanceledCycleFixture(t)
	if err != nil && strings.Contains(err.Error(), "deterministic failure cycle") {
		t.Fatalf("canceled failures should not trip deterministic cycle breaker: %v", err)
	}
}

// TestRun_DeterministicFailureCycle_ImplSucceedsVerifyFails verifies that the
// cycle breaker fires even when impl succeeds between verify failures. This
// is the pathological pattern from the Rogue pipeline incident: impl_combat_items
// succeeded 35 times but verify kept failing with write_scope_violation, and the
// breaker never tripped because success-reset zeroed the counter.
//
// With the success-reset removed, verify's deterministic failure signature now
// accumulates across cycles: count=1 after first verify fail, count=2 after
// second, count=3 triggers the breaker.
func TestRun_DeterministicFailureCycle_ImplSucceedsVerifyFails(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	// impl succeeds (exit 0), verify always fails (exit 1) with a deterministic reason.
	// check diamond routes back to impl on fail.
	dot := []byte(`
digraph G {
  graph [default_max_retry=0, loop_restart_signature_limit=3]
  start [shape=Mdiamond]
  exit [shape=Msquare]

  implement [
    shape=parallelogram,
    tool_command="echo impl_ok >> log.txt"
  ]
  verify [
    shape=parallelogram,
    tool_command="echo verify_fail >> log.txt; exit 1"
  ]
  check [shape=diamond]

  start -> implement
  implement -> verify
  verify -> check
  check -> implement [condition="outcome=fail", label="retry"]
  check -> exit [condition="outcome=success"]
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := Run(ctx, dot, RunOptions{RepoPath: repo, RunID: "impl-ok-verify-fail", LogsRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected run to abort with deterministic failure cycle error, but it succeeded")
	}
	if !strings.Contains(err.Error(), "deterministic failure cycle") {
		t.Fatalf("expected deterministic failure cycle error, got: %v", err)
	}
}

// TestRunSubgraphUntil_StructuralFailureAbortsImmediately verifies that a
// write_scope_violation in a parallel branch aborts the branch immediately
// rather than burning cycles through the signature limit.
func TestRunSubgraphUntil_StructuralFailureAbortsImmediately(t *testing.T) {
	repo := initTestRepo(t)
	logsRoot := t.TempDir()

	dot := []byte(`
digraph G {
  graph [goal="structural failure fixture", loop_restart_signature_limit=10]
  start [shape=Mdiamond]
  exit [shape=Msquare]

  implement [shape=diamond, type="structural_impl_fixture"]
  verify [shape=diamond, type="structural_verify_fixture"]
  check [shape=diamond]

  start -> implement
  implement -> verify
  verify -> check
  check -> implement [condition="outcome=fail"]
  check -> exit [condition="outcome=success"]
}
`)

	eng := newReliabilityFixtureEngine(t, repo, logsRoot, "structural-failure-fixture", dot)

	// implement always succeeds; verify always fails with write_scope_violation.
	eng.Registry.Register("structural_impl_fixture", &structuralImplFixtureHandler{})
	eng.Registry.Register("structural_verify_fixture", &structuralVerifyFixtureHandler{})

	_, err := runSubgraphUntil(context.Background(), eng, "implement", "")
	if err == nil {
		t.Fatalf("expected structural failure abort, but subgraph succeeded")
	}
	if !strings.Contains(err.Error(), "structural failure in branch") {
		t.Fatalf("expected structural failure error, got: %v", err)
	}

	// Verify it aborted immediately (verify should run exactly once, not
	// loop_restart_signature_limit times).
	events := readFixtureProgressEvents(t, filepath.Join(logsRoot, "progress.ndjson"))
	abortCount := 0
	for _, ev := range events {
		if strings.TrimSpace(fmt.Sprint(ev["event"])) == "subgraph_structural_failure_abort" {
			abortCount++
		}
	}
	if abortCount != 1 {
		t.Fatalf("expected exactly 1 structural abort event, got %d", abortCount)
	}
}

// TestRun_StructuralFailure_AccumulatesInMainLoop verifies that structural
// failures in the main loop (not subgraph) are tracked by the signature-based
// cycle breaker rather than immediately aborting — the main loop may have
// user-designed recovery edges.
func TestRun_StructuralFailure_AccumulatesInMainLoop(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	// verify always fails with write_scope_violation (structural).
	// The cycle breaker should detect the repeated structural signature and abort.
	dot := []byte(`
digraph G {
  graph [default_max_retry=0, loop_restart_signature_limit=3]
  start [shape=Mdiamond]
  exit [shape=Msquare]

  implement [
    shape=parallelogram,
    tool_command="echo impl_ok >> log.txt"
  ]
  verify [
    shape=parallelogram,
    tool_command="echo 'write_scope_violation: file outside declared scope' >&2; exit 1"
  ]
  check [shape=diamond]

  start -> implement
  implement -> verify
  verify -> check
  check -> implement [condition="outcome=fail", label="retry"]
  check -> exit [condition="outcome=success"]
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := Run(ctx, dot, RunOptions{RepoPath: repo, RunID: "structural-main-loop", LogsRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected run to abort with deterministic failure cycle error, but it succeeded")
	}
	if !strings.Contains(err.Error(), "deterministic failure cycle") {
		t.Fatalf("expected deterministic failure cycle error from structural accumulation, got: %v", err)
	}
}
