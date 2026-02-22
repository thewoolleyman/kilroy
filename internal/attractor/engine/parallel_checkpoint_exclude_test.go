package engine

import (
	"context"
	"testing"
	"time"
)

func TestRunWithConfig_ParallelBaseCheckpointRespectsCheckpointExcludes(t *testing.T) {
	cleanupStrayEngineArtifacts(t)
	t.Cleanup(func() { cleanupStrayEngineArtifacts(t) })

	repo := initTestRepo(t)
	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)
	cxdbSrv := newCXDBTestServer(t)
	cfg := newCheckpointExcludeConfig(repo, pinned, cxdbSrv)

	dot := []byte(`
digraph G {
  graph [goal="parallel base checkpoint excludes"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  prep [shape=parallelogram, tool_command="mkdir -p src .cargo_target_local/obj && echo prep > src/prep.txt && echo temp > .cargo_target_local/obj/base.bin"]
  fan  [shape=component]
  a    [shape=parallelogram, tool_command="echo a > src/a.txt"]
  b    [shape=parallelogram, tool_command="echo b > src/b.txt"]
  join [shape=tripleoctagon]
  start -> prep -> fan
  fan -> a
  fan -> b
  a -> join
  b -> join
  join -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "parallel-base-checkpoint-exclude", LogsRoot: logsRoot})
	if err != nil {
		t.Fatalf("RunWithConfig: %v", err)
	}
	if res.FinalStatus != "success" {
		t.Fatalf("final status: got %s want success", res.FinalStatus)
	}

	files := gitLsFiles(t, res.WorktreeDir)
	if !containsPath(files, "src/prep.txt") {
		t.Fatalf("expected src/prep.txt to be checkpointed, got %v", files)
	}
	if containsPath(files, ".cargo_target_local/obj/base.bin") {
		t.Fatalf("excluded artifact from pre-fanout stage should not be tracked: %v", files)
	}
}

func TestRunWithConfig_ParallelBranchCheckpointsRespectsCheckpointExcludes(t *testing.T) {
	cleanupStrayEngineArtifacts(t)
	t.Cleanup(func() { cleanupStrayEngineArtifacts(t) })

	repo := initTestRepo(t)
	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)
	cxdbSrv := newCXDBTestServer(t)
	cfg := newCheckpointExcludeConfig(repo, pinned, cxdbSrv)

	dot := []byte(`
digraph G {
  graph [goal="parallel branch checkpoint excludes"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  fan  [shape=component]
  a    [shape=parallelogram, tool_command="mkdir -p src .cargo_target_local/obj && echo a > src/a.txt && echo temp > .cargo_target_local/obj/from-a.bin"]
  b    [shape=parallelogram, tool_command="mkdir -p src && echo b > src/b.txt"]
  join [shape=tripleoctagon]
  start -> fan
  fan -> a
  fan -> b
  a -> join
  b -> join
  join -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "parallel-branch-checkpoint-exclude", LogsRoot: logsRoot})
	if err != nil {
		t.Fatalf("RunWithConfig: %v", err)
	}
	if res.FinalStatus != "success" {
		t.Fatalf("final status: got %s want success", res.FinalStatus)
	}

	files := gitLsFiles(t, res.WorktreeDir)
	if !containsPath(files, "src/a.txt") {
		t.Fatalf("expected winner branch file src/a.txt to be checkpointed, got %v", files)
	}
	if containsPath(files, ".cargo_target_local/obj/from-a.bin") {
		t.Fatalf("excluded artifact from branch checkpoint should not be tracked: %v", files)
	}
}

func TestRunWithConfig_ParallelStreamingBaseCheckpointRespectsCheckpointExcludes(t *testing.T) {
	cleanupStrayEngineArtifacts(t)
	t.Cleanup(func() { cleanupStrayEngineArtifacts(t) })

	repo := initTestRepo(t)
	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)
	cxdbSrv := newCXDBTestServer(t)
	cfg := newCheckpointExcludeConfig(repo, pinned, cxdbSrv)

	dot := []byte(`
digraph G {
  graph [goal="parallel streaming base checkpoint excludes"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  prep [shape=parallelogram, tool_command="mkdir -p src .cargo_target_local/obj && echo prep > src/prep.txt && echo temp > .cargo_target_local/obj/base-stream.bin"]
  fan  [shape=component, join_policy="first_success"]
  a    [shape=parallelogram, tool_command="echo a > src/a.txt"]
  b    [shape=parallelogram, tool_command="sleep 1; echo b > src/b.txt"]
  join [shape=tripleoctagon]
  start -> prep -> fan
  fan -> a
  fan -> b
  a -> join
  b -> join
  join -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "parallel-streaming-base-checkpoint-exclude", LogsRoot: logsRoot})
	if err != nil {
		t.Fatalf("RunWithConfig: %v", err)
	}
	if res.FinalStatus != "success" {
		t.Fatalf("final status: got %s want success", res.FinalStatus)
	}

	files := gitLsFiles(t, res.WorktreeDir)
	if !containsPath(files, "src/prep.txt") {
		t.Fatalf("expected src/prep.txt to be checkpointed, got %v", files)
	}
	if containsPath(files, ".cargo_target_local/obj/base-stream.bin") {
		t.Fatalf("excluded artifact from streaming base checkpoint should not be tracked: %v", files)
	}
}

func newCheckpointExcludeConfig(repoPath, pinnedCatalog string, cxdbSrv *cxdbTestServer) *RunConfigFile {
	cfg := &RunConfigFile{Version: 1}
	cfg.Repo.Path = repoPath
	cfg.CXDB.BinaryAddr = cxdbSrv.BinaryAddr()
	cfg.CXDB.HTTPBaseURL = cxdbSrv.URL()
	cfg.ModelDB.OpenRouterModelInfoPath = pinnedCatalog
	cfg.ModelDB.OpenRouterModelInfoUpdatePolicy = "pinned"
	cfg.Git.RunBranchPrefix = "attractor/run"
	cfg.Git.CheckpointExcludeGlobs = []string{"**/.cargo_target*/**"}
	return cfg
}
