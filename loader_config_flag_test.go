package gonfig

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ConfigWithDefault struct {
	DefaultConfigFlag
	Foo string `flag:"foo"`
}

func TestDefaultConfigFlag(t *testing.T) {
	t.Run("long-flag", func(t *testing.T) {
		var cfg ConfigWithDefault
		err := Load(&cfg, WithConfig(func(c *Config) {
			c.Args = []string{"--config", "test.json", "--foo", "bar"}
		}))
		require.NoError(t, err)
		require.Equal(t, "bar", cfg.Foo)
	})

	t.Run("short-flag", func(t *testing.T) {
		var cfg ConfigWithDefault
		err := Load(&cfg, WithConfig(func(c *Config) {
			c.Args = []string{"-c", "test.json", "--foo", "bar"}
		}))
		require.NoError(t, err)
		require.Equal(t, "bar", cfg.Foo)
	})

	t.Run("pointer", func(t *testing.T) {
		var cfg struct {
			*DefaultConfigFlag
			Foo string `flag:"foo"`
		}
		err := Load(&cfg, WithConfig(func(c *Config) {
			c.Args = []string{"--config", "test.json", "--foo", "bar"}
		}))
		require.NoError(t, err)
		require.Equal(t, "bar", cfg.Foo)
	})

	t.Run("nested", func(t *testing.T) {
		type Base struct {
			DefaultConfigFlag
		}
		var cfg struct {
			Base
			Foo string `flag:"foo"`
		}
		err := Load(&cfg, WithConfig(func(c *Config) {
			c.Args = []string{"--config", "test.json", "--foo", "bar"}
		}))
		require.NoError(t, err)
		require.Equal(t, "bar", cfg.Foo)
	})

	t.Run("no-default-flag", func(t *testing.T) {
		var cfg struct {
			Foo string `flag:"foo"`
		}
		// This should error because --config is unknown
		err := Load(&cfg, WithConfig(func(c *Config) {
			c.Args = []string{"--config", "test.json", "--foo", "bar"}
		}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown flag: --config")
	})
}

type mockConfigSetter struct {
	path string
}

func (m *mockConfigSetter) Type() ParserType { return "mock" }
func (m *mockConfigSetter) Load(any) error   { return nil }
func (m *mockConfigSetter) SetConfigPath(path string) {
	m.path = path
}

func TestDefaultConfigFlag_Internal(t *testing.T) {
	t.Run("verify-l-config", func(t *testing.T) {
		var cfg ConfigWithDefault
		mock := &mockConfigSetter{}

		l := newLoader(Config{Args: []string{"--config", "config.yaml"}}, &cfg)
		l.setLoaderDefaults()
		// Override l.groups to include our mock and execute
		l.groups["mock"] = mock
		l.orders = []ParserType{"mock"}

		err := l.groups[ParserConfigSet].Load(&cfg)
		require.NoError(t, err)

		err = l.groups[ParserFlags].Load(&cfg)
		require.NoError(t, err)

		// Run mock setter
		if setter, ok := l.groups["mock"].(ParserConfigSetter); ok {
			setter.SetConfigPath(l.config)
		}

		require.Equal(t, "config.yaml", l.config)
		require.Equal(t, "config.yaml", mock.path)
	})
}

func TestConfigField_InvalidFlag(t *testing.T) {
	load := func(v any) error {
		return Load(v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{} }))
	}

	var long struct {
		Path string `flag:"config,short:cfg,config:true"`
	}
	require.ErrorContains(t, load(&long), `(config-path) shorthand is more than one ASCII character "cfg"`)

	var twice struct {
		Path  string `flag:"config,config:true"`
		Other string `flag:"config,config:true"`
	}
	require.ErrorIs(t, load(&twice), ErrFlagRedefined)
}

func TestConfigField_WithoutFlag(t *testing.T) {
	path := writeTempFile(t, "config.yaml", "name: file")

	var v struct {
		Path string `flag:",config:true" env:"CONFIG"`
		Name string `yaml:"name"`
	}
	require.NoError(t, Load(&v, WithYAMLLoader(), WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{}, []string{"CONFIG=" + path}
	})))
	assert.Equal(t, "file", v.Name, "the path of the variable, with no flag")

	var none struct {
		Path  string `flag:",config:true"`
		Other string `flag:",config:true"` // not a flag "" defined twice
		Short string `flag:"-,short:c,config:true"`
	}
	err := Load(&none, WithConfig(func(c *Config) { c.Args, c.Envs = []string{"-c", "x.yaml"}, []string{} }))
	require.ErrorContains(t, err, "unknown shorthand flag: 'c'", "no -c for the path, as no -c in --help")
}

func TestConfigField_PathSources(t *testing.T) {
	type config struct {
		Path string `flag:"config,config:true" env:"CONFIG" default:"default.yaml"`
		Name string `yaml:"name"`
	}

	files := map[string]string{}
	for _, name := range []string{"default.yaml", "env.yaml", "flag.yaml", "code.yaml"} {
		files[name] = writeTempFile(t, name, "name: "+name)
	}

	load := func(args, envs []string, options ...LoaderOption) config {
		var v config
		if path, ok := files["default.yaml"]; ok { // the default tag names a file next to the test
			t.Chdir(filepath.Dir(path))
		}

		options = append(options, WithYAMLLoader(), WithConfig(func(c *Config) {
			c.Args, c.Envs, c.EnvPrefix = args, envs, "APP"
		}))
		require.NoError(t, Load(&v, options...))

		return v
	}

	assert.Equal(t, "default.yaml", load(nil, nil).Name, "the default tag")
	assert.Equal(t, "env.yaml", load(nil, []string{"APP_CONFIG=" + files["env.yaml"]}).Name, "the variable")
	flag := []string{"--config", files["flag.yaml"]}
	assert.Equal(t, "flag.yaml", load(flag, []string{"APP_CONFIG=" + files["env.yaml"]}).Name, "the flag")
	assert.Equal(t, "code.yaml", load(nil, nil, WithDefaults("flag", map[string]any{"config": files["code.yaml"]})).Name,
		"WithDefaults")

	got := load(nil, []string{"APP_CONFIG=" + files["env.yaml"]})
	assert.Equal(t, files["env.yaml"], got.Path, "the field tells the file that was read")
}
