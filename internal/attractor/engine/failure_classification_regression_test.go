package engine

// failure_classification_regression_test.go
//
// Pins every heuristic string pattern used by classifyFailureClass so that
// model/provider upgrades that change error message wording cause an explicit
// test failure rather than a silent flip between deterministic and transient.
//
// Design intent:
//   - Each entry in the table is ONE sample error string that exercises a
//     specific pattern in transientInfraReasonHints, budgetExhaustedReasonHints,
//     structuralReasonHints, or the inline canceled check.
//   - The "unknown / unrecognised" case explicitly documents the fall-through
//     default: unknown error strings classify as deterministic (fail-closed).
//   - normalizedFailureClass aliases are covered in a separate sub-table so
//     that alias drift is also caught.

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestClassifyFailureClass_AllHeuristicPatterns(t *testing.T) {
	// Each entry: an FailureReason string that should trigger a specific class.
	// No explicit failure_class hint is set, so the heuristic path is exercised.
	cases := []struct {
		name          string
		failureReason string
		want          string
	}{
		// ── transient_infra ─────────────────────────────────────────────────────
		{name: "transient: timeout", failureReason: "request timeout after 30s", want: failureClassTransientInfra},
		{name: "transient: timed out", failureReason: "operation timed out", want: failureClassTransientInfra},
		{name: "transient: context deadline exceeded", failureReason: "context deadline exceeded", want: failureClassTransientInfra},
		{name: "transient: connection refused", failureReason: "dial tcp: connection refused", want: failureClassTransientInfra},
		{name: "transient: connection reset", failureReason: "read: connection reset by peer", want: failureClassTransientInfra},
		{name: "transient: could not resolve host", failureReason: "curl: (6) could not resolve host: api.openai.com", want: failureClassTransientInfra},
		{name: "transient: could not resolve hostname", failureReason: "ssh: could not resolve hostname example.com", want: failureClassTransientInfra},
		{name: "transient: temporary failure in name resolution", failureReason: "getaddrinfo: temporary failure in name resolution", want: failureClassTransientInfra},
		{name: "transient: network is unreachable", failureReason: "network is unreachable", want: failureClassTransientInfra},
		{name: "transient: broken pipe", failureReason: "write: broken pipe", want: failureClassTransientInfra},
		{name: "transient: tls handshake timeout", failureReason: "tls handshake timeout", want: failureClassTransientInfra},
		{name: "transient: i/o timeout", failureReason: "read tcp: i/o timeout", want: failureClassTransientInfra},
		{name: "transient: no route to host", failureReason: "connect: no route to host", want: failureClassTransientInfra},
		{name: "transient: temporary failure", failureReason: "temporary failure, please retry", want: failureClassTransientInfra},
		{name: "transient: temporarily unavailable", failureReason: "service temporarily unavailable", want: failureClassTransientInfra},
		{name: "transient: try again", failureReason: "resource locked, try again later", want: failureClassTransientInfra},
		{name: "transient: rate limit", failureReason: "rate limit exceeded for model", want: failureClassTransientInfra},
		{name: "transient: too many requests", failureReason: "HTTP 429: too many requests", want: failureClassTransientInfra},
		{name: "transient: service unavailable", failureReason: "503 service unavailable", want: failureClassTransientInfra},
		{name: "transient: gateway timeout", failureReason: "504 gateway timeout", want: failureClassTransientInfra},
		{name: "transient: econnrefused", failureReason: "dial tcp 127.0.0.1:8080: econnrefused", want: failureClassTransientInfra},
		{name: "transient: econnreset", failureReason: "read tcp: econnreset", want: failureClassTransientInfra},
		{name: "transient: dial tcp", failureReason: "dial tcp: lookup example.com", want: failureClassTransientInfra},
		{name: "transient: transport is closing", failureReason: "rpc error: transport is closing", want: failureClassTransientInfra},
		{name: "transient: stream disconnected", failureReason: "stream disconnected unexpectedly", want: failureClassTransientInfra},
		{name: "transient: stream closed before", failureReason: "stream closed before response was complete", want: failureClassTransientInfra},
		{name: "transient: index.crates.io", failureReason: "failed to fetch from https://index.crates.io/config.json", want: failureClassTransientInfra},
		{name: "transient: download of config.json failed", failureReason: "download of config.json failed: network error", want: failureClassTransientInfra},
		{name: "transient: toolchain_or_dependency_registry_unavailable (in reason)", failureReason: "toolchain_or_dependency_registry_unavailable: cargo registry unreachable", want: failureClassTransientInfra},
		{name: "transient: toolchain dependency resolution blocked by network", failureReason: "toolchain dependency resolution blocked by network outage", want: failureClassTransientInfra},
		{name: "transient: toolchain_workspace_io (in reason)", failureReason: "toolchain_workspace_io: could not write to /tmp", want: failureClassTransientInfra},
		{name: "transient: cross-device link", failureReason: "failed to move artifact: cross-device link", want: failureClassTransientInfra},
		{name: "transient: invalid cross-device link", failureReason: "rename /a /b: invalid cross-device link", want: failureClassTransientInfra},
		{name: "transient: os error 18", failureReason: "os error 18 encountered during copy", want: failureClassTransientInfra},
		{name: "transient: 502 in reason", failureReason: "upstream returned 502", want: failureClassTransientInfra},
		{name: "transient: 503 in reason", failureReason: "upstream returned 503", want: failureClassTransientInfra},
		{name: "transient: 504 in reason", failureReason: "upstream returned 504", want: failureClassTransientInfra},
		{name: "transient: net::ERR_INTERNET_DISCONNECTED", failureReason: "page.goto failed: net::ERR_INTERNET_DISCONNECTED", want: failureClassTransientInfra},

		// ── canceled ─────────────────────────────────────────────────────────────
		{name: "canceled: canceled spelling", failureReason: "run was canceled by operator", want: failureClassCanceled},
		{name: "canceled: cancelled spelling", failureReason: "job was cancelled", want: failureClassCanceled},

		// ── budget_exhausted ──────────────────────────────────────────────────────
		{name: "budget: turn limit", failureReason: "exceeded turn limit for session", want: failureClassBudgetExhausted},
		{name: "budget: max_turns", failureReason: "max_turns=60 reached", want: failureClassBudgetExhausted},
		{name: "budget: max turns (space)", failureReason: "max turns reached for this run", want: failureClassBudgetExhausted},
		{name: "budget: token limit reached", failureReason: "token limit reached", want: failureClassBudgetExhausted},
		{name: "budget: token limit exceeded", failureReason: "token limit exceeded for session", want: failureClassBudgetExhausted},
		{name: "budget: max tokens", failureReason: "max tokens allowed exceeded", want: failureClassBudgetExhausted},
		{name: "budget: max_tokens", failureReason: "max_tokens=8192 exceeded", want: failureClassBudgetExhausted},
		{name: "budget: context length exceeded", failureReason: "context length exceeded: 200k tokens", want: failureClassBudgetExhausted},
		{name: "budget: context window exceeded", failureReason: "context window exceeded", want: failureClassBudgetExhausted},
		{name: "budget: budget exhausted", failureReason: "budget exhausted for this run", want: failureClassBudgetExhausted},

		// ── structural ────────────────────────────────────────────────────────────
		{name: "structural: write_scope_violation", failureReason: "write_scope_violation: path outside repo", want: failureClassStructural},
		{name: "structural: write scope violation (space)", failureReason: "write scope violation detected", want: failureClassStructural},
		{name: "structural: scope violation", failureReason: "scope violation: tried to write /etc/passwd", want: failureClassStructural},

		// ── deterministic (default / fall-through) ────────────────────────────────
		// IMPORTANT: any error string not matching a known hint falls through to
		// deterministic. This is an explicit design choice (fail-closed): an
		// unrecognised error must not silently enable unlimited retries.
		{name: "deterministic: unknown error", failureReason: "something went completely wrong", want: failureClassDeterministic},
		{name: "deterministic: empty reason", failureReason: "", want: failureClassDeterministic},
		{name: "deterministic: assertion failure", failureReason: "assertion failed: expected foo got bar", want: failureClassDeterministic},
		{name: "deterministic: contract mismatch", failureReason: "provider contract mismatch: missing field X", want: failureClassDeterministic},
		{name: "deterministic: playwright browser launch failed (missing deps)", failureReason: "browserType.launch: Host system is missing dependencies", want: failureClassDeterministic},
		{name: "deterministic: playwright executable missing", failureReason: "browserType.launch: Executable doesn't exist at /home/user/.cache/ms-playwright/chromium", want: failureClassDeterministic},
		{name: "deterministic: playwright install hint", failureReason: "Please run the following command to download new browsers: npx playwright install", want: failureClassDeterministic},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runtime.Outcome{
				Status:        runtime.StatusFail,
				FailureReason: tc.failureReason,
			}
			got := classifyFailureClass(out)
			if got != tc.want {
				t.Fatalf("classifyFailureClass(reason=%q) = %q, want %q", tc.failureReason, got, tc.want)
			}
		})
	}
}

// TestClassifyFailureClass_RetryStatusUsesHeuristics verifies that heuristic
// pattern matching applies to StatusRetry outcomes as well as StatusFail.
func TestClassifyFailureClass_RetryStatusUsesHeuristics(t *testing.T) {
	cases := []struct {
		name          string
		failureReason string
		want          string
	}{
		{name: "retry + timeout -> transient", failureReason: "timeout", want: failureClassTransientInfra},
		{name: "retry + turn limit -> budget", failureReason: "turn limit reached", want: failureClassBudgetExhausted},
		{name: "retry + unknown -> deterministic", failureReason: "something unrecognised", want: failureClassDeterministic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runtime.Outcome{
				Status:        runtime.StatusRetry,
				FailureReason: tc.failureReason,
			}
			got := classifyFailureClass(out)
			if got != tc.want {
				t.Fatalf("classifyFailureClass(status=retry, reason=%q) = %q, want %q", tc.failureReason, got, tc.want)
			}
		})
	}
}

// TestClassifyFailureClass_NonFailureStatusReturnsEmpty verifies that success
// and other non-failure statuses do not trigger any failure classification.
func TestClassifyFailureClass_NonFailureStatusReturnsEmpty(t *testing.T) {
	statuses := []runtime.StageStatus{
		runtime.StatusSuccess,
		runtime.StatusPartialSuccess,
		runtime.StatusSkipped,
	}
	for _, st := range statuses {
		out := runtime.Outcome{Status: st, FailureReason: "timeout"}
		if got := classifyFailureClass(out); got != "" {
			t.Fatalf("classifyFailureClass(status=%q) = %q, want empty string", st, got)
		}
	}
}

// TestNormalizedFailureClass_AllAliases pins every alias string in the
// normalizedFailureClass switch so that renaming a case arm is caught.
func TestNormalizedFailureClass_AllAliases(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		// transient_infra aliases
		{raw: "transient", want: failureClassTransientInfra},
		{raw: "transient_infra", want: failureClassTransientInfra},
		{raw: "transient-infra", want: failureClassTransientInfra},
		{raw: "infra_transient", want: failureClassTransientInfra},
		{raw: "transient infra", want: failureClassTransientInfra},
		{raw: "infrastructure_transient", want: failureClassTransientInfra},
		{raw: "retryable", want: failureClassTransientInfra},
		{raw: "toolchain_workspace_io", want: failureClassTransientInfra},
		{raw: "toolchain-workspace-io", want: failureClassTransientInfra},
		{raw: "toolchain_or_dependency_registry_unavailable", want: failureClassTransientInfra},
		{raw: "toolchain-dependency-registry-unavailable", want: failureClassTransientInfra},
		// canceled aliases
		{raw: "canceled", want: failureClassCanceled},
		{raw: "cancelled", want: failureClassCanceled},
		// deterministic aliases
		{raw: "deterministic", want: failureClassDeterministic},
		{raw: "non_transient", want: failureClassDeterministic},
		{raw: "non-transient", want: failureClassDeterministic},
		{raw: "permanent", want: failureClassDeterministic},
		{raw: "logic", want: failureClassDeterministic},
		{raw: "product", want: failureClassDeterministic},
		// budget_exhausted aliases
		{raw: "budget_exhausted", want: failureClassBudgetExhausted},
		{raw: "budget-exhausted", want: failureClassBudgetExhausted},
		{raw: "budget exhausted", want: failureClassBudgetExhausted},
		{raw: "budget", want: failureClassBudgetExhausted},
		// compilation_loop aliases
		{raw: "compilation_loop", want: failureClassCompilationLoop},
		{raw: "compilation-loop", want: failureClassCompilationLoop},
		{raw: "compilation loop", want: failureClassCompilationLoop},
		{raw: "compile_loop", want: failureClassCompilationLoop},
		{raw: "compile-loop", want: failureClassCompilationLoop},
		// structural aliases
		{raw: "structural", want: failureClassStructural},
		{raw: "structure", want: failureClassStructural},
		{raw: "scope_violation", want: failureClassStructural},
		{raw: "write_scope_violation", want: failureClassStructural},
		// empty / nil-like -> empty string (no classification)
		{raw: "", want: ""},
		{raw: "<nil>", want: ""},
		// unknown alias -> deterministic (fail-closed default)
		{raw: "completely_unknown_alias", want: failureClassDeterministic},
		// case-insensitivity
		{raw: "TRANSIENT", want: failureClassTransientInfra},
		{raw: "Deterministic", want: failureClassDeterministic},
		{raw: "CANCELED", want: failureClassCanceled},
		// trailing/leading whitespace is ignored
		{raw: "  transient  ", want: failureClassTransientInfra},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := normalizedFailureClass(tc.raw)
			if got != tc.want {
				t.Fatalf("normalizedFailureClass(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestTransientInfraReasonHints_ShadowedEntriesExist pins the five hints that
// are shadowed by broader hints appearing earlier in the slice. Because
// classifyFailureClass returns on the first match, these specific hints can
// never be isolated via routing tests — but they still document important
// user-facing error strings and must not be silently deleted.
func TestTransientInfraReasonHints_ShadowedEntriesExist(t *testing.T) {
	// These hints are shadowed by broader hints that appear earlier in the slice,
	// so they can't be isolated via classifyFailureClass routing. Test their
	// presence directly to pin them against accidental deletion.
	shadowed := []string{
		"tls handshake timeout",
		"i/o timeout",
		"gateway timeout",
		"temporary failure in name resolution",
		"could not resolve hostname",
	}
	for _, want := range shadowed {
		found := false
		for _, h := range transientInfraReasonHints {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected hint %q in transientInfraReasonHints (shadowed by broader hint but still documents intent)", want)
		}
	}
}

// TestTransientInfraReasonHints_Count guards the slice length so that adding
// or removing hints requires an explicit update to this test.
func TestTransientInfraReasonHints_Count(t *testing.T) {
	const want = 38
	if got := len(transientInfraReasonHints); got != want {
		t.Fatalf("len(transientInfraReasonHints) = %d, want %d; update this test when adding/removing hints", got, want)
	}
}

// TestBudgetExhaustedReasonHints_Count guards the slice length so that adding
// or removing hints requires an explicit update to this test.
func TestBudgetExhaustedReasonHints_Count(t *testing.T) {
	const want = 10
	if got := len(budgetExhaustedReasonHints); got != want {
		t.Fatalf("len(budgetExhaustedReasonHints) = %d, want %d; update this test when adding/removing hints", got, want)
	}
}

// TestStructuralReasonHints_Count guards the slice length so that adding
// or removing hints requires an explicit update to this test.
func TestStructuralReasonHints_Count(t *testing.T) {
	const want = 3
	if got := len(structuralReasonHints); got != want {
		t.Fatalf("len(structuralReasonHints) = %d, want %d; update this test when adding/removing hints", got, want)
	}
}
