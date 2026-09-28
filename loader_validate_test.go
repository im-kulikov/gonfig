package gonfig

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validServer struct {
	Port int `env:"PORT"`
}

func (s validServer) Validate() error {
	if s.Port < 1024 {
		return errors.New("privileged port")
	}

	return nil
}

type validDB struct {
	DSN string `env:"DSN"`
}

func (d *validDB) Validate() error { // pointer receiver
	if d.DSN == "" {
		return errors.New("empty dsn")
	}

	return nil
}

type validEmbedded struct{ calls *int }

func (e validEmbedded) Validate() error {
	*e.calls++

	return nil
}

type validRoot struct {
	validEmbedded // promoted to validRoot: validated once, as the root

	Server validServer `env:"SERVER"`
	Backup struct {
		DB validDB `env:"DB"`
	} `env:"BACKUP"`
	Pointer *validServer // a section set by the code is validated too
	hidden  validServer  // unexported: cannot be called
}

var errRoot = errors.New("root")

type validRootOwn struct {
	Server validServer `env:"SERVER"`
	err    error
}

func (r *validRootOwn) Validate() error { return r.err }

// F-07: Validate of nested structs was never called.
func TestValidate_Nested(t *testing.T) {
	calls := 0
	v := validRoot{
		validEmbedded: validEmbedded{calls: &calls},
		Pointer:       &validServer{Port: 1},
		hidden:        validServer{Port: 1}, // would fail if validated
	}

	err := Load(&v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{"SERVER_PORT=80"} }))
	require.Error(t, err)
	assert.Equal(t, "Server: privileged port\nBackup.DB: empty dsn\nPointer: privileged port", err.Error())
	assert.Equal(t, 1, calls, "an embedded Validate is promoted and runs once")

	calls, v.Pointer = 0, nil // a nil section is not validated
	err = Load(&v, WithConfig(func(c *Config) {
		c.Args, c.Envs = []string{}, []string{"SERVER_PORT=8080", "BACKUP_DB_DSN=postgres://"}
	}))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestValidate_RootErrorUnchanged(t *testing.T) {
	src := WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{"SERVER_PORT=8080"} })

	v := validRootOwn{err: errRoot}
	require.Equal(t, errRoot, Load(&v, src), "without nested errors the root error is returned as is")

	v = validRootOwn{err: errRoot}
	err := Load(&v, WithConfig(func(c *Config) { c.Args, c.Envs = []string{}, []string{"SERVER_PORT=1"} }))
	require.ErrorIs(t, err, errRoot)
	assert.Equal(t, "Server: privileged port\nroot", err.Error(), "nested structs first, the root last")
}

type validItem struct {
	Host string `yaml:"host" required:"true"`
	Port int    `yaml:"port"`
}

func (i validItem) Validate() error {
	if i.Port > 65535 {
		return errors.New("port out of range")
	}

	return nil
}

func TestValidate_ListsAndMaps(t *testing.T) {
	var config struct {
		Items  []validItem          `yaml:"items"`
		Ptrs   []*validItem         `yaml:"ptrs"`
		ByName map[string]validItem `yaml:"by_name"`
	}

	err := loadFile(t, ParserYAML, "items: [{host: a}, {port: 1}]\nptrs: [{host: b, port: 70000}]\n"+
		"by_name: {x: {port: 2}, y: {host: c, port: 99999}}", &config)
	require.NoError(t, err)

	err = ValidateRequiredFields(&config)
	require.ErrorIs(t, err, ErrMissingFields)
	assert.Contains(t, err.Error(), "`Items[1].Host`")
	assert.Contains(t, err.Error(), "`ByName[x].Host`")

	err = validate(&config)
	require.Error(t, err)
	assert.Equal(t, "Ptrs[0]: port out of range\nByName[y]: port out of range", err.Error())

	for range structElements(reflect.ValueOf(config.Items), nil) {
		break // an iterator stops when asked
	}

	config.Ptrs = append(config.Ptrs, nil) // a nil item is skipped
	loop := &validTree{Name: "root"}
	loop.Children = []*validTree{{Children: []*validTree{loop}}}
	require.NoError(t, validate(loop), "a child pointing back to the root is a cycle, not followed")
	require.ErrorContains(t, ValidateRequiredFields(loop), "`Children[0].Name`")
}

type validTree struct {
	Name     string `required:"true"`
	Children []*validTree
}
