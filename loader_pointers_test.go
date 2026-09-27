package gonfig

import (
	"errors"
	"testing"

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
