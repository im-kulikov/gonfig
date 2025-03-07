package gonfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewYamlLoader(t *testing.T) {
	parser := NewYamlLoader()
	assert.NotNil(t, parser)
}

func TestYamlLoader_SetConfigPath(t *testing.T) {
	parser := &yamlLoader{}
	expect := "config.yaml"
	parser.SetConfigPath(expect)

	assert.EqualValues(t, &expect, parser.path.Load())
}

func TestYamlLoader_Load_EmptyPath(t *testing.T) {
	parser := NewYamlLoader()

	var v any
	assert.NoError(t, parser.Load(&v))
}

func TestYamlLoader_Load_CantOpenFile(t *testing.T) {
	parser := NewYamlLoader().(*yamlLoader)
	parser.SetConfigPath("nonexistent.yaml")

	var v any
	assert.ErrorIs(t, parser.Load(&v), ErrYamlCantOpen)
}

func TestYamlLoader_Load_CantParse(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "invalid.yaml")
	require.NoError(t, err)

	_, err = file.WriteString("invalid_yaml: [unclosed_sequence")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	parser := NewYamlLoader().(*yamlLoader)
	parser.SetConfigPath(file.Name())

	var v any
	assert.ErrorIs(t, parser.Load(&v), ErrYamlCantParse)
}

func TestYamlLoader_Load_Success(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "valid.yaml")
	require.NoError(t, err)

	_, err = file.WriteString("key: value\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	parser := NewYamlLoader().(*yamlLoader)
	parser.SetConfigPath(file.Name())

	var v map[string]string
	assert.NoError(t, parser.Load(&v))
	assert.Equal(t, "value", v["key"])
}

func TestYamlLoader_Type(t *testing.T) {
	parser := NewYamlLoader()
	assert.Equal(t, ParserYAML, parser.Type())
}

func TestConstantError(t *testing.T) {
	err := constantError("test error")
	assert.EqualError(t, err, "test error")
}

func TestParserTypeEquality(t *testing.T) {
	assert.Equal(t, ParserType("yaml-loader"), ParserYAML)
}

func TestWithYamlLoader(t *testing.T) {
	var v struct{}
	require.NoError(t, Load(&v, WithYamlLoader()))
}
