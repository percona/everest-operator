package dataimporterspec

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadFromFilepath reads the configuration from a JSON file at the specified filepath.
func (in *Spec) ReadFromFilepath(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	return json.Unmarshal(data, in)
}
