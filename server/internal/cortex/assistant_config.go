package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// LoadAssistantRuntimeConfig reads only explicitly selected trusted server input.
// No provider credential is loaded: bindings contain a named-secret reference.
func LoadAssistantRuntimeConfig(path string) (*AssistantRuntimeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return nil, fmt.Errorf("invalid assistant configuration")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var cfg AssistantRuntimeConfig
	if err = d.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid assistant configuration")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return nil, fmt.Errorf("invalid assistant configuration")
	}
	return &cfg, nil
}
