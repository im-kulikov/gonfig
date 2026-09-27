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
