package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFinalOutcome_Save_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "final.json")
	fo := &FinalOutcome{
		Timestamp:         time.Unix(123, 0).UTC(),
		Status:            FinalSuccess,
		RunID:             "r1",
		FinalGitCommitSHA: "abc",
		CXDBContextID:     "c1",
		CXDBHeadTurnID:    "t1",
	}
	if err := fo.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestWriteFileAtomic_OverwritesTargetAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	if err := os.WriteFile(path, []byte(`{"status":"old"}`), 0o644); err != nil {
		t.Fatalf("seed status file: %v", err)
	}
	if err := WriteFileAtomic(path, []byte(`{"status":"new"}`)); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	if got := string(b); got != `{"status":"new"}` {
		t.Fatalf("status payload: got %q want %q", got, `{"status":"new"}`)
	}
}
