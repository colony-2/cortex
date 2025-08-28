package recipe

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func LoadRecipeFromString(data []byte) (*Recipe, error) {
	recipe := &Recipe{}
	err := yaml.Unmarshal(data, recipe)
	return recipe, err
}

func LoadRecipeFromReader(r io.Reader) (*Recipe, error) {
	recipe := &Recipe{}
	err := yaml.NewDecoder(r).Decode(&recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to decode recipe: %w", err)
	}
	return recipe, nil
}
