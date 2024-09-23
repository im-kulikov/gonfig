package gonfig_test

import (
	"encoding/json"
	"fmt"
	"io"
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
	IntField    int    `env:"INT_VALUE"`
}

type TestLoaderConfig struct {
	StringField string        `default:"default_value" flag:"string-field"`
	IntField    int           `env:"INT_VALUE" flag:"int-value" usage:"int value"`
	JSONConfig  string        `flag:"json-config,config:true"`
	Timeout     time.Duration `env:"TIMEOUT" usage:"timeout value" default:"30s"`

	TestInnerLoaderConfig `json:",inline" env:",squash"`

	Embed struct {
		IntField int `env:"INT_FIELD" default:"1" usage:"int field"`
	} `env:"EMBED"`
}

const (
	parserJSONType   gonfig.ParserType = "json"
	parserCustomType gonfig.ParserType = "custom"
)

func testCustomOptions() []gonfig.LoaderOption {
	return []gonfig.LoaderOption{
		gonfig.WithCustomParser(nil),
		gonfig.WithCustomExit(func(int) {}),
		gonfig.WithCustomParser(gonfig.NewCustomParser(parserCustomType, customLoad)),
		gonfig.WithCustomParser(new(JSONParser)),
	}
}

type JSONParser struct {
	config string
}

func (p *JSONParser) SetConfigPath(path string) { p.config = path }

func (p *JSONParser) Type() gonfig.ParserType { return parserJSONType }

func (p *JSONParser) Load(v any) error {
	if p.config == "" {
		return nil
	}

	file, err := os.OpenFile(p.config, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	if err = json.NewDecoder(file).Decode(v); err != nil {
		return fmt.Errorf("could not decode json-field: %w", err)
	}

	return file.Close()
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
	return gonfig.Config{Args: args, Envs: envs, EnvPrefix: "TEST"}, gonfig.WithOptions(testCustomOptions)
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
		"TEST_INT_VALUE=" + strconv.Itoa(math.MaxInt),
		"TEST_TIMEOUT=15s"}

	var config TestLoaderConfig
	require.NoError(t, gonfig.New(testLoaderOptions(args, envs)).Load(&config))

	require.Equal(t, "custom-value", config.StringField, "string-field should be set")
	require.Equal(t, "custom-value", config.JSONField, "json-field should be set")
	require.Equal(t, math.MaxInt, config.IntField, "int-value should be set")
	require.Equal(t, math.MaxInt, config.CustomField, "custom-field should be set")
	require.Equal(t, time.Second*15, config.Timeout, "timeout field should be set")
}

func TestUsage(t *testing.T) {
	buf, out, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	defer func() { os.Stdout = old }()

	os.Stdout = out

	var (
		conf TestLoaderConfig
		envs []string
		args = []string{"--help"}
	)
	require.NoError(t, gonfig.New(testLoaderOptions(args, envs)).Load(&conf))

	require.NoError(t, out.Close())

	expectedOutput := `Usage of flags:
      --int-value int         int value
      --json-config string    
      --string-field string    (default "default_value")

Environment variables:
  - 'TEST_INT_VALUE' <int> — int value
  - 'TEST_TIMEOUT' <time.Duration> — timeout value (default: 30s)
  - 'TEST_EMBED_INT_FIELD' <int> — int field (default: 1)
`

	tmp, err := io.ReadAll(buf)
	require.NoError(t, err)
	require.NoError(t, buf.Close())
	require.Equal(t, expectedOutput, string(tmp))
}

func TestLoader(t *testing.T) {
	require.NoError(t, gonfig.New(gonfig.Config{}).Load(&struct{}{}))

	require.NoError(t, gonfig.New(gonfig.Config{},
		gonfig.WithOptions(func() []gonfig.LoaderOption {
			return []gonfig.LoaderOption{
				gonfig.WithOptions([]gonfig.LoaderOption{}),
				gonfig.WithCustomParserInit(func(gonfig.Config) (gonfig.Parser, error) {
					return nil, nil
				}),
			}
		})).Load(&struct{}{}))

	require.EqualError(t, gonfig.New(gonfig.Config{}, gonfig.WithOptions(nil)).Load(&struct{}{}),
		"gonfig: could not init option: invalid options type: <nil>")

	require.EqualError(t, gonfig.New(gonfig.Config{},
		gonfig.WithOptions(func() []gonfig.LoaderOption {
			return []gonfig.LoaderOption{
				gonfig.WithOptions([]gonfig.LoaderOption{}),
				gonfig.WithCustomParserInit(func(gonfig.Config) (gonfig.Parser, error) {
					return nil, gonfig.ErrExpectStruct
				}),
			}
		})).Load(&struct{}{}), "gonfig: could not init option: could not init options: expect struct field")

}
