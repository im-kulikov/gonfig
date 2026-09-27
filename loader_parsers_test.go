package gonfig

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noArgs(c *Config) { c.Args, c.Envs = []string{}, []string{"NAME=from-env"} }

// H-03: a flag defined twice was a pflag panic.
func TestFlags_DefinedTwice(t *testing.T) {
	var config struct {
		DefaultConfigFlag

		Config string `flag:"config,config:true"`
	}
	require.ErrorIs(t, Load(&config, WithConfig(noArgs)), ErrFlagRedefined)

	var names struct {
		A string `flag:"name"`
		B string `flag:"name"`
	}
	require.ErrorIs(t, Load(&names, WithConfig(noArgs)), ErrFlagRedefined)

	var short struct {
		A string `flag:"a,short:x"`
		B string `flag:"b,short:x"`
	}
	err := Load(&short, WithConfig(noArgs))
	require.ErrorIs(t, err, ErrFlagRedefined)
	assert.Contains(t, err.Error(), "-x")

	var printConfig struct {
		PrintConfigFlag

		Print string `flag:"print-config"`
	}
	require.ErrorIs(t, Load(&printConfig, WithConfig(noArgs)), ErrFlagRedefined)
}

// M-10: a custom parser of a built-in type replaced the parser but was called twice.
func TestCustomParser_ReplacesBuiltin(t *testing.T) {
	calls := 0
	env := NewCustomParser(ParserEnv, func(any) error { calls++; return nil })

	var v struct {
		Name string `env:"NAME"`
	}
	require.NoError(t, Load(&v, WithConfig(noArgs), WithCustomParser(env)))

	assert.Equal(t, 1, calls)
	assert.Empty(t, v.Name, "the built-in env loader is replaced")
}

func TestCustomParser_OncePerLoad(t *testing.T) {
	calls := 0
	custom := func(any) error { calls++; return nil }
	parser := New(Config{}, WithConfig(noArgs),
		WithCustomParser(NewCustomParser("custom", custom)),
		WithCustomParser(NewCustomParser("custom", custom))) // the same type twice: the last one wins

	var v struct{}
	require.NoError(t, parser.Load(&v))
	require.NoError(t, parser.Load(&v))

	assert.Equal(t, 2, calls, "once per Load, however many times options are applied")
}

// H-04: one Parser used from several goroutines at once.
func TestParser_ConcurrentLoad(t *testing.T) {
	type settings struct {
		DefaultConfigFlag

		Name string `yaml:"name" env:"NAME" flag:"name"`
		Port int    `yaml:"port" env:"PORT" default:"80"`
	}

	path := writeTempFile(t, "config.yaml", "name: from-file\nport: 8080\n")
	parser := New(Config{Args: []string{"--config", path, "--name", "from-flag"}, Envs: []string{"PORT=9090"}},
		WithYAMLLoader(), WithStrict())

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			var v settings
			assert.NoError(t, parser.Load(&v))
			assert.Equal(t, settings{Name: "from-flag", Port: 9090}, v)
		})
	}

	wg.Wait()
}

// H-05: the config path of one Load leaked into the next one.
func TestParser_LoadsDoNotShareState(t *testing.T) {
	var v struct {
		DefaultConfigFlag

		Name string `yaml:"name"`
	}

	path := writeTempFile(t, "config.yaml", "name: from-file\n")
	args := [][]string{{"--config", path}, {}}
	parser := New(Config{Envs: []string{}}, WithYAMLLoader(), WithOptions(func() []LoaderOption {
		next := args[0]
		args = args[1:]

		return []LoaderOption{WithConfig(func(c *Config) { c.Args = next })}
	}))

	require.NoError(t, parser.Load(&v))
	require.Equal(t, "from-file", v.Name)

	v.Name = ""
	require.NoError(t, parser.Load(&v))
	require.Empty(t, v.Name, "the second Load has no --config")
}

// H-06: Skip* set through WithConfig had no effect.
func TestWithConfig_Skip(t *testing.T) {
	var v struct {
		Name string `env:"NAME" flag:"name" default:"default"`
	}

	require.NoError(t, Load(&v, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{"--unknown"}, []string{"NAME=from-env"}
		c.SkipEnv, c.SkipFlags, c.SkipDefaults = true, true, true
	})))
	assert.Empty(t, v.Name, "neither defaults, nor env, nor flags (an unknown flag is not an error)")
}
