package gonfig

import (
	"errors"
	"os"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// yamlLoader is a configuration loader that reads YAML files.
type yamlLoader struct {
	Parser

	path *atomic.Pointer[string]
}

const (
	// ParserYAML is the identifier for the YAML configuration parser.
	ParserYAML ParserType = "yaml-loader"

	// ErrYamlCantOpen is returned when the YAML file cannot be opened.
	ErrYamlCantOpen = constantError("(yaml-loader) could not open")

	// ErrYamlCantParse is returned when the YAML file cannot be parsed.
	ErrYamlCantParse = constantError("(yaml-loader) could not parse")
)

// WithYamlLoader returns a LoaderOption that enables YAML configuration parsing.
// It registers a YAML parser initializer using WithCustomParserInit.
func WithYamlLoader() LoaderOption {
	return WithCustomParserInit(initYamlLoader)
}

// yamlLoaderFunc loads and decodes a YAML configuration file into the provided structure.
//
// It retrieves the file path from an atomic pointer. If the path is not set, it returns nil.
// If the file cannot be opened, it returns ErrYamlCantOpen along with the original error.
// If decoding fails, it returns ErrYamlCantParse with the parsing error.
func yamlLoaderFunc(path *atomic.Pointer[string], v any) error {
	if filename := path.Load(); filename == nil || *filename == "" {
		return nil
	} else if file, err := os.Open(*filename); err != nil {
		return errors.Join(ErrYamlCantOpen, err)
	} else if err = yaml.NewDecoder(file).Decode(v); err != nil {
		return errors.Join(ErrYamlCantParse, err)
	}

	return nil
}

// initYamlLoader initializes a YAML parser with an atomic pointer for the configuration file path.
//
// It returns a Parser that uses yamlLoaderFunc for reading and decoding YAML files.
func initYamlLoader(_ Config) (Parser, error) {
	var path atomic.Pointer[string]

	return &yamlLoader{
		path: &path,

		Parser: &parserFunc{
			name: ParserYAML,
			call: func(v any) error {
				return yamlLoaderFunc(&path, v)
			},
		},
	}, nil
}

// NewYamlLoader creates a new YAML configuration loader.
func NewYamlLoader() Parser {
	l, _ := initYamlLoader(Config{})

	return l
}

func (y *yamlLoader) lazyInit() {
	if y.path == nil {
		y.path = new(atomic.Pointer[string])
	}
}

// SetConfigPath sets the path to the YAML configuration file.
func (y *yamlLoader) SetConfigPath(path string) { y.lazyInit(); y.path.Store(&path) }
