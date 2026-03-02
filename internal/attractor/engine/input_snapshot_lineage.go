package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

const (
	inputLineageFileName = "lineage.json"
	inputRevisionDirName = "revisions"
)

type InputSnapshotLineage struct {
	RunID       string                      `json:"run_id"`
	RunHead     string                      `json:"run_head_revision"`
	BranchHeads map[string]string           `json:"branch_heads"`
	Revisions   map[string]InputSnapshotRev `json:"revisions"`
}

type InputSnapshotRev struct {
	ID          string            `json:"id"`
	ParentIDs   []string          `json:"parent_ids"`
	Scope       string            `json:"scope"`
	BranchKey   string            `json:"branch_key,omitempty"`
	FileDigest  map[string]string `json:"run_scoped_file_digest"`
	GeneratedAt string            `json:"generated_at"`
}

type InputSnapshotConflict struct {
	Path          string                            `json:"path"`
	BranchDigests []InputSnapshotConflictDigestPair `json:"branch_digests"`
}

type InputSnapshotConflictDigestPair struct {
	BranchKey  string `json:"branch_key"`
	RevisionID string `json:"revision_id"`
	Digest     string `json:"digest"`
}

func newInputSnapshotLineage(runID string) *InputSnapshotLineage {
	return &InputSnapshotLineage{
		RunID:       strings.TrimSpace(runID),
		BranchHeads: map[string]string{},
		Revisions:   map[string]InputSnapshotRev{},
	}
}

func inputLineagePath(logsRoot string) string {
	return filepath.Join(strings.TrimSpace(logsRoot), inputSnapshotDirName, inputLineageFileName)
}

func inputRevisionRoot(logsRoot string, revID string) string {
	return filepath.Join(strings.TrimSpace(logsRoot), inputSnapshotDirName, inputRevisionDirName, strings.TrimSpace(revID), "run_scoped")
}

func LoadInputSnapshotLineage(logsRoot string) (*InputSnapshotLineage, error) {
	path := inputLineagePath(logsRoot)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lineage InputSnapshotLineage
	if err := json.Unmarshal(b, &lineage); err != nil {
		return nil, err
	}
	if lineage.BranchHeads == nil {
		lineage.BranchHeads = map[string]string{}
	}
	if lineage.Revisions == nil {
		lineage.Revisions = map[string]InputSnapshotRev{}
	}
	for revID, rev := range lineage.Revisions {
		if rev.ID == "" {
			rev.ID = revID
		}
		if rev.FileDigest == nil {
			rev.FileDigest = map[string]string{}
		}
		rev.ParentIDs = normalizeRevisionParents(rev.ParentIDs)
		lineage.Revisions[revID] = rev
	}
	return &lineage, nil
}

func (l *InputSnapshotLineage) SaveAtomic(logsRoot string) error {
	if l == nil {
		return fmt.Errorf("nil input snapshot lineage")
	}
	path := inputLineagePath(logsRoot)
	return runtime.WriteJSONAtomicFile(path, l)
}

func (l *InputSnapshotLineage) CreateRunRevision(parentRev string, digest map[string]string) string {
	parents := normalizeRevisionParents([]string{strings.TrimSpace(parentRev)})
	return l.createRevision("run", "", parents, digest)
}

func (l *InputSnapshotLineage) ForkBranch(branchKey string, baseRev string) (string, error) {
	if l == nil {
		return "", fmt.Errorf("nil input snapshot lineage")
	}
	branchKey = strings.TrimSpace(branchKey)
	if branchKey == "" {
		return "", fmt.Errorf("branch key is required")
	}
	baseRev = strings.TrimSpace(baseRev)
	if baseRev == "" {
		return "", fmt.Errorf("base revision is required")
	}
	if _, ok := l.Revisions[baseRev]; !ok {
		return "", fmt.Errorf("unknown base revision %q", baseRev)
	}
	if l.BranchHeads == nil {
		l.BranchHeads = map[string]string{}
	}
	revID := l.createRevision("branch", branchKey, []string{baseRev}, nil)
	l.BranchHeads[branchKey] = revID
	return revID, nil
}

func (l *InputSnapshotLineage) AdvanceBranch(branchKey string, parentRev string, digest map[string]string) string {
	if l == nil {
		return ""
	}
	branchKey = strings.TrimSpace(branchKey)
	if branchKey == "" {
		return ""
	}
	if l.BranchHeads == nil {
		l.BranchHeads = map[string]string{}
	}
	parentRev = strings.TrimSpace(parentRev)
	if parentRev == "" {
		parentRev = strings.TrimSpace(l.BranchHeads[branchKey])
	}
	parents := normalizeRevisionParents([]string{parentRev})
	revID := l.createRevision("branch", branchKey, parents, digest)
	l.BranchHeads[branchKey] = revID
	return revID
}

func (l *InputSnapshotLineage) MergePromotedPaths(promote []string, branchRevs map[string]string) (string, []InputSnapshotConflict, error) {
	if l == nil {
		return "", nil, fmt.Errorf("nil input snapshot lineage")
	}
	branchKeys := sortedMapKeys(branchRevs)
	if len(branchKeys) == 0 {
		return strings.TrimSpace(l.RunHead), nil, nil
	}

	patterns := normalizePromotePatterns(promote)
	if len(patterns) == 0 {
		return strings.TrimSpace(l.RunHead), nil, nil
	}

	pathToEntries := map[string]map[string]InputSnapshotConflictDigestPair{}
	for _, branchKey := range branchKeys {
		revID := strings.TrimSpace(branchRevs[branchKey])
		if revID == "" {
			continue
		}
		rev, ok := l.Revisions[revID]
		if !ok {
			return "", nil, fmt.Errorf("unknown branch revision %q for branch %q", revID, branchKey)
		}
		matched := matchPromotedPaths(patterns, rev.FileDigest)
		for _, path := range matched {
			digest := strings.TrimSpace(rev.FileDigest[path])
			if digest == "" {
				continue
			}
			if _, ok := pathToEntries[path]; !ok {
				pathToEntries[path] = map[string]InputSnapshotConflictDigestPair{}
			}
			pathToEntries[path][branchKey] = InputSnapshotConflictDigestPair{
				BranchKey:  branchKey,
				RevisionID: revID,
				Digest:     digest,
			}
		}
	}

	paths := sortedMapKeys(pathToEntries)
	merged := map[string]string{}
	conflicts := make([]InputSnapshotConflict, 0)
	for _, path := range paths {
		entriesByBranch := pathToEntries[path]
		entries := make([]InputSnapshotConflictDigestPair, 0, len(entriesByBranch))
		for _, branchKey := range sortedMapKeys(entriesByBranch) {
			entries = append(entries, entriesByBranch[branchKey])
		}
		if len(entries) == 0 {
			continue
		}
		firstDigest := entries[0].Digest
		sameDigest := true
		for _, entry := range entries[1:] {
			if entry.Digest != firstDigest {
				sameDigest = false
				break
			}
		}
		if !sameDigest {
			conflicts = append(conflicts, InputSnapshotConflict{
				Path:          path,
				BranchDigests: entries,
			})
			continue
		}
		merged[path] = firstDigest
	}

	if len(conflicts) > 0 {
		sort.Slice(conflicts, func(i, j int) bool {
			return conflicts[i].Path < conflicts[j].Path
		})
		return "", conflicts, fmt.Errorf("failure_reason=input_snapshot_conflict")
	}

	newHead := l.CreateRunRevision(l.RunHead, merged)
	l.RunHead = newHead
	return newHead, nil, nil
}

func (l *InputSnapshotLineage) createRevision(scope string, branchKey string, parentIDs []string, delta map[string]string) string {
	if l == nil {
		return ""
	}
	if l.Revisions == nil {
		l.Revisions = map[string]InputSnapshotRev{}
	}
	parents := normalizeRevisionParents(parentIDs)
	base := map[string]string{}
	for _, parentID := range parents {
		rev, ok := l.Revisions[parentID]
		if !ok {
			continue
		}
		for path, digest := range normalizeDigestMap(rev.FileDigest) {
			base[path] = digest
		}
	}
	for path, digest := range normalizeDigestMap(delta) {
		base[path] = digest
	}
	id := deterministicRevisionID(l.RunID, scope, branchKey, parents, base)
	if _, exists := l.Revisions[id]; exists {
		return id
	}
	l.Revisions[id] = InputSnapshotRev{
		ID:          id,
		ParentIDs:   parents,
		Scope:       strings.TrimSpace(scope),
		BranchKey:   strings.TrimSpace(branchKey),
		FileDigest:  base,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return id
}

func deterministicRevisionID(runID string, scope string, branchKey string, parentIDs []string, digest map[string]string) string {
	payload := struct {
		RunID     string            `json:"run_id"`
		Scope     string            `json:"scope"`
		BranchKey string            `json:"branch_key,omitempty"`
		Parents   []string          `json:"parents"`
		Digest    map[string]string `json:"digest"`
	}{
		RunID:     strings.TrimSpace(runID),
		Scope:     strings.TrimSpace(scope),
		BranchKey: strings.TrimSpace(branchKey),
		Parents:   normalizeRevisionParents(parentIDs),
		Digest:    normalizeDigestMap(digest),
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return "isr_" + hex.EncodeToString(sum[:16])
}

func normalizePromotePatterns(promote []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(promote))
	for _, raw := range promote {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		s = strings.ReplaceAll(s, "\\", "/")
		s = strings.TrimPrefix(s, "./")
		s = filepath.ToSlash(filepath.Clean(s))
		s = strings.TrimPrefix(s, "./")
		if s == "." || s == "" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func matchPromotedPaths(patterns []string, digest map[string]string) []string {
	if len(patterns) == 0 || len(digest) == 0 {
		return nil
	}
	matched := map[string]bool{}
	for path := range digest {
		normalizedPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		normalizedPath = strings.TrimPrefix(normalizedPath, "./")
		if normalizedPath == "" || normalizedPath == "." {
			continue
		}
		for _, pattern := range patterns {
			ok, err := doublestar.PathMatch(pattern, normalizedPath)
			if err != nil {
				continue
			}
			if ok {
				matched[normalizedPath] = true
				break
			}
		}
	}
	return sortedMapKeys(matched)
}

func normalizeDigestMap(digest map[string]string) map[string]string {
	if len(digest) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(digest))
	for path, rawDigest := range digest {
		normalizedPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		normalizedPath = strings.TrimPrefix(normalizedPath, "./")
		if normalizedPath == "" || normalizedPath == "." {
			continue
		}
		normalizedDigest := strings.TrimSpace(rawDigest)
		if normalizedDigest == "" {
			continue
		}
		out[normalizedPath] = normalizedDigest
	}
	return out
}

func normalizeRevisionParents(parents []string) []string {
	if len(parents) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(parents))
	for _, parent := range parents {
		s := strings.TrimSpace(parent)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
