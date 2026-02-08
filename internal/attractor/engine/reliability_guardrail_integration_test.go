package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/strongdm/kilroy/internal/attractor/model"
	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

func TestReliabilityGuardrail_DeterministicFailureFailsFast_NoRetryNoRestart(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [goal="guardrail deterministic", max_restarts="20"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  work  [shape=box, llm_provider=openai, llm_model=gpt-5.2, max_retries=3, prompt="do work"]
  check [shape=diamond]
  start -> work
  work -> check
  check -> exit [condition="outcome=success"]
  check -> work [condition="outcome=fail", loop_restart=true]
}
`)

	var calls atomic.Int32
	backend := &countingBackend{
		fn: func(ctx context.Context, exec *Execution, node *model.Node, prompt string) (string, *runtime.Outcome, error) {
			if node.ID == "work" {
				calls.Add(1)
				return "fail", &runtime.Outcome{Status: runtime.StatusFail, FailureReason: "unknown flag: --verbose"}, nil
			}
			return "ok", &runtime.Outcome{Status: runtime.StatusSuccess}, nil
		},
	}

	g, _, err := Prepare(dot)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	logsRoot := t.TempDir()
	eng := &Engine{
		Graph:           g,
		Options:         RunOptions{RepoPath: repo, RunID: "guardrail-deterministic", LogsRoot: logsRoot, WorktreeDir: filepath.Join(logsRoot, "worktree"), RunBranchPrefix: "attractor/run", RequireClean: true},
		DotSource:       dot,
		LogsRoot:        logsRoot,
		WorktreeDir:     filepath.Join(logsRoot, "worktree"),
		Context:         runtime.NewContext(),
		Registry:        NewDefaultRegistry(),
		Interviewer:     &AutoApproveInterviewer{},
		CodergenBackend: backend,
	}
	eng.RunBranch = "attractor/run/guardrail-deterministic"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err = eng.run(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("work attempts=%d want=1 (deterministic should fail fast)", got)
	}
	if _, statErr := os.Stat(filepath.Join(logsRoot, "restart-1")); statErr == nil {
		t.Fatalf("restart-1 should not exist for deterministic failure")
	}

	finalBytes, err := os.ReadFile(filepath.Join(logsRoot, "final.json"))
	if err != nil {
		t.Fatalf("read final.json: %v", err)
	}
	var final runtime.FinalOutcome
	if err := json.Unmarshal(finalBytes, &final); err != nil {
		t.Fatalf("unmarshal final.json: %v", err)
	}
	if final.Status != runtime.FinalFail {
		t.Fatalf("final.status=%q want=%q", final.Status, runtime.FinalFail)
	}
	if strings.TrimSpace(final.FailureReason) == "" {
		t.Fatalf("final.failure_reason should be non-empty")
	}
}

func TestReliabilityGuardrail_TransientLoopRestart_CircuitBreaksBeforeMaxRestarts(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [goal="guardrail transient", max_restarts="20", restart_signature_limit="2"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  work  [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="do work"]
  check [shape=diamond]
  start -> work
  work -> check
  check -> exit [condition="outcome=success"]
  check -> work [condition="outcome=fail", loop_restart=true]
}
`)

	var calls atomic.Int32
	backend := &countingBackend{
		fn: func(ctx context.Context, exec *Execution, node *model.Node, prompt string) (string, *runtime.Outcome, error) {
			if node.ID == "work" {
				calls.Add(1)
			}
			return "fail", &runtime.Outcome{Status: runtime.StatusFail, FailureReason: "request timeout after 10s"}, nil
		},
	}

	g, _, err := Prepare(dot)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	logsRoot := t.TempDir()
	eng := &Engine{
		Graph:           g,
		Options:         RunOptions{RepoPath: repo, RunID: "guardrail-transient", LogsRoot: logsRoot, WorktreeDir: filepath.Join(logsRoot, "worktree"), RunBranchPrefix: "attractor/run", RequireClean: true},
		DotSource:       dot,
		LogsRoot:        logsRoot,
		WorktreeDir:     filepath.Join(logsRoot, "worktree"),
		Context:         runtime.NewContext(),
		Registry:        NewDefaultRegistry(),
		Interviewer:     &AutoApproveInterviewer{},
		CodergenBackend: backend,
	}
	eng.RunBranch = "attractor/run/guardrail-transient"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err = eng.run(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Fatalf("expected circuit breaker error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(logsRoot, "restart-3")); statErr == nil {
		t.Fatalf("restart-3 should not exist when restart_signature_limit=2")
	}
	if got := calls.Load(); got >= 20 {
		t.Fatalf("work attempts=%d should remain below max_restarts", got)
	}

	finalBytes, err := os.ReadFile(filepath.Join(eng.LogsRoot, "final.json"))
	if err != nil {
		t.Fatalf("read final.json: %v", err)
	}
	var final runtime.FinalOutcome
	if err := json.Unmarshal(finalBytes, &final); err != nil {
		t.Fatalf("unmarshal final.json: %v", err)
	}
	if !strings.Contains(final.FailureReason, "failure_signature") {
		t.Fatalf("final.failure_reason missing signature context: %q", final.FailureReason)
	}
}

func TestReliabilityGuardrail_ResumePreservesParallelBranchPrefix(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	logsRoot := t.TempDir()
	dot := []byte(`
digraph G {
  graph [goal="resume parallel guardrail"]
  start [shape=Mdiamond]
  par [shape=component]
  a [shape=parallelogram, tool_command="echo a > a.txt"]
  b [shape=parallelogram, tool_command="echo b > b.txt"]
  join [shape=tripleoctagon]
  exit [shape=Msquare]

  start -> par
  par -> a
  par -> b
  a -> join
  b -> join
  join -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{
		RepoPath:        repo,
		RunID:           "guardrail-resume",
		LogsRoot:        logsRoot,
		RunBranchPrefix: "attractor/run",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	startSHA := findCommitForNode(t, repo, res.RunBranch, res.RunID, "start")
	cpPath := filepath.Join(res.LogsRoot, "checkpoint.json")
	cp, err := runtime.LoadCheckpoint(cpPath)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	cp.CurrentNode = "start"
	cp.CompletedNodes = []string{"start"}
	cp.NodeRetries = map[string]int{}
	cp.GitCommitSHA = startSHA
	if err := cp.Save(cpPath); err != nil {
		t.Fatalf("Save checkpoint: %v", err)
	}

	if _, err := Resume(ctx, res.LogsRoot); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	parallelBytes, err := os.ReadFile(filepath.Join(res.LogsRoot, "par", "parallel_results.json"))
	if err != nil {
		t.Fatalf("read parallel_results.json: %v", err)
	}
	var results []map[string]any
	if err := json.Unmarshal(parallelBytes, &results); err != nil {
		t.Fatalf("unmarshal parallel_results.json: %v", err)
	}
	wantPrefix := "attractor/run/parallel/" + res.RunID + "/"
	for _, r := range results {
		branchName := strings.TrimSpace(anyToString(r["branch_name"]))
		if strings.HasPrefix(branchName, "/parallel/") {
			t.Fatalf("invalid branch name: %q", branchName)
		}
		if !strings.HasPrefix(branchName, wantPrefix) {
			t.Fatalf("branch name %q missing prefix %q", branchName, wantPrefix)
		}
	}
}
