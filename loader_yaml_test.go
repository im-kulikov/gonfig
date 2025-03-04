package gonfig

import (
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewYamlLoader(t *testing.T) {
	parser := NewYamlLoader()
	assert.NotNil(t, parser)
}

func TestYamlLoader_SetConfigPath(t *testing.T) {
	parser := &yamlLoader{}
	parser.SetConfigPath("config.yaml")
	assert.Equal(t, "config.yaml", parser.path)
}

func TestYamlLoader_Load_EmptyPath(t *testing.T) {
	parser := &yamlLoader{}
	var v any
	err := parser.Load(&v)
	assert.NoError(t, err)
}

func TestYamlLoader_Load_CantOpenFile(t *testing.T) {
	parser := &yamlLoader{path: "nonexistent.yaml"}
	var v any
	err := parser.Load(&v)
	assert.ErrorIs(t, err, ErrYamlCantOpen)
}

func TestYamlLoader_Load_CantParse(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "invalid.yaml")
	require.NoError(t, err)

	spew.Dump(file.Name())

	_, err = file.WriteString("invalid_yaml: [unclosed_sequence")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	parser := &yamlLoader{path: file.Name()}
	var v any
	err = parser.Load(&v)
	assert.ErrorIs(t, err, ErrYamlCantParse)
}

func TestYamlLoader_Load_Success(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "valid.yaml")
	require.NoError(t, err)

	_, err = file.WriteString("key: value\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	parser := &yamlLoader{path: file.Name()}
	var v map[string]string
	err = parser.Load(&v)
	assert.NoError(t, err)
	assert.Equal(t, "value", v["key"])
}

func TestYamlLoader_Type(t *testing.T) {
	parser := &yamlLoader{}
	assert.Equal(t, ParserYAML, parser.Type())
}

func TestConstantError(t *testing.T) {
	err := constantError("test error")
	assert.EqualError(t, err, "test error")
}

func TestParserTypeEquality(t *testing.T) {
	assert.Equal(t, ParserType("yaml-loader"), ParserYAML)
}
