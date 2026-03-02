# Proposal: Run-Scoped `.ai` Workspace with Spec-Aligned Materialization

## Problem

The March 2, 2026 incident (`run_id=01KJPDK649C65Y07TBX1041C73`) exposed two
issues:

1. **Immediate failure mode (confirmed):** stale local `./kilroy` binary
   (`cee6fe8e...`) used old `copyInputFile` behavior that truncated same-path
   copies to zero bytes.
2. **Input boundary gap:** startup materialization implicitly copied gitignored
   repo-local `.ai/*.md` from the developer checkout, which introduced stale
   Solitaire content into the run.

## Evidence Anchors

1. Incident run id: `01KJPDK649C65Y07TBX1041C73` (March 2, 2026).
2. Binary mismatch from investigation:
   - run base SHA: `45b3956c...`
   - local build SHA: `cee6fe8e...` (stale)
3. Solitaire provenance:
   - source: `/home/user/code/kilroy/.ai/spec.md` (gitignored local scratch)
   - copied into startup snapshot/worktree
   - SHA256 match:
     `1f94094f76eeb687e0aabd4eca1c646edf8952ca7bc86ff1048f2f95604c3b5d`
4. Stale-binary truncation produced repeated 0-byte `.ai` files at stage start.

## Normative Alignment (Required)

This proposal preserves the Attractor spec contracts:

1. **Canonical run/stage artifacts remain under `logs_root`.**
   - `checkpoint.json`, run `manifest.json`, stage `status.json`, `prompt.md`,
     `response.md`, artifacts directory.
2. **Input materialization C.1 semantics are preserved verbatim.**
   - transitive closure when `follow_references=true`
   - `include` fail-on-unmatched with deterministic
     `failure_reason=input_include_missing`
   - `default_include` best-effort
   - `infer_with_llm` additive with scanner-only fallback on inferer failure
3. **Manifest coverage remains required at all levels.**
   - run-level: `logs_root/inputs_manifest.json`
   - branch-level: `<branch_logs_root>/inputs_manifest.json`
   - stage-level: `logs_root/<node_id>/inputs_manifest.json`
4. **`KILROY_INPUTS_MANIFEST_PATH` remains the required runtime contract.**
   - no new required env var in this proposal.

## Decision

Adopt this runtime boundary model:

1. `.ai` in the worktree is **stage-visible run workspace state only**, scoped
   to `./.ai/runs/<run_id>/...`.
2. Shared project inputs remain outside `.ai` (for example `docs/`, `specs/`,
   `policies/`), tracked and reviewed in normal Git workflows.
3. Repo-level `.ai/*` ingestion is disabled by default to prevent accidental
   import of local scratch files.
4. Hydration for linear, parallel, and resume paths must come from persisted
   snapshot/manifest state under `logs_root`, not mutable developer workspace
   state.

## Why This Is Correct For Worktrees

When a new worktree is recreated from `HEAD`, untracked `.ai/*` content is
absent by definition. Therefore Git cannot be the durability path for `.ai`.

Durability comes from persisted materialization state:

1. Persisted snapshots/manifests under `logs_root`.
2. Deterministic hydration before stage execution (including branch and resume).
3. Stage-local manifest path contract via `KILROY_INPUTS_MANIFEST_PATH`.

This keeps code checkout isolation (worktrees) separate from run-state
durability (materialization).

## Proposed Design

### 1. Input Declaration (Spec-Preserving)

- Keep existing `inputs.materialize` controls and semantics.
- Introduce an explicit import declaration that compiles into existing
  `include/default_include` behavior rather than replacing it.
- Default posture:
  - no implicit repo-root `.ai/*` ingestion
  - no broad globs that sweep local gitignored scratch files

### 2. Canonical Storage Locations

1. Startup canonical input snapshot: `logs_root/input_snapshot/files/...`
2. Run-level input manifest: `logs_root/inputs_manifest.json`
3. Branch-level input manifest:
   `<branch_logs_root>/inputs_manifest.json`
4. Stage-level input manifest:
   `logs_root/<node_id>/inputs_manifest.json`
5. Stage workspace files visible to handlers/agents:
   `./.ai/runs/<run_id>/...`

### 3. Snapshot Evolution (Explicit Normative Addition)

The startup snapshot remains canonical. This proposal adds revisioned mutation
semantics for run-scoped workspace persistence:

1. Create snapshot revision `0` at run startup.
2. After each node completion, persist eligible run-scoped files
   (`./.ai/runs/<run_id>/...`) into the canonical snapshot storage and bump
   revision `N -> N+1`.
3. Record `snapshot_revision` in run/branch/stage input manifests.
4. Branch hydration and resume hydrate from latest committed snapshot revision.
5. Snapshot-update failures are deterministic and recorded in checkpoint/progress
   artifacts (no silent fallback to mutable workspace state).

### 4. Read/Write Contract

- Nodes read required inputs from hydrated run-scoped workspace paths.
- Nodes write scratch/outputs to `./.ai/runs/<run_id>/...`.
- No requirement to git-track `.ai` files.
- If output should become shared repository truth, use explicit publish/promotion
  flow, not implicit runtime leakage.

## Why Shared Project Sources Still Matter

Run-scoped execution does not remove the need for shared source-of-truth files.
Shared files are still needed for:

1. reviewed canonical requirements/specs
2. persistent policy/rubric files
3. stable baselines reused across many runs
4. durable governance/audit history in Git

Correct split:

- authoritative sources: repo paths outside `.ai`
- runtime working set: hydrated per-run workspace + persisted snapshot state

## Implementation Plan

1. **Boundary guardrail**
   - disable implicit repo `.ai/*.md` ingestion by default
   - keep explicit opt-in paths only
2. **Spec-aligned import mapping**
   - map explicit imports into existing `include/default_include`
   - preserve C.1 include/default/include/closure/inference semantics
3. **Revisioned snapshot manager**
   - add snapshot revision metadata
   - persist revision id into run/branch/stage manifests
4. **Hydration parity**
   - hydrate branch and resume from persisted snapshot revisions
   - never read mutable source workspace as authoritative after startup
5. **Diagnostics**
   - record binary revision and snapshot revision transitions in run artifacts
   - deterministic failure reasons for include/snapshot update failures
6. **Migration**
   - update templates/docs to use `./.ai/runs/<run_id>/...`
   - provide compatibility notes for older graphs referencing `.ai/*.md`

## Test Plan

1. Canonical run/stage outputs remain under `logs_root` per spec.
2. `KILROY_INPUTS_MANIFEST_PATH` is present and points to stage-local manifest.
3. `include` unmatched => deterministic `failure_reason=input_include_missing`.
4. `default_include` unmatched does not fail run.
5. `follow_references=true` computes transitive closure.
6. `infer_with_llm=true` inferer failure falls back to scanner-only closure with
   warnings.
7. Startup materialization does not ingest repo `.ai/*` unless explicitly
   included.
8. Run/branch/stage manifests all exist and carry consistent
   `snapshot_revision`.
9. Parallel branch worktrees hydrate latest persisted revision before stage exec.
10. Resume after worktree recreation hydrates from persisted snapshot/manifest
    state (not mutable source workspace).
11. Same-file copy path cannot truncate content.

## Reviewer Checklist

1. Does proposal preserve `logs_root` as canonical run/stage artifact location?
2. Does it keep all Appendix C.1 materialization semantics intact?
3. Does it retain required run/branch/stage manifest coverage?
4. Does it avoid introducing a new required runtime env var?
5. Does it define explicit, deterministic snapshot evolution semantics?
6. Does it prevent implicit repo `.ai/*` ingestion by default?

## Incident-Specific Cleanup (Completed)

- Removed stale local scratch file:
  `/home/user/code/kilroy/.ai/spec.md`
