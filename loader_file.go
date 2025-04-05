package gonfig

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	// ParserJSON is the identifier for the JSON configuration parser.
	ParserJSON ParserType = "json-loader"

	// ParserYAML is the identifier for the YAML configuration parser.
	ParserYAML ParserType = "yaml-loader"

	// ParserTOML is the identifier for the TOML configuration parser.
	ParserTOML ParserType = "toml-loader"

	// ErrCantOpen is returned when the file cannot be opened.
	ErrCantOpen Error = "could not open"

	// ErrCantClose is returned when the file cannot be closed.
	ErrCantClose Error = "could not close"

	// ErrCantParse is returned when the file cannot be parsed.
	ErrCantParse Error = "could not parse"

	// ErrUnknownFileTypeParser is returned when ParserType is not known.
	ErrUnknownFileTypeParser Error = "unknown parser"
)

type fileLoader struct {
	Parser

	path *atomic.Pointer[string]
}

type fileOpener func(string) (io.ReadCloser, error)

type fileLoaderOptions struct{ open fileOpener }

type fileLoaderOption func(*fileLoaderOptions)

// WithJSONLoader returns a LoaderOption that enables JSON configuration parsing.
// It registers a JSON parser initializer using WithCustomParserInit.
func WithJSONLoader(options ...fileLoaderOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserJSON, options))
}

// WithYAMLLoader returns a LoaderOption that enables YAML configuration parsing.
// It registers a YAML parser initializer using WithCustomParserInit.
func WithYAMLLoader(options ...fileLoaderOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserYAML, options))
}

// WithTOMLLoader returns a LoaderOption that enables TOML configuration parsing.
func WithTOMLLoader(options ...fileLoaderOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserTOML, options))
}

// initFileLoader initializes a JSON parser with an atomic pointer for the configuration file path.
//
// It returns a Parser that uses jsonLoaderFunc for reading and decoding JSON files.
func initFileLoader(kind ParserType, options []fileLoaderOption) func(_ Config) (Parser, error) {
	settings := &fileLoaderOptions{open: func(filename string) (io.ReadCloser, error) {
		filename = filepath.Clean(filename)

		return os.Open(filename)
	}}

	for _, option := range options {
		option(settings)
	}

	return func(_ Config) (Parser, error) {
		var path atomic.Pointer[string]

		return &fileLoader{
			path: &path,

			Parser: &parserFunc{
				name: kind,
				call: func(v any) error {
					switch kind {
					case ParserJSON:
						return loadFromFile(&path, settings.open, func(r io.Reader) error {
							return json.NewDecoder(r).Decode(v)
						})
					case ParserYAML:
						return loadFromFile(&path, settings.open, func(r io.Reader) error {
							return yaml.NewDecoder(r).Decode(v)
						})
					case ParserTOML:
						return loadFromFile(&path, settings.open, func(r io.Reader) error {
							return toml.NewDecoder(r).Decode(v)
						})
					default:
						return ErrUnknownFileTypeParser
					}
				},
			},
		}, nil
	}
}

// SetConfigPath sets the path to the JSON configuration file.
func (y *fileLoader) SetConfigPath(path string) { y.path.Store(&path) }

func loadFromFile(path *atomic.Pointer[string], open fileOpener, decode func(io.Reader) error) error {
	if filename := path.Load(); filename == nil || *filename == "" {
		return nil
	} else if file, err := open(*filename); err != nil {
		return errors.Join(ErrCantOpen, err)
	} else if err = decode(file); err != nil {
		return errors.Join(ErrCantParse, err)
	} else if err = file.Close(); err != nil {
		return errors.Join(ErrCantClose, err)
	}

	return nil
}
