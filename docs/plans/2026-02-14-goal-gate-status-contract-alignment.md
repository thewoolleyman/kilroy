# Goal-Gate Status Contract Alignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the deterministic goal-gate contract mismatch caused by `outcome=pass` in both prompt instructions and exit-edge conditions, and add blocking validation guardrails so future graphs cannot reintroduce this failure mode.

**Architecture:** Keep runtime semantics unchanged and spec-canonical: goal gates are satisfied only by `success|partial_success`. Fix authoring sources (`reference_template.dot` and `SKILL.md`) and add validator enforcement as an ERROR for incompatible goal-gate routing conditions. Add a secondary best-effort warning for prompt text that instructs non-success goal-gate outcomes (for example `outcome=pass`) to catch drift early.

**Tech Stack:** Go (`internal/attractor/validate`), DOT templates (`skills/english-to-dotfile/reference_template.dot`), markdown skill/docs (`skills/english-to-dotfile/SKILL.md`), Go tests (`go test`).

---

### Task 1: Lock in Existing Contract With a Focused Regression Test

**Files:**
- Modify: `internal/attractor/validate/validate_test.go`

**Step 1: Add failing test cases for the double mismatch**

Add a new test that builds a small DOT graph where:
- node `review_consensus` has `goal_gate=true`
- edge `review_consensus -> exit` uses `condition="outcome=pass"`
- prompt text on `review_consensus` includes outcome instructions using `outcome=pass`

Assert validator emits:
- an ERROR diagnostic for invalid goal-gate outcome routing condition
- a WARNING diagnostic for prompt text drift (best-effort heuristic)

**Step 2: Run targeted test to verify failure before implementation**

Run:
```bash
go test ./internal/attractor/validate -run GoalGate -v
```

Expected: FAIL because the new rule does not exist yet.

**Step 3: Commit test scaffold**

Run:
```bash
git add internal/attractor/validate/validate_test.go
git commit -m "test(validate): add regression for goal-gate exit condition using non-success status"
```

---

### Task 2: Add Validator Rule for Goal-Gate Exit Status Compatibility

**Files:**
- Modify: `internal/attractor/validate/validate.go`
- Modify: `internal/attractor/validate/validate_test.go`

**Step 1: Implement goal-gate status contract lints in validator**

Add a primary lint function (ERROR severity) that:
- finds nodes with `goal_gate=true`
- inspects outgoing edges from those nodes
- parses/evaluates each edge condition expression syntactically (reusing current condition parsing conventions)
- raises an ERROR when an edge condition explicitly routes on a non-success outcome value as the success path to termination (for example `outcome=pass`)
- uses the validator's existing terminal-node identification logic (same behavior as `findExitNodeID` / exit heuristics) instead of duplicating a new terminal detector

Recommended rule id: `goal_gate_exit_status_contract`.

Recommended message:
- "goal_gate node routes to terminal on non-success outcome; use outcome=success (or partial_success) to satisfy goal-gate contract"

Add a secondary lint function (WARNING severity) for prompt drift:
- if `goal_gate=true` prompt/llm_prompt text contains explicit non-success outcome instruction (for example `outcome=pass`)
- emit a warning with a fix hint to use `outcome=success` (or `partial_success`) for gate satisfaction

**Step 2: Register the rule in `Validate()`**

Wire new lints into `Validate()` after `lintGoalGateHasRetry`:
- ERROR rule first (`goal_gate_exit_status_contract`)
- WARNING prompt-drift rule second (`goal_gate_prompt_status_hint`)

**Step 3: Add/expand tests**

In `validate_test.go`, cover:
- ERROR emitted for goal-gate edge `condition="outcome=pass"` on path to exit
- no ERROR for `condition="outcome=success"`
- no ERROR for `condition="outcome=partial_success"`
- WARNING emitted when goal-gate prompt contains `outcome=pass`
- no WARNING when goal-gate prompt uses canonical outcomes
- no new diagnostics when goal-gate node routes without terminal-exit mismatch

**Step 4: Run validate package tests**

Run:
```bash
go test ./internal/attractor/validate -v
```

Expected: PASS.

**Step 5: Commit validator implementation**

Run:
```bash
git add internal/attractor/validate/validate.go internal/attractor/validate/validate_test.go
git commit -m "feat(validate): enforce goal-gate terminal routing status contract as error"
```

---

### Task 3: Fix Reference Template to Use Canonical Goal-Gate Success Status

**Files:**
- Modify: `skills/english-to-dotfile/reference_template.dot`

**Step 1: Update review prompts to canonical status vocabulary (atomic with edge changes)**

Change review-related status instructions from `outcome=pass` to `outcome=success` where the branch means "approved".

At minimum update:
- `review_a`, `review_b`, `review_c` prompt outcome instructions
- `review_consensus` prompt outcome instructions

**Step 2: Update consensus routing edge**

Change:
- `review_consensus -> exit [condition="outcome=pass"]`

to:
- `review_consensus -> exit [condition="outcome=success"]`

Retain retry/failure path semantics.

Note: this prompt+edge change is atomic. Changing only one side still leaves deterministic misrouting.

**Step 3: Validate the template graph**

Run one of:
```bash
./kilroy attractor validate --graph skills/english-to-dotfile/reference_template.dot
```
or
```bash
go run ./cmd/kilroy attractor validate --graph skills/english-to-dotfile/reference_template.dot
```

Expected: no errors; no warning from new goal-gate status rule.

**Step 4: Commit template fix**

Run:
```bash
git add skills/english-to-dotfile/reference_template.dot
git commit -m "fix(template): align goal-gate success routing with canonical outcome statuses"
```

---

### Task 4: Align Skill Guidance to Prevent Future Drift

**Files:**
- Modify: `skills/english-to-dotfile/SKILL.md`

**Step 1: Update routing guidance where it currently normalizes `pass`**

Adjust sections that imply `pass` is standard in the core loop. Make explicit:
- custom outcomes are allowed for steering nodes
- goal-gate satisfaction must use `outcome=success` or `outcome=partial_success`
- explicitly correct `skills/english-to-dotfile/SKILL.md` Reference Looping Profile line with `outcome=pass|retry|fail|skip|done` to canonical goal-gate-safe wording

**Step 2: Add explicit anti-pattern entry**

Add a concise anti-pattern similar to:
- "Do not use `outcome=pass` as success signal on `goal_gate=true` nodes that route to terminal; use canonical success statuses."

**Step 3: Keep examples consistent with updated template**

Ensure all review/consensus examples in skill prose reflect `success/retry/fail` status routing for goal-gate flow.
Ensure canonical outcomes section and looping profile are no longer contradictory.

**Step 4: Commit skill-doc changes**

Run:
```bash
git add skills/english-to-dotfile/SKILL.md
git commit -m "docs(skill): codify canonical goal-gate success statuses and remove pass-based drift"
```

---

### Task 5: End-to-End Validation of Behavior and Tooling

**Files:**
- No new files required

**Step 1: Run targeted engine and validator tests**

Run:
```bash
go test ./internal/attractor/validate -run GoalGate -v
go test ./internal/attractor/engine -run GoalGate -v
```

Expected: PASS.

**Step 2: Run full repository test suite**

Run:
```bash
go test ./...
```

Expected: PASS.

**Step 3: Build CLI and validate template again**

Run:
```bash
go build -o ./kilroy ./cmd/kilroy
./kilroy attractor validate --graph skills/english-to-dotfile/reference_template.dot
```

Expected: successful build and clean validation.

**Step 4: Commit final verification note (if any test fixtures/log snapshots changed)**

Run:
```bash
git add <explicit-file-list>
git commit -m "chore: run full validation for goal-gate status contract alignment"
```

Only commit if there are intentional tracked changes.

---

### Task 6: PR Assembly and Reviewer Checklist

**Files:**
- No source changes required

**Step 1: Produce concise change summary for PR description**

Include:
- root cause (double mismatch: prompt instructs `pass`, edge routes on `pass`, goal_gate requires `success|partial_success`)
- why engine semantics were kept unchanged
- why lint enforcement is ERROR (fatal) for deterministic contract violations
- validator guardrails added (ERROR + prompt-drift WARNING)
- template + skill/docs aligned

**Step 2: Include reviewer verification commands**

```bash
go test ./internal/attractor/validate -v
go test ./internal/attractor/engine -run GoalGate -v
./kilroy attractor validate --graph skills/english-to-dotfile/reference_template.dot
```

**Step 3: Ensure commit history is narrow and ordered**

Recommended commit order:
1. test regression
2. validator rule
3. template alignment
4. skill/docs alignment
5. verification/meta (if needed)
