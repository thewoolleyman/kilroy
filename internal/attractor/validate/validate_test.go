package validate

import (
	"strings"
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/dot"
)

func TestValidate_StartAndExitNodeRules(t *testing.T) {
	// Missing start node.
	g1, err := dot.Parse([]byte(`digraph G { exit [shape=Msquare] }`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d1 := Validate(g1)
	assertHasRule(t, d1, "start_node", SeverityError)

	// Missing exit node.
	g2, err := dot.Parse([]byte(`digraph G { start [shape=Mdiamond] }`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d2 := Validate(g2)
	assertHasRule(t, d2, "terminal_node", SeverityError)
}

func TestValidate_ReachabilityAndEdgeTargets(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  orphan [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a -> exit
  a -> missing
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "reachability", SeverityError)
	assertHasRule(t, diags, "edge_target_exists", SeverityError)

	// Spec DoD: lint diagnostics include node/edge IDs.
	foundNode := false
	foundEdge := false
	for _, d := range diags {
		if d.Rule == "reachability" && strings.TrimSpace(d.NodeID) != "" {
			foundNode = true
		}
		if d.Rule == "edge_target_exists" && (strings.TrimSpace(d.EdgeFrom) != "" || strings.TrimSpace(d.EdgeTo) != "") {
			foundEdge = true
		}
	}
	if !foundNode {
		t.Fatalf("expected reachability diagnostic to include node_id")
	}
	if !foundEdge {
		t.Fatalf("expected edge_target_exists diagnostic to include edge ids")
	}
}

func TestValidate_StartNoIncomingAndExitNoOutgoing(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a -> exit
  a -> start
  exit -> a
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "start_no_incoming", SeverityError)
	assertHasRule(t, diags, "exit_no_outgoing", SeverityError)
}

func TestValidate_ConditionAndStylesheetSyntax(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [model_stylesheet="* { llm_provider: openai; } box { llm_model: gpt-5.2; }"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a -> exit
  a -> exit [condition="outcome>success"]
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "condition_syntax", SeverityError)
}

func TestValidate_LLMProviderRequired_Metaspec(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_model=gpt-5.2]
  start -> a -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "llm_provider_required", SeverityError)
}

func TestValidate_PromptOnCodergenNodes_WarnsWhenMissingPrompt(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	found := false
	for _, d := range diags {
		if d.Rule == "prompt_on_llm_nodes" && d.Severity == SeverityWarning && d.NodeID == "a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected prompt_on_llm_nodes WARNING for node a; got %+v", diags)
	}
}

func TestValidate_LoopRestartFailureEdgeRequiresTransientInfraGuard(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  check [shape=diamond]
  start -> a -> check
  check -> a [condition="outcome=fail", loop_restart=true]
  check -> exit [condition="outcome=success"]
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "loop_restart_failure_class_guard", SeverityWarning)
}

func TestValidate_LoopRestartFailureEdgeWithTransientInfraGuard_NoWarning(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  check [shape=diamond]
  start -> a -> check
  check -> a [condition="outcome=fail && context.failure_class=transient_infra", loop_restart=true]
  check -> exit [condition="outcome=success"]
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	for _, d := range diags {
		if d.Rule == "loop_restart_failure_class_guard" {
			t.Fatalf("unexpected loop_restart_failure_class_guard warning: %+v", d)
		}
	}
}

func TestValidate_EscalationModelsSyntax_Valid_NoWarning(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x", escalation_models="kimi:kimi-k2.5, anthropic:claude-opus-4-6"]
  start -> a -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	for _, d := range diags {
		if d.Rule == "escalation_models_syntax" {
			t.Fatalf("unexpected escalation_models_syntax warning for valid entries: %+v", d)
		}
	}
}

func TestValidate_EscalationModelsSyntax_MissingColon(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x", escalation_models="kimi-kimi-k2.5"]
  start -> a -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "escalation_models_syntax", SeverityWarning)
}

func TestValidate_EscalationModelsSyntax_EmptyProvider(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x", escalation_models=":some-model"]
  start -> a -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "escalation_models_syntax", SeverityWarning)
}

func TestValidate_ShapeAliases_DownstreamLintsFireForCircleAndDoublecircle(t *testing.T) {
	// circle=start, doublecircle=exit aliases should be recognized by downstream lints
	// (start_no_incoming, exit_no_outgoing, reachability) not just lintStartNode/lintExitNode.
	g, err := dot.Parse([]byte(`
digraph G {
  s [shape=circle]
  e [shape=doublecircle]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  s -> a -> e
  a -> s
  e -> a
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	// Should fire start_no_incoming (a->s) and exit_no_outgoing (e->a).
	assertHasRule(t, diags, "start_no_incoming", SeverityError)
	assertHasRule(t, diags, "exit_no_outgoing", SeverityError)

	// Reachability should also work — all nodes reachable, so no reachability errors.
	for _, d := range diags {
		if d.Rule == "reachability" {
			t.Fatalf("unexpected reachability error for fully connected alias-shaped graph: %+v", d)
		}
	}
}

func TestValidate_GoalGateExitStatusContract_ErrorAndPromptWarning(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [retry_target=implement]
  start [shape=Mdiamond]
  exit [shape=Msquare]
  implement [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  review_consensus [
    shape=box,
    goal_gate=true,
    llm_provider=openai,
    llm_model=gpt-5.2,
    prompt="Review and decide outcome.\nWrite status JSON with outcome=pass when approved."
  ]
  start -> review_consensus
  review_consensus -> exit [condition="outcome=pass"]
  review_consensus -> implement [condition="outcome=retry"]
  implement -> exit [condition="outcome=success"]
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertHasRule(t, diags, "goal_gate_exit_status_contract", SeverityError)
	assertHasRule(t, diags, "goal_gate_prompt_status_hint", SeverityWarning)
}

func TestValidate_GoalGateExitStatusContract_AllowsCanonicalSuccessOutcomes(t *testing.T) {
	tests := []struct {
		name         string
		exitOutcome  string
		promptStatus string
	}{
		{
			name:         "success",
			exitOutcome:  "success",
			promptStatus: "success",
		},
		{
			name:         "partial_success",
			exitOutcome:  "partial_success",
			promptStatus: "partial_success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dotSrc := `
digraph G {
  graph [retry_target=implement]
  start [shape=Mdiamond]
  exit [shape=Msquare]
  implement [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  review_consensus [
    shape=box,
    goal_gate=true,
    llm_provider=openai,
    llm_model=gpt-5.2,
    prompt="Review and decide outcome.\nWrite status JSON with outcome=` + tc.promptStatus + ` when approved."
  ]
  start -> review_consensus
  review_consensus -> exit [condition="outcome=` + tc.exitOutcome + `"]
  review_consensus -> implement [condition="outcome=retry"]
  implement -> exit [condition="outcome=success"]
}
`
			g, err := dot.Parse([]byte(dotSrc))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diags := Validate(g)
			assertNoRule(t, diags, "goal_gate_exit_status_contract")
			assertNoRule(t, diags, "goal_gate_prompt_status_hint")
		})
	}
}

func TestValidate_GoalGateExitStatusContract_NoTerminalMismatchNoError(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [retry_target=implement]
  start [shape=Mdiamond]
  exit [shape=Msquare]
  implement [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="x"]
  review_consensus [
    shape=box,
    goal_gate=true,
    llm_provider=openai,
    llm_model=gpt-5.2,
    prompt="Review and decide outcome.\nWrite status JSON with outcome=success when approved."
  ]
  review_router [shape=diamond]
  start -> review_consensus
  review_consensus -> review_router [condition="outcome=pass"]
  review_router -> exit [condition="outcome=success"]
  review_consensus -> implement [condition="outcome=retry"]
  implement -> exit [condition="outcome=success"]
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(g)
	assertNoRule(t, diags, "goal_gate_exit_status_contract")
	assertNoRule(t, diags, "goal_gate_prompt_status_hint")
}

func assertHasRule(t *testing.T, diags []Diagnostic, rule string, sev Severity) {
	t.Helper()
	for _, d := range diags {
		if d.Rule == rule && d.Severity == sev {
			return
		}
	}
	var got []string
	for _, d := range diags {
		got = append(got, string(d.Severity)+":"+d.Rule)
	}
	t.Fatalf("expected %s:%s; got %s", sev, rule, strings.Join(got, ", "))
}

func assertNoRule(t *testing.T, diags []Diagnostic, rule string) {
	t.Helper()
	for _, d := range diags {
		if d.Rule == rule {
			t.Fatalf("unexpected diagnostic %s:%s (%s)", d.Severity, d.Rule, d.Message)
		}
	}
}
