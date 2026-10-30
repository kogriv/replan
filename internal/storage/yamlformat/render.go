package yamlformat

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Render is the canonical two-space YAML renderer for project files and
// command previews that create complete YAML documents.
func Render(value any) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close YAML encoder: %w", err)
	}
	return out.Bytes(), nil
}
