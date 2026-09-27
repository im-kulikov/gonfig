package gonfig

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type argsConfig struct {
	Verbose bool     `flag:"verbose,short:v"`
	Files   []string `flag:",args" default:"default.txt"`
}

func loadArgs(t *testing.T, v any, args ...string) error {
	t.Helper()

	// Not nil even without arguments: nil means os.Args, and a test runner that passes
	// "-test.run ^TestArgs$" as two arguments leaves the pattern as a positional one.
	return Load(v, WithConfig(func(c *Config) { c.Args, c.Envs = append([]string{}, args...), []string{} }))
}

// M-16: positional arguments were lost.
func TestArgs(t *testing.T) {
	var v argsConfig
	require.NoError(t, loadArgs(t, &v, "-v", "a.txt", "--", "-b.txt"))
	assert.Equal(t, argsConfig{Verbose: true, Files: []string{"a.txt", "-b.txt"}}, v)

	v = argsConfig{}
	require.NoError(t, loadArgs(t, &v, "-v"))
	assert.Equal(t, []string{"default.txt"}, v.Files, "no arguments: the value of other sources stays")

	var required struct {
		Files []string `flag:",args" required:"true"`
	}
	require.ErrorIs(t, loadArgs(t, &required), ErrMissingFields)

	var wrong struct {
		Files string `flag:",args"`
	}
	require.ErrorContains(t, loadArgs(t, &wrong, "a"), "field Files: `flag:\",args\"` needs []string")
}

func TestArgs_NotInConfigOutput(t *testing.T) {
	v := argsConfig{Verbose: true, Files: []string{"a.txt"}}

	var out bytes.Buffer
	require.NoError(t, Write(&out, &v, FormatYAML))
	assert.Equal(t, "Verbose: true\n", out.String(), "positional arguments are not configuration")
}
