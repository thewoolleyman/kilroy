package llm

import "testing"

func TestExecutionPolicy_Kimi(t *testing.T) {
	p := ExecutionPolicy("moonshot")
	if !p.ForceStream {
		t.Fatalf("ForceStream=false want true")
	}
	if got, want := p.MinMaxTokens, 16000; got != want {
		t.Fatalf("MinMaxTokens=%d want %d", got, want)
	}
	if p.Reason == "" {
		t.Fatalf("Reason should be non-empty")
	}
}

func TestExecutionPolicy_NonKimi(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic", "google", "zai", "minimax"} {
		t.Run(provider, func(t *testing.T) {
			p := ExecutionPolicy(provider)
			if p.ForceStream {
				t.Fatalf("ForceStream=true want false")
			}
			if p.MinMaxTokens != 0 {
				t.Fatalf("MinMaxTokens=%d want 0", p.MinMaxTokens)
			}
			if p.Reason != "" {
				t.Fatalf("Reason=%q want empty", p.Reason)
			}
		})
	}
}

func TestApplyExecutionPolicy_MinMaxTokens(t *testing.T) {
	cases := []struct {
		name     string
		start    *int
		policy   ProviderExecutionPolicy
		want     int
		wantSame bool
	}{
		{
			name:     "no max tokens set",
			start:    nil,
			policy:   ProviderExecutionPolicy{MinMaxTokens: 16000},
			want:     16000,
			wantSame: false,
		},
		{
			name:     "below floor",
			start:    intRef(16),
			policy:   ProviderExecutionPolicy{MinMaxTokens: 16000},
			want:     16000,
			wantSame: false,
		},
		{
			name:     "already at floor",
			start:    intRef(16000),
			policy:   ProviderExecutionPolicy{MinMaxTokens: 16000},
			want:     16000,
			wantSame: true,
		},
		{
			name:     "above floor",
			start:    intRef(32000),
			policy:   ProviderExecutionPolicy{MinMaxTokens: 16000},
			want:     32000,
			wantSame: true,
		},
		{
			name:     "policy disabled",
			start:    intRef(42),
			policy:   ProviderExecutionPolicy{MinMaxTokens: 0},
			want:     42,
			wantSame: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{
				Model:     "m",
				Provider:  "kimi",
				Messages:  []Message{User("hi")},
				MaxTokens: tc.start,
			}
			got := ApplyExecutionPolicy(req, tc.policy)
			if got.MaxTokens == nil {
				t.Fatalf("MaxTokens=nil")
			}
			if *got.MaxTokens != tc.want {
				t.Fatalf("MaxTokens=%d want %d", *got.MaxTokens, tc.want)
			}
			if tc.wantSame && tc.start != nil && got.MaxTokens != tc.start {
				t.Fatalf("expected MaxTokens pointer to be unchanged")
			}
			if !tc.wantSame && tc.start != nil && got.MaxTokens == tc.start {
				t.Fatalf("expected MaxTokens pointer to be replaced")
			}
		})
	}
}

func intRef(v int) *int { return &v }
