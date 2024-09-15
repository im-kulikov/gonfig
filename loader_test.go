package gonfig_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
)

type TestInnerLoaderConfig struct {
	CustomField int    `custom:"custom-field"`
	JSONField   string `json:"json-field" default:"default_value"`
}

type TestLoaderConfig struct {
	StringField string        `default:"default_value" flag:"string-field"`
	IntField    int           `env:"INT_VALUE" flag:"int-value"`
	JSONConfig  string        `flag:"json-config"`
	Timeout     time.Duration `env:"TIMEOUT"`

	TestInnerLoaderConfig `json:",inline"`
}

const (
	parserJSONType   gonfig.ParserType = "json"
	parserCustomType gonfig.ParserType = "custom"
)

func testCustomParsers() []gonfig.LoaderOption {
	return []gonfig.LoaderOption{
		gonfig.WithCustomParser(nil),
		gonfig.WithCustomParser(gonfig.NewCustomParser(parserCustomType, customLoad)),
		gonfig.WithCustomParserInit(newTestJSONParser),
	}
}

func newTestJSONParser(gonfig.Config) (gonfig.Parser, error) {
	var filename string

	return gonfig.NewCustomParser(parserJSONType, func(v interface{}) error {
		filename = v.(*TestLoaderConfig).JSONConfig

		file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return err
		}

		if err = json.NewDecoder(file).Decode(v); err != nil {
			return fmt.Errorf("could not decode json-field: %w", err)
		}

		return file.Close()
	}), nil
}

func customLoad(dest interface{}) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("(custom) dest must be a pointer, got %T", dest)
	}

	v = v.Elem()
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("(custom) expected struct type, got %T", dest)
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		if field.Kind() == reflect.Struct {
			return customLoad(field.Addr().Interface())
		}

		if t.Field(i).Tag.Get("custom") == "" {
			continue
		}

		field.SetInt(math.MaxInt)
	}

	return nil
}

func testLoaderOptions(args, envs []string) (gonfig.Config, gonfig.LoaderOption) {
	return gonfig.Config{
			Args: args,
			Envs: envs,

			LoaderOrder: []gonfig.ParserType{gonfig.ParserDefaults, gonfig.ParserEnv, gonfig.ParserFlags, parserJSONType, parserCustomType}},
		gonfig.WithOptions(testCustomParsers)
}

func TestNew(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "custom.json")
	require.NoError(t, err)

	defer func() { require.NoError(t, os.Remove(file.Name())) }()

	require.NoError(t, json.NewEncoder(file).Encode(map[string]interface{}{
		"json-field": "custom-value",
	}))

	require.NoError(t, file.Close())

	args := []string{
		"--string-field", "custom-value",
		"--json-config", file.Name()}

	envs := []string{
		"INT_VALUE=" + strconv.Itoa(math.MaxInt),
		"TIMEOUT=15s"}

	var config TestLoaderConfig
	require.NoError(t, gonfig.New(testLoaderOptions(args, envs)).Load(&config))

	require.Equal(t, "custom-value", config.StringField, "string-field should be set")
	require.Equal(t, "custom-value", config.JSONField, "json-field should be set")
	require.Equal(t, math.MaxInt, config.IntField, "int-value should be set")
	require.Equal(t, math.MaxInt, config.CustomField, "custom-field should be set")
	require.Equal(t, time.Second*15, config.Timeout, "timeout field should be set")
}
