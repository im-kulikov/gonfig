package gonfig

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flagPort int

type flagEnabled bool

type flagTypes struct {
	Int8     int8              `flag:"int8"`
	Int16    int16             `flag:"int16"`
	Uint8    uint8             `flag:"uint8"`
	Uint16   uint16            `flag:"uint16"`
	Uints    []uint            `flag:"uints"`
	Labels   map[string]string `flag:"labels"`
	Counts   map[string]int    `flag:"counts"`
	Sizes    map[string]int64  `flag:"sizes"`
	Port     flagPort          `flag:"port"    default:"8080" usage:"a named type"`
	Level    slog.Level        `flag:"level"   default:"warn" usage:"a TextUnmarshaler"`
	Since    time.Time         `flag:"since"`
	Pointer  *int              `flag:"pointer" default:"7"`
	Enabled  flagEnabled       `flag:"enabled"`
	Weights  map[string]bool   `flag:"weights"`
	Ports    []flagPort        `flag:"ports"`
	Complex  complex64         `flag:"complex"`
	Unset    map[string]bool   `flag:"unset"`
	Timeouts map[string]string `flag:"timeouts,short:t"`
	Plain    string            `flag:"plain,short:-"` // no shorthand
}

// M-14: flags of int8/16, uint8/16, maps, named types, TextUnmarshaler and pointers.
func TestFlags_MoreTypes(t *testing.T) {
	var v flagTypes
	err := Load(&v, WithConfig(func(c *Config) {
		c.Envs = []string{}
		c.Args = []string{
			"--int8=-8", "--int16=-16", "--uint8=8", "--uint16=16", "--uints=1,2",
			"--labels=team=core,tier=1", "--counts=a=1", "--sizes=b=2",
			"--port=9090", "--level=debug", "--since=2026-09-28T00:00:00Z", "--pointer=5",
			"--enabled", "--weights=a:true,b:false", "--ports=1,2", "--complex=1+2i", "-t", "read=1s",
			"--plain=text",
		}
	}))
	require.NoError(t, err)

	five := 5
	assert.Equal(t, flagTypes{
		Int8: -8, Int16: -16, Uint8: 8, Uint16: 16, Uints: []uint{1, 2},
		Labels:   map[string]string{"team": "core", "tier": "1"},
		Counts:   map[string]int{"a": 1},
		Sizes:    map[string]int64{"b": 2},
		Port:     9090,
		Level:    slog.LevelDebug,
		Since:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		Pointer:  &five,
		Enabled:  true,
		Weights:  map[string]bool{"a": true, "b": false},
		Ports:    []flagPort{1, 2},
		Complex:  1 + 2i,
		Timeouts: map[string]string{"read": "1s"},
		Plain:    "text",
	}, v)
}

func TestFlags_MoreTypesHelp(t *testing.T) {
	var (
		out bytes.Buffer
		v   flagTypes
	)

	err := Load(&v, WithCustomOutput(&out), WithCustomExit(func(int) {}), WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{"--help"}, []string{"PORT=1"}
	}))
	require.ErrorIs(t, err, ErrTestExit)

	line := func(flag string) string {
		for l := range strings.Lines(out.String()) {
			if strings.Contains(l, flag+" ") {
				return strings.Join(strings.Fields(l), " ")
			}
		}

		return ""
	}

	assert.Equal(t, "--port flagPort a named type (default 8080)", line("--port"))
	assert.Equal(t, "--level Level a TextUnmarshaler (default WARN)", line("--level"))
	assert.Equal(t, "--enabled flagEnabled[=true]", line("--enabled"))
	assert.Equal(t, "--pointer *int (default 7)", line("--pointer"))
	assert.Equal(t, "--plain string", line("--plain"))
	assert.Equal(t, "--unset map[string]bool", line("--unset"), "a zero value shows no default")
	assert.NotContains(t, out.String(), "(default [])", "empty lists and maps show no default either")
}

func TestFlags_MoreTypesInvalid(t *testing.T) {
	var v flagTypes
	err := Load(&v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{"--port=x"}, []string{} }))
	require.ErrorContains(t, err, `invalid argument "x" for "--port" flag`)
}

func TestFlags_RepeatedAndPointerBool(t *testing.T) {
	var config struct {
		Levels  []slog.Level    `flag:"level" default:"error"`
		Weights map[string]bool `flag:"weight" default:"z:true"`
		Debug   *bool           `flag:"debug"`
	}

	require.NoError(t, Load(&config, WithConfig(func(c *Config) {
		c.Envs = []string{}
		c.Args = []string{"--level=debug", "--level=warn,info", "--weight=a:true", "--weight=b:false", "--debug"}
	})))

	assert.Equal(t, []slog.Level{slog.LevelDebug, slog.LevelWarn, slog.LevelInfo}, config.Levels,
		"a flag given again adds to the list, the first replaces the default")
	assert.Equal(t, map[string]bool{"a": true, "b": false}, config.Weights)
	require.NotNil(t, config.Debug)
	assert.True(t, *config.Debug, "--debug needs no value")
}

func TestFlags_SecretValueNotInError(t *testing.T) {
	var config struct {
		Key []byte `flag:"key,base:hex" secret:"true"`
		Pin int    `flag:"pin" secret:"true"`
	}

	for _, arg := range []string{"--key=00112233XX", "--pin=12x34"} {
		var out bytes.Buffer

		err := Load(&config, WithCustomOutput(&out), WithConfig(func(c *Config) {
			c.Args, c.Envs = []string{arg}, []string{}
		}))
		require.ErrorContains(t, err, "the value is secret")
		assert.NotContains(t, err.Error()+out.String(), arg[strings.Index(arg, "=")+1:])
	}
}
