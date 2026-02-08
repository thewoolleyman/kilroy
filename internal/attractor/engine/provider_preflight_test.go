package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWithConfig_FailsFast_WhenCLIModelNotInCatalogForProvider(t *testing.T) {
	repo := initTestRepo(t)
	catalog := writeCatalogForPreflight(t, `{
  "gemini/gemini-3-pro-preview": {
    "litellm_provider": "google",
    "mode": "chat"
  }
}`)

	cfg := testPreflightConfig(repo, catalog)
	dot := []byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  a [shape=box, llm_provider=google, llm_model=gemini-3-pro, prompt="x"]
  exit [shape=Msquare]
  start -> a -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "preflight-fail", LogsRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected preflight error, got nil")
	}
	want := "preflight: llm_provider=google backend=cli model=gemini-3-pro not present in run catalog"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected preflight error containing %q, got %v", want, err)
	}
}

func TestRunWithConfig_AllowsCLIModel_WhenCatalogHasProviderMatch(t *testing.T) {
	repo := initTestRepo(t)
	catalog := writeCatalogForPreflight(t, `{
  "gemini/gemini-3-pro-preview": {
    "litellm_provider": "google",
    "mode": "chat"
  }
}`)

	cfg := testPreflightConfig(repo, catalog)
	dot := []byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  a [shape=box, llm_provider=google, llm_model=gemini-3-pro-preview, prompt="x"]
  exit [shape=Msquare]
  start -> a -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "preflight-pass", LogsRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected downstream error after preflight (cxdb is intentionally unreachable), got nil")
	}
	if strings.Contains(err.Error(), "preflight:") {
		t.Fatalf("expected preflight to pass for provider/model in catalog, got %v", err)
	}
}

func testPreflightConfig(repo string, catalog string) *RunConfigFile {
	cfg := &RunConfigFile{Version: 1}
	cfg.Repo.Path = repo
	cfg.CXDB.BinaryAddr = "127.0.0.1:1"
	cfg.CXDB.HTTPBaseURL = "http://127.0.0.1:1"
	cfg.LLM.Providers = map[string]struct {
		Backend BackendKind `json:"backend" yaml:"backend"`
	}{
		"google": {Backend: BackendCLI},
	}
	cfg.ModelDB.LiteLLMCatalogPath = catalog
	cfg.ModelDB.LiteLLMCatalogUpdatePolicy = "pinned"
	cfg.Git.RunBranchPrefix = "attractor/run"
	return cfg
}

func writeCatalogForPreflight(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	return p
}
