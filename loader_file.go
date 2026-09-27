package gonfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
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

// fileFormat describes how a file loader reads its format: parse turns the file
// into a generic map, tag names the struct tag that maps its keys to fields.
type fileFormat struct {
	tag   string
	parse func(data []byte, tree any) error
}

// fileFormats are the supported file loaders.
var fileFormats = map[ParserType]fileFormat{
	ParserJSON: {tag: "json", parse: parseJSON},
	ParserYAML: {tag: "yaml", parse: yaml.Unmarshal},
	ParserTOML: {tag: "toml", parse: toml.Unmarshal},
}

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

// initFileLoader initializes a file parser of the given kind. The path to the
// file is set later through SetConfigPath; without a path the parser does nothing.
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
					format, ok := fileFormats[kind]
					if !ok {
						return ErrUnknownFileTypeParser
					}

					return loadFromFile(&path, settings.open, func(r io.Reader) error {
						return decodeFile(format, r, v)
					})
				},
			},
		}, nil
	}
}

// SetConfigPath sets the path to the configuration file.
func (y *fileLoader) SetConfigPath(filename string) {
	y.path.Store(&filename)
}

func loadFromFile(path *atomic.Pointer[string], open fileOpener, decode func(io.Reader) error) (err error) {
	filename := path.Load()
	if filename == nil || *filename == "" {
		return nil
	}

	file, err := open(*filename)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCantOpen, err)
	}

	defer func() {
		// A close error matters only when reading succeeded; after a parse
		// error the file is closed anyway and the parse error is the one to report.
		if errClose := file.Close(); errClose != nil && err == nil {
			err = fmt.Errorf("%w %s: %w", ErrCantClose, *filename, errClose)
		}
	}()

	if err = decode(file); err != nil {
		return fmt.Errorf("%w %s: %w", ErrCantParse, *filename, singleLine{err})
	}

	return nil
}

// decodeFile reads a file into a generic map and decodes it by the same rules as
// environment variables: embedded structs are inlined, strings are converted by
// the shared hooks (durations, IP networks, encoding.TextUnmarshaler).
func decodeFile(format fileFormat, r io.Reader, dest any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var tree map[string]any
	if err = format.parse(data, &tree); err != nil || len(tree) == 0 {
		return err // an empty file or one with comments only changes nothing
	}

	return decodeMap(dest, tree, decodeOptions{tag: format.tag, inline: fileInlineOption, untagged: true})
}

// parseJSON keeps numbers as json.Number, so int64 values beyond 2^53 are not
// rounded through float64, and treats an empty file as an empty object.
func parseJSON(data []byte, tree any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	if err := dec.Decode(tree); !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}
