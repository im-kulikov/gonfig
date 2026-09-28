package gonfig

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type printSettings struct {
	PrintConfigFlag
	DefaultConfigFlag

	Host     string `yaml:"host"     json:"host"     env:"HOST"     default:"localhost" usage:"server host"`
	Port     int    `yaml:"port"     json:"port"     env:"PORT"     default:"8080"`
	Token    string `yaml:"token"    json:"token"    env:"TOKEN"    secret:"true"`
	Required string `yaml:"required" json:"required" env:"REQUIRED" required:"true"`
}

// printConfig runs Load with the arguments and returns what it printed and the exit code.
func printConfig(t *testing.T, args []string, envs []string, options ...LoaderOption) (string, int) {
	t.Helper()

	var (
		out  bytes.Buffer
		code = -1
		v    printSettings
	)

	err := Load(&v, append(options,
		WithCustomOutput(&out),
		WithCustomExit(func(c int) { code = c }),
		WithConfig(func(c *Config) {
			c.Args, c.Envs, c.EnvPrefix = args, envs, "APP"
		}))...)
	require.ErrorIs(t, err, ErrTestExit)

	return out.String(), code
}

func TestPrintConfig(t *testing.T) {
	envs := []string{"APP_PORT=9090", "APP_TOKEN=hunter2"}

	t.Run("yaml without a file loader, before required fields are checked", func(t *testing.T) {
		out, code := printConfig(t, []string{"--print-config"}, envs)

		assert.Equal(t, 0, code)
		assert.Equal(t, `# server host (env: APP_HOST, default: localhost)
host: localhost
# env: APP_PORT, default: 8080
port: 9090
# env: APP_TOKEN, secret
token: ""
# env: APP_REQUIRED
required: ""
`, out)
	})

	t.Run("the format of the file loader", func(t *testing.T) {
		out, _ := printConfig(t, []string{"--print-config"}, envs, WithJSONLoader())
		assert.JSONEq(t, `{"host": "localhost", "port": 9090, "token": "", "required": ""}`, out)
	})

	t.Run("the format of the file from --config", func(t *testing.T) {
		config := writeTempFile(t, "app.toml", "port = 7070\n")
		out, _ := printConfig(t, []string{"--config", config, "--print-config"}, nil, WithYAMLLoader(), WithTOMLLoader())
		assert.Contains(t, out, "Port = 7070\n", "TOML, and without toml tags the keys are the field names")
	})

	t.Run("an explicit format", func(t *testing.T) {
		out, _ := printConfig(t, []string{"--print-config=env"}, envs, WithJSONLoader())
		assert.Contains(t, out, "APP_PORT=9090\n")
		assert.NotContains(t, out, "hunter2")
	})

	t.Run("listed in --help", func(t *testing.T) {
		out, _ := printConfig(t, []string{"--help"}, nil, WithTOMLLoader())
		assert.Contains(t, out, `--print-config format[="toml"]`)
	})
}

func TestPrintConfig_Errors(t *testing.T) {
	for _, arg := range []string{"--print-config=xml", "--print-config="} {
		var v printSettings
		err := Load(&v, WithConfig(func(c *Config) {
			c.Args, c.Envs = []string{arg}, []string{}
		}))
		require.ErrorIs(t, err, ErrUnknownFormat, arg)
	}

	var conflict struct {
		PrintConfigFlag

		Print string `flag:"print-config"`
	}
	require.ErrorIs(t, Load(&conflict, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{}, []string{}
	})), ErrFlagRedefined)

	var without struct {
		Host string `flag:"host"`
	}
	require.Error(t, Load(&without, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{"--print-config"}, []string{}
	})), "the flag exists only with PrintConfigFlag")
}

// The output of --print-config is a config file the same application loads back.
func TestPrintConfig_LoadsBack(t *testing.T) {
	for _, loader := range []LoaderOption{WithYAMLLoader(), WithJSONLoader(), WithTOMLLoader()} {
		out, _ := printConfig(t, []string{"--print-config"}, []string{"APP_PORT=9090", "APP_REQUIRED=set"}, loader)

		var got printSettings
		require.NoError(t, Load(&got, loader, WithConfig(func(c *Config) {
			c.Args, c.Envs = []string{"--config", writeTempFile(t, "config", out)}, []string{}
		})), out)
		assert.Equal(t, printSettings{Host: "localhost", Port: 9090, Required: "set"}, got)
	}
}

func TestPrintConfigFlag_IsPrintConfig(t *testing.T) {
	PrintConfigFlag{}.IsPrintConfig()

	type nested struct{ PrintConfigFlag }
	assert.True(t, containsMarker(&struct{ nested }{}, printMarkerType))
	assert.False(t, containsMarker(&struct{ DefaultConfigFlag }{}, printMarkerType))
}
