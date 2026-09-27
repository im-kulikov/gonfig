package gonfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
)

// EnvUsageOption defines a function type used to configure options for environment variable usage.
type EnvUsageOption func(*envUsageOptions)

// envUsageOptions holds configuration options for generating environment variable usage information.
type envUsageOptions struct {
	prefix string // Optional prefix to be added to environment variable names.
}

// envUsage represents metadata about an environment variable, including its name, usage description, and type.
// This struct is typically used to store and display information about environment variables in a user-friendly format.
//
// Fields:
// - Usage: A description of how the environment variable is intended to be used.
// - Name: The name of the environment variable.
// - Type: The expected data type of the environment variable (e.g., string, int, bool).
//
// This struct is useful when documenting or parsing environment variables in an application.
type envUsage struct {
	Usage string
	Name  string
	Type  string
}

const (
	// envPairDelim is the delimiter used to separate the environment variable name from its value.  Example: "KEY=value".
	envPairDelim = "="

	// envDelimiter is the delimiter used to separate different parts of a composite environment variable name.
	// It's typically used in multipart names where sections are separated by underscores.
	// Example: "APP_CONFIG_PATH".
	envDelimiter = "_"

	// envTag defines the struct tag key used to specify environment variable names for struct fields.
	// When parsing struct tags, this key indicates that a field should be populated from an environment variable.
	// Example usage: `env:"DB_HOST"`.
	envTag = "env"

	// ErrTestExit is an error indicating that a test process should exit.
	// This error can be used in testing scenarios where an explicit termination
	// or exit condition needs to be simulated.
	ErrTestExit = Error("exit code")

	// ErrPrepareDecoder is returned when the decoder initialization fails.
	// This error typically occurs when setting up a configuration decoder
	// encounters an issue, such as invalid decoder settings or unsupported types.
	ErrPrepareDecoder = Error("could not prepare decoder")

	// ErrDecode is returned when decoding a configuration fails.
	// This error indicates that the process of converting configuration data
	// into the expected structure was unsuccessful, possibly due to type mismatches
	// or missing required fields.
	ErrDecode = Error("could not decode")
)

// newEnvLoader creates a new parser that loads configuration from environment variables.
// It uses the provided environment variable slice and prefix to populate the configuration.
// Returns a Parser that processes environment variables with the specified prefix.
func newEnvLoader(l *loader) *parserFunc {
	return &parserFunc{name: ParserEnv, call: func(v any) error {
		return LoadEnvs(PrepareEnvs(l.Envs, l.EnvPrefix), v)
	}}
}

// EnvUsageWithPrefix creates an EnvUsageOption that sets a prefix for environment variables.
// This prefix is applied to each environment variable name when generating usage information.
//
// Parameters:
//   - prefix: The string prefix to add to environment variable names.
//
// Returns:
//   - EnvUsageOption: A function that modifies the prefix in envUsageOptions.
func EnvUsageWithPrefix(prefix string) EnvUsageOption {
	return func(opts *envUsageOptions) { opts.prefix = prefix }
}

// UsageOfEnvs generates a human-readable string that describes the environment variables
// expected by a given structure, based on struct tags (e.g., "env" and "usage").
//
// Parameters:
//   - dest: A pointer to a struct that defines the expected environment variables.
//     The struct fields must use the "env" tag to define environment variable names
//     and the "usage" tag to describe their purpose.
//   - opts: Optional EnvUsageOption(s) to configure behavior, such as adding a prefix to environment variable names.
//
// Returns:
//   - A string describing the environment variables and their usage, or an empty string if the input is not valid.
//
// The function ensures that the input is a pointer to a struct. It traverses the struct fields,
// generating usage information based on the tags. If a struct field is another struct, it recurses
// into the nested fields.
//
//nolint:funlen,gocognit
func UsageOfEnvs(dest any, opts ...EnvUsageOption) string {
	output := make([]envUsage, 0)
	exists := make(map[string]struct{})
	for field, err := range ReflectFieldsOf(withSections(dest), ReflectOptions{CanSet: True(), Pointers: true}) {
		if err != nil {
			return ""
		}

		var name string
		var unreachable bool
		for parent := field; parent != nil; parent = parent.Owner {
			tag := parent.Field.Tag.Get(envTag)
			tmp := strings.Split(tag, ",")
			env := tmp[0]

			if env == "-" {
				unreachable = true

				break
			}

			if env == "" && parent.Owner != nil {
				if slices.Contains(tmp, "squash") || parent.Field.Anonymous {
					continue
				}

				unreachable = true

				break
			}

			if env == "" {
				continue
			}

			if name == "" {
				name = env

				continue
			}

			name = env + envDelimiter + name
		}

		if unreachable || name == "" {
			continue
		}

		if _, ok := exists[name]; ok {
			continue
		}

		exists[name] = struct{}{}

		var usage string
		if usage = field.Field.Tag.Get(FlagTagUsage); usage != "" {
			usage = " — " + usage
		}

		if isSecret(field) {
			usage += " (secret)"
		} else if tmp := field.Field.Tag.Get(defaultTagName); tmp != "" {
			usage += fmt.Sprintf(" (default: %s)", tmp)
		}

		output = append(output, envUsage{Usage: usage, Name: name, Type: field.Value.Type().String()})
	}

	var options envUsageOptions
	for _, opt := range opts {
		opt(&options)
	}

	prefix := envPrefix(options.prefix)

	out := make([]string, 0, len(output))
	for _, item := range output {
		out = append(out, fmt.Sprintf("  - '%s%s' <%s>%s", prefix, item.Name, item.Type, item.Usage))
	}

	return fmt.Sprintf("Environment variables:\n%s", strings.Join(out, "\n"))
}

// envPrefix returns the prefix of environment variable names with exactly one
// trailing "_", so "APP" and "APP_" both mean APP_NAME, and APPLE_X is not included.
func envPrefix(prefix string) string {
	if prefix = strings.TrimSuffix(prefix, envDelimiter); prefix == "" {
		return ""
	}

	return prefix + envDelimiter
}

// withSections returns a new zero value of the struct dest points to, with every
// section (a pointer to a struct) allocated, so that the help lists the variables
// inside sections too: env creates them. dest itself is not changed. A section of
// a type that contains itself is left nil. Anything but a pointer to a struct is
// returned as is, for ReflectFieldsOf to report.
func withSections(dest any) any {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return dest
	}

	template := reflect.New(v.Elem().Type())
	allocateSections(template.Elem(), []reflect.Type{v.Elem().Type()})

	return template.Interface()
}

func allocateSections(v reflect.Value, path []reflect.Type) {
	for field, value := range v.Fields() {
		elem := field.Type
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		switch {
		case elem.Kind() != reflect.Struct || textStruct(elem):
		case field.Type.Kind() == reflect.Struct:
			allocateSections(value, path)
		case value.CanSet() && !slices.Contains(path, elem):
			value.Set(reflect.New(elem))
			allocateSections(value.Elem(), append(slices.Clip(path), elem))
		}
	}
}

// wrapUsageLoader wraps the provided loader function to add additional functionality
// for handling help flags and printing environment variable usage. It ensures that when
// the help flag (`--help`) is provided, the program prints the environment variable usage
// and exits gracefully. This function is typically used to augment the configuration loading
// mechanism.
//
// The wrapped handler function behaves as follows:
//  1. If the handler returns an error equal to `pflag.ErrHelp`, it prints environment variable
//     usage (with an optional prefix) and terminates the program.
//  2. If any other error occurs during the handler execution, the error is returned.
//  3. On successful execution of the handler without errors, it proceeds normally.
//
// Params:
// - l: The *loader, which contains the `EnvPrefix`, the output and the exit function.
// - handler: The function responsible for loading the configuration (e.g., from flags or envs).
//
// Returns:
// - A new function that wraps the original handler with additional error handling and help output logic.
func wrapUsageLoader(l *loader, handler func(any) error) func(any) error {
	return func(v any) error {
		// Attempt to load the configuration
		if err := handler(v); errors.Is(err, pflag.ErrHelp) {
			// If the error is the help flag, print environment variable usage
			writeln(l.buffer)
			writeln(l.buffer, UsageOfEnvs(v, EnvUsageWithPrefix(l.EnvPrefix)))

			// Handle program exit for tests or production
			l.exit(0)

			// allows tests to proceed without terminating the program
			return ErrTestExit
		} else if err != nil {
			// Return any other errors from the loader
			return err
		}

		return nil
	}
}

func writeln(out io.Writer, a ...any) {
	_, _ = fmt.Fprintln(out, a...)
}

// PrepareEnvs prepares a map from the given environment variable slice.
// It filters and parses the environment variables based on the provided prefix.
// The resulting map has a nested structure based on the environment variable names,
// using the specified delimiter for nesting.
func PrepareEnvs(envs []string, prefix string) map[string]any {
	prefix = envPrefix(prefix)

	out := make(map[string]any, len(envs))
	for _, env := range envs {
		var ok bool
		if env, ok = strings.CutPrefix(env, prefix); !ok {
			continue
		}

		parts := strings.SplitN(env, envPairDelim, 2)
		if len(parts) != 2 {
			continue
		}

		keys := strings.Split(parts[0], envDelimiter)

		// Insert into a map with the correct nesting
		insertIntoMap(out, keys, parts[1])
	}

	return out
}

// insertIntoMap inserts the value into the map with the specified keys.
// The keys define the nesting level of the map. If the keys are exhausted, the value is set.
// This function creates nested maps as needed to match the structure defined by the keys.
//
// A name can be a value and the start of other names at once (DB and DB_HOST):
// then it holds the nested map and keeps its own value under the "" key, whatever
// the order of the variables. envClashHook picks what the field needs.
func insertIntoMap(m map[string]any, keys []string, value any) {
	setLeaf(m, strings.Join(keys, envDelimiter), value)

	if len(keys) == 1 {
		return
	}

	nested, ok := m[keys[0]].(map[string]any)
	if !ok {
		nested = make(map[string]any)
		if own, exists := m[keys[0]]; exists {
			nested[""] = own
		}

		m[keys[0]] = nested
	}

	insertIntoMap(nested, keys[1:], value)
}

// setLeaf sets the value of a name, next to the nested names under it, if any.
func setLeaf(m map[string]any, key string, value any) {
	if nested, ok := m[key].(map[string]any); ok {
		nested[""] = value

		return
	}

	m[key] = value
}

// envClashHook resolves a name that is both a variable and the start of other ones
// (see insertIntoMap) before a struct or a map is decoded: a field of a struct or
// map type takes the nested names and ignores the variable (DB and DB_HOST), any
// other field takes the variable and ignores the nested names, which belong to
// something else (NAME and NAME_SUFFIX). Without it the whole loading failed.
func envClashHook(_, target reflect.Type, data any) (any, error) {
	tree, ok := data.(map[string]any)
	if !ok {
		return data, nil
	}

	switch target.Kind() {
	case reflect.Struct:
		return resolveClashes(maps.Clone(tree), target), nil
	case reflect.Map:
		if _, own := tree[""]; own {
			tree = maps.Clone(tree)
			delete(tree, "")
		}
	}

	return tree, nil
}

// resolveClashes fixes the values of the fields of t in tree, see envClashHook.
func resolveClashes(tree map[string]any, t reflect.Type) map[string]any {
	for field := range t.Fields() {
		parts := strings.Split(field.Tag.Get(envTag), ",")

		kind := field.Type
		for kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}

		container := kind.Kind() == reflect.Map || kind.Kind() == reflect.Struct && !textStruct(kind)

		switch key, found := lookupKey(tree, parts[0]); {
		case parts[0] == "" && container && (field.Anonymous || slices.Contains(parts[1:], envInlineOption)):
			resolveClashes(tree, kind) // inlined: its fields read from this level
		case !found:
		case container:
			if _, isTree := tree[key].(map[string]any); !isTree {
				delete(tree, key) // DB=x next to the struct DB
			}
		default:
			if nested, isTree := tree[key].(map[string]any); isTree {
				tree[key] = nested[""] // NAME next to NAME_SUFFIX
				if tree[key] == nil {
					delete(tree, key) // NAME_SUFFIX alone
				}
			}
		}
	}

	return tree
}

// lookupKey finds the key of a field as mapstructure does: exact, else case-insensitive.
func lookupKey(tree map[string]any, name string) (string, bool) {
	if _, ok := tree[name]; ok || name == "" {
		return name, ok
	}

	for key := range tree {
		if strings.EqualFold(key, name) {
			return key, true
		}
	}

	return "", false
}

// sectionDefaultsHook gives a section a source is about to create, a nil pointer to a
// struct, the values of its `default` tags first, as the root of the config has them;
// the source then overrides what it sets. A section no source sets stays nil.
func sectionDefaultsHook(from, to reflect.Value) (any, error) {
	if to.Kind() == reflect.Pointer && to.IsNil() && to.CanSet() &&
		to.Type().Elem().Kind() == reflect.Struct && !textStruct(to.Type().Elem()) {
		section := reflect.New(to.Type().Elem())
		if err := SetDefaults(section.Interface()); err != nil {
			return nil, err
		}

		to.Set(section)
	}

	return from.Interface(), nil
}

// textStruct reports whether a struct is read from a string: net.IPNet, time.Time
// or any other encoding.TextUnmarshaler.
func textStruct(t reflect.Type) bool {
	return t == ipNetType || reflect.PointerTo(t).Implements(unmarshalType)
}

// decodeHooks converts the provided data into the target type using type-specific parsing.
// It supports basic types, time.Duration, IP-related types and encoding.TextUnmarshaler.
// It returns the parsed value or an error if the conversion fails.
//
//nolint:ireturn
func decodeHooks() mapstructure.DecodeHookFunc {
	decoders := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToBasicTypeHookFunc())

	return mapstructure.ComposeDecodeHookFunc(
		jsonNumberHook,
		mapstructure.TextUnmarshallerHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToBasicTypeHookFunc(),

		// decode net-values
		mapstructure.StringToIPHookFunc(),
		mapstructure.StringToIPNetHookFunc(),

		// slice types
		func(
			f reflect.Value,
			t reflect.Value,
		) (any, error) {
			if f.Kind() != reflect.String {
				return f.Interface(), nil
			}
			if t.Kind() != reflect.Slice {
				return f.Interface(), nil
			}

			var str string
			if in, ok := f.Interface().(string); ok {
				str = in
			} else {
				str = f.String()
			}

			if str == "" {
				return nil, nil
			}

			raw := strings.Split(str, ",")
			tmp := reflect.MakeSlice(t.Type(), len(raw), len(raw))
			for i := range raw {
				from := reflect.ValueOf(raw[i])
				to := reflect.New(t.Type().Elem()).Elem()

				val, err := mapstructure.DecodeHookExec(
					decoders, from, to)

				if err != nil {
					return nil, err
				}

				tmp.Index(i).Set(reflect.ValueOf(val).Convert(t.Type().Elem()))
			}

			return tmp.Interface(), nil
		})
}

// Tag options that inline a named struct field into its parent: `env:",squash"`
// for environment variables, `yaml:",inline"` (and json/toml alike) for files.
// Embedded structs are inlined without any option.
const (
	envInlineOption  = "squash"
	fileInlineOption = "inline"
)

// decodeOptions describe how a generic map is decoded into a struct.
type decodeOptions struct {
	tag      string // struct tag with the key names
	inline   string // tag option that inlines a named struct field
	untagged bool   // match fields without the tag by their name (files); env skips them
	strict   bool   // keys that match no field are an error (files only: env holds every variable)
	env      bool   // resolve a name that is both a variable and the start of others (envClashHook)
}

func decodeMapToStruct(dest any, from map[string]any, tag string) error {
	return decodeMap(dest, from, decodeOptions{tag: tag, inline: envInlineOption})
}

// decodeMap is the single decoder behind environment variables, files and
// WithDefaults, so all of them treat embedding and value types the same way.
func decodeMap(dest any, from map[string]any, options decodeOptions) error {
	conf := &mapstructure.DecoderConfig{
		Result:               dest,
		TagName:              options.tag,
		Squash:               true,
		SquashTagOption:      options.inline,
		IgnoreUntaggedFields: !options.untagged,
		ErrorUnused:          options.strict,
		DecodeHook:           mapstructure.ComposeDecodeHookFunc(sectionDefaultsHook, decodeHooks()),
	}

	if options.env {
		conf.DecodeHook = mapstructure.ComposeDecodeHookFunc(envClashHook, conf.DecodeHook)
	}

	dec, err := mapstructure.NewDecoder(conf)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPrepareDecoder, err)
	}

	if err = dec.Decode(from); err != nil {
		// mapstructure prefixes the list of errors with a multi-line header.
		if inner := errors.Unwrap(err); inner != nil {
			err = inner
		}

		return fmt.Errorf("%w: %w", ErrDecode, singleLine{err})
	}

	return nil
}

// LoadEnvs decodes the provided environment variables map into the destination object.
// It uses mapstructure to map the environment variables to the fields of the destination
// object based on the "env" tag. It returns an error if decoding fails.
func LoadEnvs(envs map[string]any, dest any) error {
	return decodeMap(dest, envs, decodeOptions{tag: envTag, inline: envInlineOption, env: true})
}

// jsonNumberHook turns json.Number from JSON files into a Go number before the
// other hooks run: they expect a plain string or number and would misread it.
func jsonNumberHook(_, _ reflect.Type, data any) (any, error) {
	number, ok := data.(json.Number)
	if !ok {
		return data, nil
	}

	if v, err := number.Int64(); err == nil {
		return v, nil
	}

	if v, err := strconv.ParseUint(number.String(), 10, 64); err == nil {
		return v, nil
	}

	return number.Float64()
}
