# Kilroy Attractor Errors Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the confirmed Kilroy/Attractor reliability failures from the latest DTTF runs (resume/parallel branch naming, relative state-path handling, retry/restart over-looping, and missing terminal artifacts), with deterministic tests and end-to-end validation.

**Architecture:** Introduce a single failure-policy layer used by stage retries, loop-restart decisions, fan-in aggregation, and CLI failure handling; centralize terminal finalization so all fatal exits produce `final.json` and terminal CXDB failure events; and make resume/parallel invariants explicit by persisting/restoring branch-prefix state. Keep behavior fail-closed on unknown failure classes and ensure deterministic failures do not trigger unbounded retries/restarts.

**Tech Stack:** Go (`internal/attractor/engine`, `internal/attractor/runtime`), Git worktrees/refs, CLI adapters (`codex`, `claude`, `gemini`), CXDB artifact pipeline, Go test.

---

## Scope and Evidence

This plan addresses Kilroy/Attractor issues only (not DTTF implementation correctness).

### Confirmed Kilroy failures (from run artifacts)

1. Invalid parallel branch names on restart/resume:
   - `/home/user/code/kilroy-wt-state-isolation-watchdog/restart-120/par_tracer/parallel_results.json`
   - Example: `"/parallel/dttf-real-cxdb-20260208T070119Z/... is not a valid branch name"`

2. Relative `CODEX_HOME` broken under `-C <worktree>`:
   - `/home/user/code/kilroy-wt-state-isolation-watchdog/restart-1/verify_tracer/cli_invocation.json`
   - `/home/user/code/kilroy-wt-state-isolation-watchdog/restart-1/verify_tracer/stderr.log`
   - Error: `CODEX_HOME points to "...", but that path does not exist`

3. Retry/restart amplification on deterministic failures:
   - `/tmp/kilroy-dttf-real-cxdb-20260208T070102Z/logs/graph.dot` (loop_restart edge on `check_tracer -> par_tracer`)
   - 120 restarts with deterministic infra/config errors (from prior run analysis).

4. Terminal artifact gap:
   - `/tmp/kilroy-dttf-real-cxdb-20260208T070102Z/logs/final.json` missing on fatal path.

---

## Architectural Requirements (what “correct” means)

1. Retry and restart decisions must be class-aware (`transient_infra` vs `deterministic`) and fail-closed.
2. Stage retry and loop restart must share the same classification/policy logic.
3. Deterministic repeat loops must have a circuit breaker with explicit signature + threshold.
4. Fatal exits must always persist `final.json` with failure reason and emit terminal CXDB failure events.
5. Resume must preserve run-branch invariants (`run_branch_prefix`) so parallel refs are always valid.
6. CLI adapters must preserve actionable failure reasons and classify provider-contract failures deterministically.
7. Path-sensitive env vars for subprocess CLIs must be absolute in invocation context.
8. Fan-in all-fail outcomes must carry aggregated failure class/reason for downstream policy.
9. Preflight should fail fast for known provider CLI contract mismatches in both run and resume flows.
10. Failure-policy metadata keys (`failure_class`, `failure_signature`) must be canonical and shared across retry/restart/fan-in/CLI code paths.
11. End-to-end verification must demonstrate no infinite loop and proper terminal artifact behavior.

---

## Task 1: Add Shared Failure Policy Module (Red/Green)

**Files:**
- Create: `internal/attractor/engine/loop_restart_policy.go`
- Create: `internal/attractor/engine/loop_restart_policy_test.go`

**Step 1: Write failing tests for normalization/classification/signature**

Add tests that assert:
- `failure_class=transient_infra` remains transient.
- Unknown/empty class defaults to deterministic.
- CLI contract errors classify deterministic.
- Network/timeout errors classify transient.
- Stable signature generation for equivalent deterministic failures.

Example test skeleton:

```go
func TestClassifyFailureClass_FailClosedToDeterministic(t *testing.T) {
	out := runtime.Outcome{Status: runtime.StatusFail, FailureReason: "some unknown error"}
	if got := classifyFailureClass(out); got != failureClassDeterministic {
		t.Fatalf("class=%q want=%q", got, failureClassDeterministic)
	}
}
```

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestClassifyFailureClass|TestRestartFailureSignature' -v`

Expected: FAIL (policy behavior not implemented yet).

**Step 3: Implement minimal policy module**

Implement:
- `type failureClass string`
- constants: `failureClassTransientInfra`, `failureClassDeterministic`
- metadata-key constants: `failureMetaClass = "failure_class"` and `failureMetaSignature = "failure_signature"`
- `normalizedFailureClass(...)`
- `classifyFailureClass(out runtime.Outcome) failureClass`
- `restartFailureSignature(out runtime.Outcome) string`
- `shouldRetryOutcome(out runtime.Outcome) bool`

Policy rules:
- default/fallback = deterministic.
- prioritize explicit metadata (`out.Meta["failure_class"]`) if valid.
- classify obvious transient patterns (`timeout`, `temporary`, `connection reset`, `429`) as transient.
- classify provider-contract/arg/capability errors as deterministic.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestClassifyFailureClass|TestRestartFailureSignature' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/loop_restart_policy.go internal/attractor/engine/loop_restart_policy_test.go
git commit -m "feat(attractor): add shared failure-class policy for retry/restart decisions

Introduce normalized failure classes and stable failure signatures used by
stage retry, loop_restart, and fan-in aggregation. Default unknown failures
to deterministic to fail closed."
```

---

## Task 2: Gate Stage Retries by Failure Class

**Files:**
- Modify: `internal/attractor/engine/engine.go` (`executeWithRetry`)
- Create: `internal/attractor/engine/retry_failure_class_test.go`

**Step 1: Write failing tests for deterministic retry blocking**

Add tests for `executeWithRetry` behavior:
- Deterministic `StatusFail` should not consume additional attempts.
- Transient `StatusRetry` should still retry up to `max_retries`.

Use a counting test backend/handler to assert attempt counts.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestExecuteWithRetry_.*FailureClass' -v`

Expected: FAIL (current behavior retries both fail/retry unconditionally).

**Step 3: Implement retry gate**

In `executeWithRetry`, before sleeping/retrying:
- call `shouldRetryOutcome(out)`.
- if false, stop retries immediately and return fail outcome.

Preserve existing `allow_partial` semantics when retries are legitimately exhausted.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestExecuteWithRetry_.*FailureClass' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/retry_failure_class_test.go
git commit -m "feat(attractor): block stage retries for deterministic failures

Wire executeWithRetry to shared failure policy so deterministic failures fail
fast while transient failures continue to use retry budgets."
```

---

## Task 3: Gate `loop_restart` and Add Deterministic-Failure Circuit Breaker

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/engine/loop_restart_test.go`

**Step 1: Write failing tests**

Add tests:
- deterministic failure on a `loop_restart=true` edge does not restart.
- repeated identical transient failure signatures abort after threshold.
- threshold sourced from graph attr (e.g. `restart_signature_limit`, default `3`).

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestRun_LoopRestart.*(Deterministic|Circuit)' -v`

Expected: FAIL.

**Step 3: Implement loop-restart policy**

In run loop / `loopRestart` path:
- classify failure from current outcome.
- allow restart only for `transient_infra`.
- compute signature via `restartFailureSignature`.
- track signature counts on `Engine` state and abort when threshold reached.

Failure message format:
- include class, signature, count, threshold, and node id.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestRun_LoopRestart.*(Deterministic|Circuit)' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/loop_restart_test.go
git commit -m "feat(attractor): class-gate loop_restart and add deterministic circuit breaker

Prevent loop_restart on deterministic failures and stop repeated identical
failure signatures early with explicit terminal reason."
```

---

## Task 4: Always Write Terminal `final.json` on Fatal Paths

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/runtime/final.go`
- Modify: `internal/attractor/runtime/final_test.go`
- Create: `internal/attractor/engine/finalization_fatal_paths_test.go`

**Step 1: Write failing tests**

Add tests asserting:
- `final.json` exists when run fails before terminal exit node.
- `final.json` exists when loop_restart limit/circuit-breaker aborts.
- `final.json` contains non-empty `failure_reason` for failure cases.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/runtime ./internal/attractor/engine -run 'Test.*(Final|Fatal|LoopRestartLimit)' -v`

Expected: FAIL (fatal paths currently return errors without guaranteed final artifact; `FinalOutcome` currently has no `failure_reason` field).

**Step 3: Implement centralized finalization**

Changes:
- add `FailureReason string` to `runtime.FinalOutcome`.
- introduce engine helper (single write path) for:
  - `final.json`
  - terminal CXDB turn (`RunCompleted` / `RunFailed`)
  - optional CXDB artifact upload
  - run tarball generation
- replace duplicated success/fail finalization code with helper calls.
- ensure all fatal exits in `run`/`runLoop` invoke failure finalization before returning error.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/runtime ./internal/attractor/engine -run 'Test.*(Final|Fatal|LoopRestartLimit)' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/runtime/final.go internal/attractor/runtime/final_test.go internal/attractor/engine/engine.go internal/attractor/engine/finalization_fatal_paths_test.go
git commit -m "feat(attractor): guarantee final.json on fatal exits with failure reason

Centralize terminal artifact writing so success and all fatal paths persist
final.json (including loop_restart aborts) and include terminal failure_reason."
```

---

## Task 5: Fix Resume Branch-Prefix Invariants (parallel refs)

**Files:**
- Modify: `internal/attractor/engine/engine.go` (`writeManifest`)
- Modify: `internal/attractor/engine/resume.go`
- Modify: `internal/attractor/engine/parallel_handlers.go` (defensive fallback)
- Create: `internal/attractor/engine/resume_parallel_branch_prefix_test.go`

**Step 1: Write failing test**

Create a resume scenario with parallel node after restart and assert branch names:
- start with `attractor/run/<run_id>`
- parallel branches must be `attractor/run/parallel/<run_id>/...`, never `/parallel/...`.

**Step 2: Run test to verify red**

Run: `go test ./internal/attractor/engine -run 'TestResume_ParallelBranchPrefixPreserved' -v`

Expected: FAIL (resume currently sets `RunOptions` without branch prefix).

**Step 3: Implement invariant preservation**

Changes:
- persist `run_branch_prefix` in manifest.
- in resume, set `Engine.Options.RunBranchPrefix` from:
  1. manifest `run_branch_prefix`
  2. `run_config.json` `git.run_branch_prefix`
  3. derive from `run_branch`
  4. default `attractor/run`
- defensive fallback in parallel branch builder if prefix is empty.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestResume_ParallelBranchPrefixPreserved' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/resume.go internal/attractor/engine/parallel_handlers.go internal/attractor/engine/resume_parallel_branch_prefix_test.go
git commit -m "fix(attractor): preserve run_branch_prefix across resume and parallel fan-out

Store and restore run branch prefix so resumed parallel branches always produce
valid git refs and never default to /parallel/... paths."
```

---

## Task 6: Normalize Relative CLI State Paths (`CODEX_HOME`, etc.)

**Files:**
- Modify: `internal/attractor/engine/codergen_router.go`
- Create: `internal/attractor/engine/codergen_cli_env_paths_test.go`

**Step 1: Write failing test**

Add test with fake CLI that prints env vars:
- set `CODEX_HOME` to relative path.
- execute with `cmd.Dir` pointing to worktree.
- assert subprocess receives absolute `CODEX_HOME` resolved from Kilroy launch cwd.

**Step 2: Run test to verify red**

Run: `go test ./internal/attractor/engine -run 'TestRunCLI_RelativeStateEnvBecomesAbsolute' -v`

Expected: FAIL.

**Step 3: Implement path normalization**

In CLI runner:
- build explicit env list from `os.Environ()`.
- for known path vars (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `GEMINI_CONFIG_DIR`), if value is relative, convert to absolute before command run.
- record normalized paths in `cli_invocation.json` for observability.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestRunCLI_RelativeStateEnvBecomesAbsolute' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/codergen_router.go internal/attractor/engine/codergen_cli_env_paths_test.go
git commit -m "fix(attractor): absolutize relative CLI state env paths before subprocess launch

Prevent CODEX_HOME-style path breakage when running providers with -C worktree by
normalizing relative state paths to absolute values."
```

---

## Task 7: Provider CLI Contract Hardening (`claude --verbose`) + Preflight

**Files:**
- Create: `internal/attractor/engine/provider_cli_preflight.go`
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/run_with_config.go`
- Modify: `internal/attractor/engine/resume.go`
- Modify: `internal/attractor/engine/codergen_cli_invocation_test.go`
- Create: `internal/attractor/engine/run_with_config_preflight_test.go`
- Create: `internal/attractor/engine/resume_preflight_test.go`

**Step 1: Write failing tests**

Add tests:
- Anthropic CLI args include `--verbose` only when capability probe indicates support for that flag.
- RunWithConfig preflight fails fast when configured provider CLI lacks required flags/capabilities.
- Resume preflight fails fast when resumed run config includes provider CLI contracts that are no longer satisfiable.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestDefaultCLIInvocation_Anthropic|TestRunWithConfig_Preflight|TestResume_Preflight' -v`

Expected: FAIL.

**Step 3: Implement CLI contract hardening**

Changes:
- update Anthropic default invocation to include `--verbose` only when capability-probe says supported; otherwise omit and emit a warning.
- add shared provider CLI preflight helper and call it in:
  - `RunWithConfig` after graph parse + catalog resolution and before CXDB startup
  - `resumeFromLogsRoot` after run-config/model-catalog load and before backend creation
- preflight checks:
  - verify executable exists.
  - probe capabilities minimally (flag presence or probe command behavior).
  - return deterministic error with actionable message.

Keep probes lightweight and deterministic; no network required.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestDefaultCLIInvocation_Anthropic|TestRunWithConfig_Preflight|TestResume_Preflight' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/provider_cli_preflight.go internal/attractor/engine/codergen_router.go internal/attractor/engine/run_with_config.go internal/attractor/engine/resume.go internal/attractor/engine/codergen_cli_invocation_test.go internal/attractor/engine/run_with_config_preflight_test.go internal/attractor/engine/resume_preflight_test.go
git commit -m "feat(attractor): harden provider CLI contracts with anthropic verbose flag and run preflight

Require known CLI capabilities up front and include --verbose for anthropic
stream-json output to avoid deterministic runtime contract failures."
```

---

## Task 8: Preserve CLI Failure Semantics and Classification

**Files:**
- Modify: `internal/attractor/engine/codergen_router.go`
- Create: `internal/attractor/engine/codergen_cli_failure_class_test.go`

**Step 1: Write failing tests**

Add tests that run fake provider CLI failures and assert:
- failure reason includes provider + stderr classification detail (not just `exit status 1`).
- outcome metadata includes canonical keys `failure_class` and `failure_signature` for policy consumption.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestRunCLI_FailureClassification' -v`

Expected: FAIL.

**Step 3: Implement classification bridge**

In `runCLI` error paths:
- parse stderr/stdout for known contract/transient indicators.
- populate `runtime.Outcome.Meta` using shared constants (`failureMetaClass`, `failureMetaSignature`).
- enrich `FailureReason` with concise provider-specific detail.

Do not classify internal retry-able schema fallback failures as terminal until fallback exhausted.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestRunCLI_FailureClassification' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/codergen_router.go internal/attractor/engine/codergen_cli_failure_class_test.go
git commit -m "feat(attractor): classify provider CLI failures and preserve actionable failure reasons

Emit failure_class metadata and detailed failure_reason from CLI stderr patterns
so retry/restart policy can correctly differentiate deterministic vs transient errors."
```

---

## Task 9: Fan-In All-Fail Aggregation Must Propagate Failure Class

**Files:**
- Modify: `internal/attractor/engine/parallel_handlers.go`
- Create: `internal/attractor/engine/parallel_fanin_failure_class_test.go`

**Step 1: Write failing tests**

Add tests for all-fail fan-in:
- aggregate class deterministic when any deterministic branch exists.
- aggregate class transient only when all branches are transient.
- returned outcome includes class + signature metadata using canonical keys.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestFanIn_AllFail_AggregatesFailureClass' -v`

Expected: FAIL.

**Step 3: Implement aggregation**

In `FanInHandler.Execute` all-failed path:
- inspect branch outcomes with `classifyFailureClass`.
- build aggregate reason and stable signature.
- return `StatusFail` with populated metadata for downstream loop-restart gate using canonical keys.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestFanIn_AllFail_AggregatesFailureClass' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/parallel_handlers.go internal/attractor/engine/parallel_fanin_failure_class_test.go
git commit -m "feat(attractor): propagate aggregated failure class for fan-in all-fail outcomes

Make parallel fan-in failures policy-aware by attaching deterministic/transient
classification and signatures to the returned fail outcome."
```

---

## Task 10: End-to-End Guardrail Tests for Combined Policy Flow

**Files:**
- Create: `internal/attractor/engine/reliability_guardrail_integration_test.go`
- Modify: `internal/attractor/engine/run_with_config_integration_test.go` (if shared helpers are needed)

**Step 1: Write failing integration tests**

Add end-to-end tests proving combined invariants:
- deterministic CLI failure -> no stage retry -> no loop_restart -> terminal `final.json` with `failure_reason`.
- transient CLI failure repeating same signature -> circuit breaker triggers before max restarts.
- resume after checkpoint preserves branch-prefix and parallel branch refs.

**Step 2: Run tests to verify red**

Run: `go test ./internal/attractor/engine -run 'TestReliabilityGuardrail_' -v`

Expected: FAIL.

**Step 3: Implement minimal wiring fixes discovered by integration tests**

Apply only the minimal code changes still required after Tasks 1-9.

**Step 4: Run tests to verify green**

Run: `go test ./internal/attractor/engine -run 'TestReliabilityGuardrail_' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/reliability_guardrail_integration_test.go
git add internal/attractor/engine/run_with_config_integration_test.go
# Add only explicitly named implementation files touched by Task 10 fixes (no wildcards).
git add internal/attractor/engine/engine.go internal/attractor/engine/parallel_handlers.go internal/attractor/engine/codergen_router.go internal/attractor/engine/run_with_config.go internal/attractor/engine/resume.go internal/attractor/engine/provider_cli_preflight.go
git commit -m "test(attractor): add integration guardrails for retry/restart/finalization invariants

Cover full-path reliability behavior: deterministic fail-fast, transient circuit
breaking, terminal artifact persistence, and resume branch-prefix safety."
```

---

## Task 11: Documentation + Runbook Updates (Operator-Safe)

**Files:**
- Modify: `docs/strongdm/attractor/README.md`
- Create: `docs/strongdm/attractor/reliability-troubleshooting.md`

**Step 1: Write docs changes**

Document:
- failure-class semantics and defaults.
- loop-restart gate + circuit-breaker behavior.
- guaranteed `final.json` semantics on fatal exits.
- where to inspect signature/class in logs.
- provider CLI preflight expectations.

**Step 2: Verify docs references**

Run: `rg -n 'failure_class|loop_restart|final.json|circuit' docs/strongdm/attractor`

Expected: updated references present and consistent.

**Step 3: Commit**

```bash
git add docs/strongdm/attractor/README.md docs/strongdm/attractor/reliability-troubleshooting.md
git commit -m "docs(attractor): document reliability guardrails and fatal-path observability

Add a troubleshooting guide covering failure classes, restart gating, circuit
breakers, and terminal artifact guarantees."
```

---

## Task 12: Full Verification and Real DTTF Validation Run

**Files:**
- No source changes expected (validation + evidence only)

**Step 1: Run full attractor engine tests**

Run: `go test ./internal/attractor/engine/... -count=1`

Expected: PASS.

**Step 2: Run full attractor runtime/modeldb/cxdb-adjacent tests**

Run:
- `go test ./internal/attractor/runtime/... -count=1`
- `go test ./internal/attractor/modeldb/... -count=1`
- `go test ./internal/cxdb/... -count=1`

Expected: PASS.

**Step 3: Execute real DTTF resume validation from existing logs root (detached)**

Use detached launch to avoid parent-session teardown:

```bash
# Resume requires an existing logs root with manifest/checkpoint artifacts.
SOURCE_LOGS_ROOT=/tmp/kilroy-dttf-real-cxdb-20260208T070102Z/logs
RUN_ROOT=/tmp/kilroy-dttf-real-cxdb-$(date -u +%Y%m%dT%H%M%SZ)-postfix-guardrail
mkdir -p "$RUN_ROOT"
export SOURCE_LOGS_ROOT RUN_ROOT
setsid -f bash -lc 'cd /home/user/code/kilroy-wt-state-isolation-watchdog && ./kilroy attractor resume --logs-root "$SOURCE_LOGS_ROOT" > "$RUN_ROOT/resume.out" 2>&1'
```

Monitor:
- `tail -f "$RUN_ROOT/resume.out"`
- `rg -n 'loop_restart|failure_class|failure_signature|final.json' "$SOURCE_LOGS_ROOT" -S`

Acceptance criteria:
- no `/parallel/...` invalid branch names.
- no relative `CODEX_HOME` path errors.
- deterministic failures do not spin for dozens of restarts.
- terminal `final.json` always exists on termination (`$SOURCE_LOGS_ROOT/final.json` or latest `restart-*/final.json` when restarts occur).

**Step 4: Archive verification evidence**

Capture:
- test outputs
- run root path
- terminal `final.json`
- first fatal signature/class event (if failed)

**Step 5: Commit any last-mile fixes if needed**

If verification required code changes, create focused follow-up commits (no `git add -A`).

---

## Rollback / Risk Controls

1. Keep each task commit isolated and revertible.
2. If Task 7 preflight causes environment-specific breakage, guard with config flag default-on and test both paths.
3. Keep failure-class taxonomy minimal in v1 (`transient_infra`, `deterministic`) but preserve raw reason text for future expansion.
4. Do not change dotfile semantics in this plan; only engine reliability behavior.

---

## Final Verification Checklist

1. `go test` passes for all touched packages.
2. Deterministic failure path executes exactly one attempt for class-gated nodes.
3. Loop restart requires transient class and obeys signature circuit breaker.
4. Resume preserves run-branch prefix and parallel refs remain valid.
5. `final.json` exists and includes `failure_reason` for all fatal exits.
6. DTTF real run demonstrates guardrails and terminal artifact guarantees.
