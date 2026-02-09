package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/strongdm/kilroy/internal/llm"
	"github.com/strongdm/kilroy/internal/llm/providers/anthropic"
	"github.com/strongdm/kilroy/internal/llm/providers/google"
	"github.com/strongdm/kilroy/internal/llm/providers/openai"
	"github.com/strongdm/kilroy/internal/providerspec"
)

func newAPIClientFromProviderRuntimes(runtimes map[string]ProviderRuntime) (*llm.Client, error) {
	c := llm.NewClient()
	for _, key := range sortedKeys(runtimes) {
		rt := runtimes[key]
		if rt.Backend != BackendAPI {
			continue
		}
		apiKey := strings.TrimSpace(os.Getenv(rt.API.DefaultAPIKeyEnv))
		if apiKey == "" {
			continue
		}
		switch rt.API.Protocol {
		case providerspec.ProtocolOpenAIResponses:
			c.Register(openai.NewWithProvider(key, apiKey, rt.API.DefaultBaseURL))
		case providerspec.ProtocolAnthropicMessages:
			c.Register(anthropic.NewWithProvider(key, apiKey, rt.API.DefaultBaseURL))
		case providerspec.ProtocolGoogleGenerateContent:
			c.Register(google.NewWithProvider(key, apiKey, rt.API.DefaultBaseURL))
		case providerspec.ProtocolOpenAIChatCompletions:
			// Added in Task 5 with openaicompat adapter registration.
			continue
		default:
			return nil, fmt.Errorf("unsupported api protocol %q for provider %s", rt.API.Protocol, key)
		}
	}
	// Empty API clients are valid (for example, CLI-only runs).
	return c, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
