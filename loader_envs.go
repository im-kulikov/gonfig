package gonfig

import (
	"errors"
	"fmt"
	"os"
	"reflect"
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

const (
	envPairDelim = "=" // Delimiter used to separate environment variable names from their values.
	envDelimiter = "_" // Delimiter used to separate parts of the environment variable name.

	envTag = "env"
)

// newEnvLoader creates a new parser that loads configuration from environment variables.
// It uses the provided environment variable slice and prefix to populate the configuration.
// Returns a Parser that processes environment variables with the specified prefix.
func newEnvLoader(envs []string, prefix string) Parser {
	return &parserFunc{name: ParserEnv, call: func(v interface{}) error {
		return LoadEnvs(PrepareEnvs(envs, prefix), v)
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
//     The struct's fields must use the "env" tag to define environment variable names
//     and the "usage" tag to describe their purpose.
//   - opts: Optional EnvUsageOption(s) to configure behavior, such as adding a prefix to environment variable names.
//
// Returns:
//   - A string describing the environment variables and their usage, or an empty string if the input is not valid.
//
// The function ensures that the input is a pointer to a struct. It traverses the struct fields,
// generating usage information based on the tags. If a struct field is another struct, it recurses
// into the nested fields.
func UsageOfEnvs(dest any, opts ...EnvUsageOption) string {
	val := reflect.ValueOf(dest)

	// Ensure that the input is a pointer to a struct.
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return ""
	}

	var options envUsageOptions
	for _, opt := range opts {
		opt(&options)
	}

	var result string
	if result = prepareEnvUsage(val.Elem(), options.prefix); result == "" {
		return ""
	}

	return fmt.Sprintf("Environment variables:\n%s", result)
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
func wrapUsageLoader(svc *loader, handler func(v any) error) func(v any) error {
	return func(v any) error {
		// Attempt to load the configuration
		if err := handler(v); errors.Is(err, pflag.ErrHelp) {
			// If the error is the help flag, print environment variable usage
			fmt.Println()
			fmt.Println(UsageOfEnvs(v, EnvUsageWithPrefix(svc.EnvPrefix)))

			// Handle program exit for tests or production
			if svc.exit != nil {
				svc.exit(0)
				return nil // allows tests to proceed without terminating the program
			}

			// If no custom exit function is provided, exit the program
			os.Exit(0)
		} else if err != nil {
			// Return any other errors from the loader
			return err
		}

		return nil
	}
}

// prepareEnvUsage recursively processes the fields of a struct to generate environment variable
// usage information, including descriptions and default values (if provided).
//
// Parameters:
//   - v: The reflect.Value representing the struct to process.
//   - prefix: A string prefix to apply to environment variable names.
//
// Returns:
//   - A string describing the environment variables for the given struct, including the name, type,
//     and usage description for each variable.
//
// The function iterates through each field of the struct, checking for the "env" tag to determine the
// environment variable name. It uses the "usage" tag to describe the variable's purpose, and adds any
// default values if specified in the "default" tag. For nested structs, the function recurses to process
// the fields of the embedded structure.
func prepareEnvUsage(v reflect.Value, prefix string) string {
	t := v.Type()
	if prefix != "" {
		prefix += envPairDelim
	}

	out := make([]string, 0, t.NumField())
	for i := 0; i < v.NumField(); i++ {
		var name string
		if name = t.Field(i).Tag.Get(envTag); name == "" || name == "-" {
			continue
		}

		var usage string
		if usage = t.Field(i).Tag.Get(FlagTagUsage); usage != "" {
			usage = " — " + usage
		}

		if defaults := t.Field(i).Tag.Get("default"); defaults != "" {
			usage += fmt.Sprintf(" (default %s)", defaults)
		}

		if t.Field(i).Type.Kind() != reflect.Struct {
			out = append(out, fmt.Sprintf("  - '%s' <%s>%s", prefix+name, t.Field(i).Type, usage))

			continue
		}

		out = append(out, prepareEnvUsage(v.Field(i), prefix+name))
	}

	return strings.Join(out, "\n")
}

// PrepareEnvs prepares a map from the given environment variable slice.
// It filters and parses the environment variables based on the provided prefix.
// The resulting map has a nested structure based on the environment variable names,
// using the specified delimiter for nesting.
func PrepareEnvs(envs []string, prefix string) map[string]interface{} {
	out := make(map[string]interface{}, len(envs))
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

		// Insert into map with the correct nesting
		insertIntoMap(out, keys, parts[1])
	}

	return out
}

// insertIntoMap inserts the value into the map with the specified keys.
// The keys define the nesting level of the map. If the keys are exhausted, the value is set.
// This function creates nested maps as needed to match the structure defined by the keys.
func insertIntoMap(m map[string]interface{}, keys []string, value interface{}) {
	if len(keys) == 1 {
		m[keys[0]] = value
		return
	}

	m[strings.Join(keys, envDelimiter)] = value

	// Create a nested map if it does not exist
	if _, ok := m[keys[0]]; !ok {
		m[keys[0]] = make(map[string]interface{})
	}

	if nestedMap, ok := m[keys[0]].(map[string]interface{}); ok {
		insertIntoMap(nestedMap, keys[1:], value)
	}
}

// decodeEnv converts the provided data into the target type using type-specific parsing.
// It supports basic types, time.Duration, and IP-related types. It returns the parsed value
// or an error if the conversion fails.
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
		) (interface{}, error) {
			if f.Kind() != reflect.String {
				return f.Interface(), nil
			}
			if t.Kind() != reflect.Slice {
				return f.Interface(), nil
			}

			raw := strings.Split(f.Interface().(string), ",")
			tmp := reflect.MakeSlice(t.Type(), len(raw), len(raw))
			for i := 0; i < len(raw); i++ {
				from := reflect.ValueOf(raw[i])
				to := reflect.New(t.Type().Elem()).Elem()

				val, err := mapstructure.DecodeHookExec(
					decoders, from, to)

				if err != nil {
					return f.Interface(), nil
				}

				tmp = reflect.Append(tmp, reflect.ValueOf(val))
			}

			return tmp.Interface(), nil
		})

}

// LoadEnvs decodes the provided environment variables map into the destination object.
// It uses mapstructure to map the environment variables to the fields of the destination
// object based on the "env" tag. It returns an error if decoding fails.
func LoadEnvs(envs map[string]interface{}, dest any) error {
	conf := &mapstructure.DecoderConfig{
		Result:     dest,
		TagName:    envTag,
		DecodeHook: decodeEnv()}
	if dec, err := mapstructure.NewDecoder(conf); err != nil {
		return fmt.Errorf("could not prepare encoder: %w", err)
	} else if err = dec.Decode(envs); err != nil {
		return fmt.Errorf("could not decode: %w", err)
	}

	return nil
}
