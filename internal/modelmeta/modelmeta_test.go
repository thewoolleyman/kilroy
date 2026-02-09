package modelmeta

import "testing"

func TestNormalizeProvider_UsesCanonicalAliases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "gemini", want: "google"},
		{in: "google_ai_studio", want: "google"},
		{in: "moonshot", want: "kimi"},
		{in: "moonshotai", want: "kimi"},
		{in: "z-ai", want: "zai"},
		{in: "z.ai", want: "zai"},
		{in: " openai ", want: "openai"},
	}
	for _, tc := range cases {
		if got := NormalizeProvider(tc.in); got != tc.want {
			t.Fatalf("NormalizeProvider(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
