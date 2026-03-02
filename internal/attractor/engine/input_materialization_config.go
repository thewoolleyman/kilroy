package engine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type InputImportEntry struct {
	Pattern  string `json:"pattern" yaml:"pattern"`
	Required *bool  `json:"required,omitempty" yaml:"required,omitempty"`
}

type InputMaterializationFanInConfig struct {
	PromoteRunScoped []string `json:"promote_run_scoped,omitempty" yaml:"promote_run_scoped,omitempty"`
}

type materializeFieldPresence struct {
	IncludeDeclared        bool
	DefaultIncludeDeclared bool
}

func detectMaterializeFieldPresence(path string, raw []byte) (materializeFieldPresence, error) {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return detectMaterializeFieldPresenceJSON(raw)
	}
	return detectMaterializeFieldPresenceYAML(raw)
}

func detectMaterializeFieldPresenceYAML(raw []byte) (materializeFieldPresence, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return materializeFieldPresence{}, err
	}

	doc := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		doc = root.Content[0]
	}

	inputs := yamlChildMap(doc, "inputs")
	materialize := yamlChildMap(inputs, "materialize")
	return materializeFieldPresence{
		IncludeDeclared:        yamlMapHasKey(materialize, "include"),
		DefaultIncludeDeclared: yamlMapHasKey(materialize, "default_include"),
	}, nil
}

func detectMaterializeFieldPresenceJSON(raw []byte) (materializeFieldPresence, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return materializeFieldPresence{}, err
	}

	m := nestedMap(root, "inputs", "materialize")
	_, includeDeclared := m["include"]
	_, defaultIncludeDeclared := m["default_include"]
	return materializeFieldPresence{
		IncludeDeclared:        includeDeclared,
		DefaultIncludeDeclared: defaultIncludeDeclared,
	}, nil
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	cur := root
	for _, key := range keys {
		next, ok := cur[key].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = next
	}
	return cur
}

func yamlChildMap(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if strings.TrimSpace(k.Value) == key {
			return v
		}
	}
	return nil
}

func yamlMapHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.TrimSpace(node.Content[i].Value) == key {
			return true
		}
	}
	return false
}

var materializeConfigWindowsAbsPathRE = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

func isMaterializeConfigAbsolutePathLike(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if materializeConfigWindowsAbsPathRE.MatchString(path) {
		return true
	}
	return strings.HasPrefix(path, `\\`)
}

func (m *InputMaterializationConfig) normalizeImports(p materializeFieldPresence) error {
	if len(m.Imports) == 0 {
		return nil
	}
	if p.IncludeDeclared || p.DefaultIncludeDeclared || len(m.Include) > 0 || len(m.DefaultInclude) > 0 {
		return fmt.Errorf("failure_reason=input_imports_conflict: imports cannot be combined with include/default_include")
	}

	required := make([]string, 0, len(m.Imports))
	bestEffort := make([]string, 0, len(m.Imports))
	seen := map[string]bool{}
	for i, imp := range m.Imports {
		pattern := strings.TrimSpace(imp.Pattern)
		if pattern == "" {
			return fmt.Errorf("inputs.materialize.imports[%d].pattern is required", i)
		}
		if seen[pattern] {
			continue
		}
		isRequired := true
		if imp.Required != nil {
			isRequired = *imp.Required
		}
		if isRequired {
			required = append(required, pattern)
		} else {
			bestEffort = append(bestEffort, pattern)
		}
		seen[pattern] = true
	}
	m.Include = required
	m.DefaultInclude = bestEffort
	return nil
}

func normalizeAndValidatePromoteRunScoped(entries []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(entries))
	for i, raw := range entries {
		s := strings.TrimSpace(raw)
		if s == "" {
			return nil, fmt.Errorf("inputs.materialize.fan_in.promote_run_scoped[%d] must be non-empty", i)
		}
		if isMaterializeConfigAbsolutePathLike(s) {
			return nil, fmt.Errorf("inputs.materialize.fan_in.promote_run_scoped[%d] must be relative", i)
		}

		normalizedSlashes := strings.ReplaceAll(s, `\`, `/`)
		for _, seg := range strings.Split(normalizedSlashes, "/") {
			if seg == ".." {
				return nil, fmt.Errorf("inputs.materialize.fan_in.promote_run_scoped[%d] must not contain '..' segments", i)
			}
		}

		clean := filepath.ToSlash(filepath.Clean(normalizedSlashes))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("inputs.materialize.fan_in.promote_run_scoped[%d] must not contain '..' segments", i)
		}
		clean = strings.TrimPrefix(clean, "./")
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out, nil
}
