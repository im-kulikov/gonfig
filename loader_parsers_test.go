package gonfig

import (
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
