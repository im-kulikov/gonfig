package gonfig

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// yamlLoader is a configuration loader that reads YAML files.
type yamlLoader struct{ path string }

const (
	// ParserYAML is the identifier for the YAML configuration parser.
	ParserYAML ParserType = "yaml-loader"

	// ErrYamlCantOpen is returned when the YAML file cannot be opened.
	ErrYamlCantOpen = constantError("(yaml-loader) could not open")

	// ErrYamlCantParse is returned when the YAML file cannot be parsed.
	ErrYamlCantParse = constantError("(yaml-loader) could not parse")
)

// NewYamlLoader creates a new YAML configuration loader.
func NewYamlLoader() Parser { return new(yamlLoader) }

// SetConfigPath sets the path to the YAML configuration file.
func (y *yamlLoader) SetConfigPath(path string) { y.path = path }

// Load reads the YAML configuration file and decodes it into the provided variable.
func (y *yamlLoader) Load(v any) error {
	if y.path == "" {
		return nil
	}

	if file, err := os.Open(y.path); err != nil {
		return errors.Join(ErrYamlCantOpen, err)
	} else if err = yaml.NewDecoder(file).Decode(v); err != nil {
		return errors.Join(ErrYamlCantParse, err)
	}

	return nil
}

// Type returns the parser type identifier for YAML.
func (y *yamlLoader) Type() ParserType { return ParserYAML }
