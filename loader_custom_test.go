package gonfig_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
)

type NestedCustomLoaderConfig struct {
	String string  `json:"string-field" flag:"embed-string-field" default:"default-value"`
	Int    int     `json:"int-field" flag:"embed-int-field" default:"-2"`
	Float  float64 `json:"float-field" flag:"embed-float-field" default:"3.14"`
}

type CustomLoaderConfig struct {
	FieldString string                   `json:"string-field" flag:"string-field" default:"default-value"`
	FieldInt    int                      `json:"int-field" flag:"int-field" default:"-1"`
	Config      string                   `json:"-" flag:"config,config:true,short:c"`
	StructField NestedCustomLoaderConfig `json:"struct-field"`
}

type customJSONParser struct {
	path string
}

func (c *customJSONParser) SetConfigPath(path string) { c.path = path }

func (c *customJSONParser) Load(dest interface{}) error {
	if c.path == "" {
		return nil
	}

	file, err := os.Open(c.path)
	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	return json.NewDecoder(file).Decode(dest)
}

func (*customJSONParser) Type() gonfig.ParserType { return "json" }

func TestCustomLoaders(t *testing.T) {
	args := []string{
		"--string-field", "flag-value",
		"--int-field", "80",
		"--embed-float-field", "3.18"}

	file, err := os.CreateTemp(t.TempDir(), "test.json")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Remove(file.Name()))
	}()

	require.NoError(t, json.NewEncoder(file).Encode(CustomLoaderConfig{
		FieldString: "json-value",
		FieldInt:    19,
		StructField: NestedCustomLoaderConfig{String: "value-from-json", Int: 94, Float: 1.85},
	}))

	require.NoError(t, file.Close())

	args = append(args, "--config", file.Name())

	var cfg CustomLoaderConfig

	require.NoError(t, gonfig.New(gonfig.Config{Args: args},
		gonfig.WithCustomParserInit(func(gonfig.Config) (gonfig.Parser, error) {
			return &customJSONParser{}, nil
		})).Load(&cfg))

	require.Equal(t, CustomLoaderConfig{
		FieldString: "flag-value",
		FieldInt:    80,
		Config:      file.Name(),
		StructField: NestedCustomLoaderConfig{String: "value-from-json", Int: 94, Float: 3.18},
	}, cfg)
}

func TestCustomErrors(t *testing.T) {
	{
		var cfg struct {
			Config int `flag:"config,short:c,config:true"`
		}

		require.EqualError(t, gonfig.New(gonfig.Config{Args: []string{
			"--config", "path/to/file"}}).Load(&cfg),
			"gonfig: could not load: (config-path) expect string, got \"int\"")
	}

	{
		var cfg struct {
			Field int `flag:"field,short:ff"`
		}

		require.EqualError(t, gonfig.New(gonfig.Config{Args: []string{
			"--config", "path/to/file"}}).Load(&cfg),
			"gonfig: could not load: (flags) shorthand is more than one ASCII character \"ff\"")
	}
}
