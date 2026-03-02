package gonfig

import (
	"testing"

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

		l := setLoaderDefaults(Config{
			Args: []string{"--config", "config.yaml"},
		})
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
