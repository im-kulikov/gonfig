package gonfig

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for M-33: pointers to structs, "sections" that are nil until something sets them.

type ptrTLS struct {
	Enabled    bool   `yaml:"enabled"     env:"ENABLED"`
	Cert       string `yaml:"cert"        env:"CERT"        required:"true"`
	MinVersion string `yaml:"min_version" env:"MIN_VERSION" default:"TLS13"`
}

func (t *ptrTLS) Validate() error {
	if t.MinVersion == "TLS10" {
		return errors.New("TLS10 is not allowed")
	}

	return nil
}

type ptrServer struct {
	TLS *ptrTLS `yaml:"tls" env:"TLS"`
}

func noSources(c *Config) { c.Args, c.Envs = []string{}, []string{} }

func TestPointers_NilSectionStaysNil(t *testing.T) {
	var v ptrServer
	require.NoError(t, Load(&v, WithConfig(noSources)))
	assert.Nil(t, v.TLS, "no defaults, no required check, no Validate for a section nobody set")
}

func TestPointers_SectionSetInCode(t *testing.T) {
	v := ptrServer{TLS: &ptrTLS{Cert: "cert.pem"}}
	require.NoError(t, Load(&v, WithConfig(noSources)))
	assert.Equal(t, "TLS13", v.TLS.MinVersion, "defaults fill the section")

	v = ptrServer{TLS: &ptrTLS{}}
	err := Load(&v, WithConfig(noSources))
	require.ErrorIs(t, err, ErrMissingFields)
	assert.Contains(t, err.Error(), "`TLS.Cert`")

	v = ptrServer{TLS: &ptrTLS{Cert: "cert.pem", MinVersion: "TLS10"}}
	require.EqualError(t, Load(&v, WithConfig(noSources)), "TLS: TLS10 is not allowed")
}

func TestReflectFieldsOf_Pointers(t *testing.T) {
	v := ptrServer{TLS: &ptrTLS{}}

	var names []string
	for elem, err := range ReflectFieldsOf(&v, ReflectOptions{}) {
		require.NoError(t, err)
		names = append(names, elem.Field.Name)
	}
	assert.Equal(t, []string{"TLS"}, names, "without the option a pointer is a field, as before")

	names = nil
	for elem, err := range ReflectFieldsOf(&v, ReflectOptions{Pointers: true}) {
		require.NoError(t, err)
		names = append(names, elem.Owner.Field.Name+"."+elem.Field.Name)
	}
	assert.Equal(t, []string{"TLS.Enabled", "TLS.Cert", "TLS.MinVersion"}, names)

	names = nil
	for elem, err := range ReflectFieldsOf(&ptrServer{}, ReflectOptions{Pointers: true}) {
		require.NoError(t, err)
		names = append(names, elem.Field.Name)
	}
	assert.Equal(t, []string{"TLS"}, names, "a nil pointer is a field")
}

// A section created by a source starts from its defaults, like the root of the config.
func TestPointers_SectionCreatedByFile(t *testing.T) {
	type settings struct {
		DefaultConfigFlag
		ptrServer `yaml:",inline"`
	}

	cases := map[string]*ptrTLS{
		"other: 1\n":        nil,
		"tls: null\n":       nil,
		"tls:\n  cert: c\n": {Cert: "c", MinVersion: "TLS13"},
		"tls:\n  cert: c\n  min_version: TLS12\n": {Cert: "c", MinVersion: "TLS12"},
	}

	for content, want := range cases {
		path := writeTempFile(t, "config.yaml", content)

		var v settings
		require.NoError(t, Load(&v, WithYAMLLoader(), WithConfig(func(c *Config) {
			c.Args, c.Envs = []string{"--config", path}, []string{}
		})), content)
		assert.Equal(t, want, v.TLS, content)
	}
}

// The case of go-bones: TLS enabled through the environment lost min_version.
func TestPointers_SectionCreatedByEnv(t *testing.T) {
	var v ptrServer
	require.NoError(t, Load(&v, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{}, []string{"TLS_ENABLED=true", "TLS_CERT=c"}
	})))
	assert.Equal(t, &ptrTLS{Enabled: true, Cert: "c", MinVersion: "TLS13"}, v.TLS)

	v = ptrServer{}
	err := Load(&v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{"TLS_ENABLED=true"} }))
	require.ErrorIs(t, err, ErrMissingFields, "a created section checks its required fields")
}

func TestPointers_NotSections(t *testing.T) {
	var v struct {
		Since *time.Time `env:"SINCE"` // a text value, not a section
		Port  *int       `env:"PORT"`
		Inner *struct {
			Deeper *ptrTLS `env:"DEEPER"`
			Name   string  `env:"NAME" default:"inner"`
		} `env:"INNER"`
	}

	require.NoError(t, Load(&v, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{}, []string{"SINCE=2026-09-28T00:00:00Z", "PORT=1", "INNER_OTHER=x"}
	})))
	assert.Equal(t, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), *v.Since)
	assert.Equal(t, 1, *v.Port)
	require.NotNil(t, v.Inner, "INNER_OTHER creates the section")
	assert.Equal(t, "inner", v.Inner.Name)
	assert.Nil(t, v.Inner.Deeper, "a nested section nobody set stays nil")
}

func TestPointers_SectionWithInvalidDefault(t *testing.T) {
	var v struct {
		Section *struct {
			Port int `env:"PORT" default:"not a number"`
		} `env:"SECTION"`
	}

	err := Load(&v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{"SECTION_PORT=1"} }))
	require.ErrorContains(t, err, `could not parse "not a number"`)
}

type ptrNode struct {
	Name string   `env:"NAME"`
	Next *ptrNode `env:"NEXT"` // a type that contains itself
}

func TestUsageOfEnvs_Sections(t *testing.T) {
	v := ptrServer{}
	usage := UsageOfEnvs(&v)

	assert.Contains(t, usage, "'TLS_ENABLED' <bool>")
	assert.Contains(t, usage, "'TLS_MIN_VERSION' <string> (default: TLS13)")
	assert.NotContains(t, usage, "<*gonfig.ptrTLS>")
	assert.Nil(t, v.TLS, "the struct passed in is not changed")

	assert.Equal(t, "Environment variables:\n  - 'NAME' <string>\n  - 'NEXT' <*gonfig.ptrNode>", UsageOfEnvs(&ptrNode{}))
}
