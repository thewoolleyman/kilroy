package engine

import (
	"reflect"
	"testing"
)

func TestDetectMaterializeFieldPresenceYAML_ExplicitEmptyLegacyFields(t *testing.T) {
	p, err := detectMaterializeFieldPresence("run.yaml", []byte(`
version: 1
inputs:
  materialize:
    include: []
    default_include: []
`))
	if err != nil {
		t.Fatalf("detectMaterializeFieldPresence: %v", err)
	}
	if !p.IncludeDeclared {
		t.Fatal("expected include to be declared")
	}
	if !p.DefaultIncludeDeclared {
		t.Fatal("expected default_include to be declared")
	}
}

func TestDetectMaterializeFieldPresenceJSON_ExplicitEmptyLegacyFields(t *testing.T) {
	p, err := detectMaterializeFieldPresence("run.json", []byte(`{
  "version": 1,
  "inputs": {
    "materialize": {
      "include": [],
      "default_include": []
    }
  }
}`))
	if err != nil {
		t.Fatalf("detectMaterializeFieldPresence: %v", err)
	}
	if !p.IncludeDeclared {
		t.Fatal("expected include to be declared")
	}
	if !p.DefaultIncludeDeclared {
		t.Fatal("expected default_include to be declared")
	}
}

func TestNormalizeAndValidatePromoteRunScoped_NormalizesSlashesAndDedupe(t *testing.T) {
	got, err := normalizeAndValidatePromoteRunScoped([]string{
		`foo\bar.md`,
		"./foo/bar.md",
		"review_final.md",
		"review_final.md",
	})
	if err != nil {
		t.Fatalf("normalizeAndValidatePromoteRunScoped: %v", err)
	}
	want := []string{"foo/bar.md", "review_final.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized promote_run_scoped mismatch: got %v want %v", got, want)
	}
}

func TestNormalizeAndValidatePromoteRunScoped_RejectsDotDot(t *testing.T) {
	_, err := normalizeAndValidatePromoteRunScoped([]string{"safe/../escape.md"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
