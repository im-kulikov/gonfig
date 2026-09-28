package gonfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-viper/mapstructure/v2"
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

	path   *atomic.Pointer[string]
	strict *atomic.Bool
}

type fileOpener func(string) (io.ReadCloser, error)

type fileLoaderOptions struct{ open fileOpener }

// FileOption configures a file loader: WithYAMLLoader(FromFS(configs)).
type FileOption func(*fileLoaderOptions)

// FromFS makes a file loader read the config path from fsys, such as an embed.FS with
// a config built into the binary, or a fstest.MapFS in tests. The path is a path in
// fsys: slash-separated and unrooted, a leading "./" is dropped (configs/app.yaml).
func FromFS(fsys fs.FS) FileOption {
	return func(o *fileLoaderOptions) {
		o.open = func(name string) (io.ReadCloser, error) { return fsys.Open(path.Clean(name)) }
	}
}

// fileFormat describes how a file loader reads its format: parse turns the file
// into a generic map, tag names the struct tag that maps its keys to fields, and
// extensions are the file names it reads when several file loaders are added.
type fileFormat struct {
	tag        string
	parse      func(data []byte, tree any) error
	extensions []string
}

// fileFormats are the supported file loaders.
var fileFormats = map[ParserType]fileFormat{
	ParserJSON: {tag: "json", parse: parseJSON, extensions: []string{".json"}},
	ParserYAML: {tag: "yaml", parse: parseYAML, extensions: []string{".yaml", ".yml"}},
	ParserTOML: {tag: "toml", parse: toml.Unmarshal, extensions: []string{".toml"}},
}

// WithJSONLoader returns a LoaderOption that enables JSON configuration parsing.
// It registers a JSON parser initializer using WithCustomParserInit.
func WithJSONLoader(options ...FileOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserJSON, options))
}

// WithYAMLLoader returns a LoaderOption that enables YAML configuration parsing.
// It registers a YAML parser initializer using WithCustomParserInit.
func WithYAMLLoader(options ...FileOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserYAML, options))
}

// WithTOMLLoader returns a LoaderOption that enables TOML configuration parsing.
func WithTOMLLoader(options ...FileOption) LoaderOption {
	return WithCustomParserInit(initFileLoader(ParserTOML, options))
}

// initFileLoader initializes a file parser of the given kind. The path to the
// file is set later through SetConfigPath; without a path the parser does nothing.
func initFileLoader(kind ParserType, options []FileOption) func(_ Config) (Parser, error) {
	settings := &fileLoaderOptions{open: func(filename string) (io.ReadCloser, error) {
		filename = filepath.Clean(filename)

		return os.Open(filename)
	}}

	for _, option := range options {
		option(settings)
	}

	return func(_ Config) (Parser, error) {
		var (
			path   atomic.Pointer[string]
			strict atomic.Bool
		)

		return &fileLoader{
			path:   &path,
			strict: &strict,

			Parser: &parserFunc{
				name: kind,
				call: func(v any) error {
					format, ok := fileFormats[kind]
					if !ok {
						return ErrUnknownFileTypeParser
					}

					return loadFromFile(&path, settings.open, func(r io.Reader) error {
						return decodeFile(format, r, v, strict.Load())
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

func (y *fileLoader) setStrict(strict bool) {
	y.strict.Store(strict)
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
func decodeFile(format fileFormat, r io.Reader, dest any, strict bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var tree map[string]any
	if err = format.parse(data, &tree); err != nil || len(tree) == 0 {
		return err // an empty file or one with comments only changes nothing
	}

	return decodeMap(dest, tree, decodeOptions{
		tag:      format.tag,
		inline:   fileInlineOption,
		untagged: true,
		strict:   strict,
	})
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

// yamlScalar is a scalar of a YAML file. A field reads its text, as it reads an
// environment variable: a string gets 1.10 as written, a number that does not fit
// is an error, a duration needs its unit. A field of type any, bool, a number or
// time.Time takes the value YAML resolves the scalar to (true, 8080, a date).
type yamlScalar struct {
	text  string
	value any
}

// yamlNode reads a YAML value into maps, lists and yamlScalar values; yaml.v3
// resolves anchors, aliases and merge keys (<<) on the way.
type yamlNode struct{ value any }

func (y *yamlNode) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		var nodes map[string]yamlNode
		if err := node.Decode(&nodes); err != nil {
			return err
		}

		tree := make(map[string]any, len(nodes))
		for key, value := range nodes {
			tree[key] = value.value
		}

		y.value = tree
	case yaml.SequenceNode:
		var nodes []yamlNode
		if err := node.Decode(&nodes); err != nil {
			return err
		}

		list := make([]any, len(nodes))
		for i, value := range nodes {
			list[i] = value.value
		}

		y.value = list
	default:
		var value any
		if err := node.Decode(&value); err != nil {
			return err
		}

		if value != nil { // null leaves the field as it is
			y.value = yamlScalar{text: node.Value, value: value}
		}
	}

	return nil
}

func parseYAML(data []byte, tree any) error {
	var root yamlNode
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	switch value := root.value.(type) {
	case nil: // empty
	case map[string]any:
		*tree.(*map[string]any) = value
	default:
		return fmt.Errorf("the file holds %T, not a mapping of keys", plain(value))
	}

	return nil
}

// yamlScalarHook gives a field the text or the value of a YAML scalar, see yamlScalar.
// A pointer gets it for its element.
func yamlScalarHook(_, to reflect.Type, data any) (any, error) {
	scalar, ok := data.(yamlScalar)
	if !ok || to.Kind() == reflect.Pointer {
		return data, nil
	}

	resolved := reflect.TypeOf(scalar.value)

	switch {
	case to == resolved:
		return scalar.value, nil
	case to == durationType || reflect.PointerTo(to).Implements(unmarshalType):
		return scalar.text, nil
	case to.Kind() == reflect.Bool:
		return yamlBool(scalar), nil
	case isNumber(to.Kind()) && isNumber(resolved.Kind()):
		return scalar.value, nil // numberRangeHook checks that it fits
	}

	return scalar.text, nil
}

// yamlBool accepts the booleans of YAML 1.1 in a bool field, as yaml.v3 does.
func yamlBool(scalar yamlScalar) any {
	switch scalar.text {
	case "y", "Y", "yes", "Yes", "YES", "on", "On", "ON":
		return true
	case "n", "N", "no", "No", "NO", "off", "Off", "OFF":
		return false
	}

	return scalar.text
}

// tomlTimeHook turns a TOML local date or date-time into a time.Time in the local
// time zone, as go-toml does; a time of day without a date is an error.
func tomlTimeHook(_, to reflect.Type, data any) (any, error) {
	if to != timeType {
		return data, nil
	}

	switch value := data.(type) {
	case toml.LocalDate:
		return value.AsTime(time.Local), nil
	case toml.LocalDateTime:
		return value.AsTime(time.Local), nil
	case toml.LocalTime:
		return nil, fmt.Errorf("the time of day %s has no date", value)
	}

	return data, nil
}

var timeType = reflect.TypeFor[time.Time]()

// noTagSquash turns off inlining by a tag option in mapstructure, which fails on a map:
// no tag has this option. Embedded structs are still inlined by mapstructure.
const noTagSquash = "\x00"

// inlineHook inlines the fields tagged `,inline` of a struct, as yaml.v3 does, by
// moving their keys under the field name before the struct is decoded: a struct, or a
// pointer to one, takes the keys of its fields; a map takes the keys no other field has.
// Without any of its keys a pointer stays nil.
func inlineHook(tag string) mapstructure.DecodeHookFuncType {
	return func(_, to reflect.Type, data any) (any, error) {
		tree, ok := data.(map[string]any)
		if !ok || to.Kind() != reflect.Struct {
			return data, nil
		}

		var inlined []reflect.StructField

		taken := inlineKeys(to, tag, &inlined)
		if len(inlined) == 0 {
			return data, nil
		}

		tree = maps.Clone(tree)
		fields := make(map[string]any, len(inlined))

		for _, field := range inlined {
			moved := make(map[string]any)

			own := inlineKeys(derefType(field.Type), tag, nil)
			for key, value := range tree {
				if field.Type.Kind() == reflect.Map && !matchesKey(taken, key) || matchesKey(own, key) {
					moved[key] = value
					delete(tree, key)
				}
			}

			if len(moved) > 0 {
				fields[field.Name] = moved
			}
		}

		maps.Copy(tree, fields)

		return tree, nil
	}
}

// inlineKeys returns the keys of the fields of a struct t, with the keys of its inlined
// structs, and adds its fields tagged `,inline` to inlined.
func inlineKeys(t reflect.Type, tag string, inlined *[]reflect.StructField) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}

	var keys []string

	for field := range t.Fields() {
		parts := strings.Split(field.Tag.Get(tag), ",")
		elem := derefType(field.Type)
		isStruct := elem.Kind() == reflect.Struct && !textStruct(elem)

		switch {
		case parts[0] == "-" || !field.IsExported() && !field.Anonymous:
		case field.Anonymous && isStruct: // inlined by mapstructure
			keys = append(keys, inlineKeys(elem, tag, inlined)...)
		case slices.Contains(parts[1:], fileInlineOption) && (isStruct || field.Type.Kind() == reflect.Map):
			if inlined != nil {
				*inlined = append(*inlined, field)
			}

			keys = append(keys, inlineKeys(elem, tag, nil)...)
		case parts[0] == "":
			keys = append(keys, field.Name)
		default:
			keys = append(keys, parts[0])
		}
	}

	return keys
}

// matchesKey reports whether key is one of keys, ignoring case, as mapstructure matches them.
func matchesKey(keys []string, key string) bool {
	return slices.ContainsFunc(keys, func(k string) bool { return strings.EqualFold(k, key) })
}

func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}

	return t
}
