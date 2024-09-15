package gonfig

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

const (
	envPairDelim = "=" // Delimiter used to separate environment variable names from their values.
	envDelimiter = "_" // Delimiter used to separate parts of the environment variable name.
)

// newEnvLoader creates a new parser that loads configuration from environment variables.
// It uses the provided environment variable slice and prefix to populate the configuration.
// Returns a Parser that processes environment variables with the specified prefix.
func newEnvLoader(envs []string, prefix string) Parser {
	return &parserFunc{name: ParserEnv, call: func(v interface{}) error {
		return LoadEnvs(PrepareEnvs(envs, prefix), v)
	}}
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
		TagName:    "env",
		DecodeHook: decodeEnv()}
	if dec, err := mapstructure.NewDecoder(conf); err != nil {
		return fmt.Errorf("could not prepare encoder: %w", err)
	} else if err = dec.Decode(envs); err != nil {
		return fmt.Errorf("could not decode: %w", err)
	}

	return nil
}
