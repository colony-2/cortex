package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

func readData(path string) ([]byte, error) {
	switch path {
	case "":
		return nil, fmt.Errorf("no input provided; use --file or - for stdin")
	case "-":
		return io.ReadAll(os.Stdin)
	default:
		return os.ReadFile(path)
	}
}

func unmarshalYAMLOrJSON(data []byte, dest any) error {
	return yaml.Unmarshal(data, dest)
}

func parseKeyValue(values []string) (map[string]interface{}, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]interface{}, len(values))
	for _, v := range values {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field %q (use key=value)", v)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func boolPtr(v bool) *bool {
	return &v
}
