package gonfig

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type secretDB struct {
	User string `env:"USER" flag:"db-user"`
	Pass string `env:"PASS" flag:"db-pass"`
}

type secretSettings struct {
	Host     string   `env:"HOST"     flag:"host"     default:"localhost" usage:"server host"`
	Password string   `env:"PASSWORD" flag:"password" default:"dev-only"  usage:"admin password" secret:"true"`
	DB       secretDB `env:"DB"       secret:"true"` // every nested field is a secret
}

func help(t *testing.T, envs []string) string {
	t.Helper()

	var out bytes.Buffer
	var v secretSettings
	err := Load(&v, WithCustomOutput(&out), WithCustomExit(func(int) {}), WithConfig(func(c *Config) {
		c.Args = []string{"--help"}
		c.Envs = envs
	}))
	require.ErrorIs(t, err, ErrTestExit)

	return out.String()
}

// H-29: --help showed values loaded from env and files as flag defaults.
func TestHelpShowsTagDefaultsNotValues(t *testing.T) {
	out := help(t, []string{"HOST=prod.internal", "PASSWORD=hunter2", "DB_PASS=s3cret"})

	assert.NotContains(t, out, "prod.internal")
	assert.NotContains(t, out, "hunter2")
	assert.NotContains(t, out, "s3cret")
	assert.Contains(t, out, `(default "localhost")`, "a regular flag shows its tag default")
	assert.NotContains(t, out, "dev-only", "a secret flag hides even its tag default")
}

func TestUsageOfEnvs_Secret(t *testing.T) {
	usage := UsageOfEnvs(&secretSettings{})

	assert.Contains(t, usage, "'HOST' <string> — server host (default: localhost)")
	assert.Contains(t, usage, "'PASSWORD' <string> — admin password (secret)")
	assert.Contains(t, usage, "'DB_PASS' <string> (secret)")
	assert.NotContains(t, usage, "dev-only")
}

func TestParseTagOptions_Secret(t *testing.T) {
	assert.True(t, ParseTagOptions(reflect.StructTag(`secret:"true"`)).FieldSecret)
	assert.False(t, ParseTagOptions(reflect.StructTag(`secret:"false"`)).FieldSecret)
	assert.False(t, ParseTagOptions(``).FieldSecret)
}
