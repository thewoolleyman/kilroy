package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Checkpoint struct {
	Timestamp time.Time `json:"timestamp"`

	// CurrentNode is the ID of the last completed node.
	CurrentNode string `json:"current_node"`

	CompletedNodes []string       `json:"completed_nodes"`
	NodeRetries    map[string]int `json:"node_retries"`
	ContextValues  map[string]any `json:"context"`
	Logs           []string       `json:"logs"`
	GitCommitSHA   string         `json:"git_commit_sha,omitempty"` // Kilroy extension (metaspec)
	Extra          map[string]any `json:"extra,omitempty"`          // forward-compat
}

func NewCheckpoint() *Checkpoint {
	return &Checkpoint{
		Timestamp:      time.Now().UTC(),
		CompletedNodes: []string{},
		NodeRetries:    map[string]int{},
		ContextValues:  map[string]any{},
		Logs:           []string{},
		Extra:          map[string]any{},
	}
}

func (cp *Checkpoint) Save(path string) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	return WriteJSONAtomicFile(path, cp)
}

func LoadCheckpoint(path string) (*Checkpoint, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	if cp.NodeRetries == nil {
		cp.NodeRetries = map[string]int{}
	}
	if cp.ContextValues == nil {
		cp.ContextValues = map[string]any{}
	}
	if cp.CompletedNodes == nil {
		cp.CompletedNodes = []string{}
	}
	if cp.Logs == nil {
		cp.Logs = []string{}
	}
	if cp.Extra == nil {
		cp.Extra = map[string]any{}
	}
	return &cp, nil
}
