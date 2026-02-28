package gonfig

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

type ConfigWithValidator struct {
	mock.Mock

	SomeField string `json:"some-field" env:"SOME_FIELD" required:"true"`
}

func (c *ConfigWithValidator) Validate() error {
	return c.Called().Error(0)
}

func TestWithValidate(t *testing.T) {
	cases := []struct {
		name string
		envs []string
		err  error
	}{
		{name: "success", envs: []string{"SOME_FIELD=OK"}},
		{name: "failure", envs: []string{"SOME_FIELD=FAIL"}, err: errors.New("test")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ConfigWithValidator
			cfg.Test(t)

			defer cfg.On("Validate").Return(tc.err).Unset()
			require.ErrorIs(t, Load(&cfg, WithConfig(func(config *Config) { config.Envs = tc.envs })), tc.err)

			cfg.AssertExpectations(t)
		})
	}
}

func TestCustomLoaders(t *testing.T) {
	args := make([]string, 0, 8)
	args = append(args,
		"--string-field", "flag-value",
		"--int-field", "80",
		"--embed-float-field", "3.18")

	file, err := os.CreateTemp(t.TempDir(), "test.json")
	require.NoError(t, err)

	require.NoError(t, json.NewEncoder(file).Encode(CustomLoaderConfig{
		FieldString: "json-value",
		FieldInt:    19,
		StructField: NestedCustomLoaderConfig{String: "value-from-json", Int: 94, Float: 1.85},
	}))

	require.NoError(t, file.Close())

	args = append(args, "--config", file.Name())

	var cfg CustomLoaderConfig
	require.NoError(t, New(Config{Args: args}, WithJSONLoader()).Load(&cfg))

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

		require.EqualError(t, New(Config{Args: []string{
			"--config", "path/to/file"}}).Load(&cfg),
			"gonfig: could not load: (config-path) expect string, got \"int\"")
	}

	{
		var cfg struct {
			Field int `flag:"field,short:ff"`
		}

		require.EqualError(t, New(Config{Args: []string{
			"--config", "path/to/file"}}).Load(&cfg),
			"gonfig: could not load: (flags) shorthand is more than one ASCII character \"ff\"")
	}
}
