package gonfig

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
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
// nolint:funlen,gocognit
func UsageOfEnvs(dest any, opts ...EnvUsageOption) string {
	output := make([]envUsage, 0)
	exists := make(map[string]struct{})
	for field, err := range ReflectFieldsOf(dest, ReflectOptions{CanSet: True()}) {
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

		if tmp := field.Field.Tag.Get(defaultTagName); tmp != "" {
			usage += fmt.Sprintf(" (default: %s)", tmp)
		}

		output = append(output, envUsage{Usage: usage, Name: name, Type: field.Value.Type().String()})
	}

	var options envUsageOptions
	for _, opt := range opts {
		opt(&options)
	}

	var prefix string
	if options.prefix != "" {
		prefix = options.prefix + envDelimiter
	}

	out := make([]string, 0, len(output))
	for _, item := range output {
		out = append(out, fmt.Sprintf("  - '%s%s' <%s>%s", prefix, item.Name, item.Type, item.Usage))
	}

	return fmt.Sprintf("Environment variables:\n%s", strings.Join(out, "\n"))
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
// - svc: The *loader, which contains the `EnvPrefix` and an optional custom exit function.
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

func writeln(out io.Writer, a ...interface{}) {
	_, _ = fmt.Fprintln(out, a...)
}

// PrepareEnvs prepares a map from the given environment variable slice.
// It filters and parses the environment variables based on the provided prefix.
// The resulting map has a nested structure based on the environment variable names,
// using the specified delimiter for nesting.
func PrepareEnvs(envs []string, prefix string) map[string]any {
	out := make(map[string]any, len(envs))
	for _, env := range envs {
		if prefix != "" && !strings.HasPrefix(env, prefix) {
			continue
		}

		if prefix != "" {
			env = strings.TrimPrefix(env, prefix+envDelimiter)
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
func insertIntoMap(m map[string]any, keys []string, value any) {
	if len(keys) == 1 {
		m[keys[0]] = value
		return
	}

	m[strings.Join(keys, envDelimiter)] = value

	// Create a nested map if it does not exist
	if _, ok := m[keys[0]]; !ok {
		m[keys[0]] = make(map[string]any)
	}

	if nestedMap, ok := m[keys[0]].(map[string]any); ok {
		insertIntoMap(nestedMap, keys[1:], value)
	}
}

// decodeEnv converts the provided data into the target type using type-specific parsing.
// It supports basic types, time.Duration, and IP-related types. It returns the parsed value
// or an error if the conversion fails.
//
// nolint:ireturn
func decodeEnv() mapstructure.DecodeHookFunc {
	decoders := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToBasicTypeHookFunc())

	return mapstructure.ComposeDecodeHookFunc(
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
			if in, ok := f.Interface().(string); !ok {
				return f.Interface(), nil
			} else if str = in; str == "" {
				return nil, nil
			}

			raw := strings.Split(str, ",")
			tmp := reflect.MakeSlice(t.Type(), len(raw), len(raw))
			for i := 0; i < len(raw); i++ {
				from := reflect.ValueOf(raw[i])
				to := reflect.New(t.Type().Elem()).Elem()

				val, err := mapstructure.DecodeHookExec(
					decoders, from, to)

				if err != nil {
					return nil, err
				}

				tmp.Index(i).Set(reflect.ValueOf(val))
			}

			return tmp.Interface(), nil
		})
}

func decodeMapToStruct(dest any, from map[string]any, tag string) error {
	conf := &mapstructure.DecoderConfig{
		Result:               dest,
		TagName:              tag,
		Squash:               true,
		SquashTagOption:      "squash",
		IgnoreUntaggedFields: true,
		DecodeHook:           decodeEnv()}
	if dec, err := mapstructure.NewDecoder(conf); err != nil {
		return errors.Join(ErrPrepareDecoder, err)
	} else if err = dec.Decode(from); err != nil {
		return errors.Join(ErrDecode, err)
	}

	return nil
}

// LoadEnvs decodes the provided environment variables map into the destination object.
// It uses mapstructure to map the environment variables to the fields of the destination
// object based on the "env" tag. It returns an error if decoding fails.
func LoadEnvs(envs map[string]any, dest any) error {
	return decodeMapToStruct(dest, envs, envTag)
}
