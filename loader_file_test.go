package gonfig

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testFileLoader(t *testing.T, kind ParserType, options ...fileLoaderOption) *fileLoader {
	t.Helper()

	tmp, err := initFileLoader(kind, options)(Config{})
	require.NoError(t, err)

	parser, ok := tmp.(*fileLoader)
	require.True(t, ok)

	return parser
}

func TestFileLoader_SetConfigPath(t *testing.T) {
	parser := testFileLoader(t, ParserYAML)
	expect := "config.yaml"
	parser.SetConfigPath(expect)

	assert.EqualValues(t, &expect, parser.path.Load())
}

func TestFileLoader_Load_CantOpenFile(t *testing.T) {
	parser := testFileLoader(t, ParserYAML)
	parser.SetConfigPath("nonexistent.toml")

	var v any
	assert.ErrorIs(t, parser.Load(&v), ErrCantOpen)
}

func TestFileLoader_Load_CantParse(t *testing.T) {
	parsers := map[ParserType]string{
		ParserYAML: `invalid_yaml: [unclosed_sequence`,
		ParserTOML: `invalid_toml=["unclosed_sequence"`,
		ParserJSON: `{"invalid_json": ["unclosed_sequence"}`,
	}

	for kind, content := range parsers {
		t.Run(string(kind), func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "invalid-confing")
			require.NoError(t, err)

			_, err = file.WriteString(content)
			require.NoError(t, err)
			require.NoError(t, file.Close())

			parser := testFileLoader(t, kind)
			parser.SetConfigPath(file.Name())

			var v any
			assert.ErrorIs(t, parser.Load(&v), ErrCantParse)
		})
	}
}

func TestFileLoader_Load_Success(t *testing.T) {
	parsers := map[ParserType]string{
		ParserYAML: `key: value`,
		ParserTOML: `key="value"`,
		ParserJSON: `{"key": "value"}`,
	}

	for kind, content := range parsers {
		t.Run(string(kind), func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "valid-config")
			require.NoError(t, err)

			_, err = file.WriteString(content)
			require.NoError(t, err)
			require.NoError(t, file.Close())

			parser := testFileLoader(t, kind)
			parser.SetConfigPath(file.Name())

			var v map[string]string
			assert.NoError(t, parser.Load(&v))
			assert.Equal(t, "value", v["key"])
		})
	}
}

func TestFileLoader_Type(t *testing.T) {
	parser := testFileLoader(t, ParserYAML)
	assert.Equal(t, ParserYAML, parser.Type())
}

func TestParserTypeEquality(t *testing.T) {
	assert.Equal(t, ParserType("json-loader"), ParserJSON)
	assert.Equal(t, ParserType("yaml-loader"), ParserYAML)
}

func TestWithFileLoader(t *testing.T) {
	var v struct{}
	require.NoError(t, Load(&v,
		WithYAMLLoader(),
		WithTOMLLoader(),
		WithJSONLoader()))
}

type failOpener struct{}

func (failOpener) Read(data []byte) (n int, err error) {
	tmp := []byte("key: value\n")
	copy(data, tmp)

	return len(tmp), io.EOF
}

func (failOpener) Close() error { return errors.New("fail opener") }

func failOsOpener(_ string) (io.ReadCloser, error) { return failOpener{}, nil }

func TestFileLoader_failOnClose(t *testing.T) {
	parser := testFileLoader(t, ParserYAML, func(options *fileLoaderOptions) {
		options.open = failOsOpener
	})

	parser.SetConfigPath("/path/to/file.yaml")

	var v any
	assert.ErrorIs(t, parser.Load(&v), ErrCantClose)
}

func Test_shouldFailOnUnknownType(t *testing.T) {
	parser := testFileLoader(t, ParserDefaults)
	parser.SetConfigPath("/path/to/file.yaml")

	var v any
	assert.ErrorIs(t, parser.Load(&v), ErrUnknownFileTypeParser)
}

type fileConfig struct {
	ConfigFile string `flag:"config,config:true,short:c"`

	Field1 string `yaml:"field1"`
	Field2 string `yaml:"field2"`

	inlineStruct `yaml:",inline"`

	Inner innerStruct `yaml:"inner"`
}

type inlineStruct struct {
	Field3 string `yaml:"field3"`
	Field4 string `yaml:"field4"`
}

type innerStruct struct {
	Field5 string `yaml:"field5"`
	Field6 string `yaml:"field6"`
}

const testConfigFileContent = `
field1: "field_1_content"
field2: "field_2_content"
field3: "field_3_content"
field4: "field_4_content"
inner:
  field5: "field_5_content"
  field6: "field_6_content"
`

func TestFileConfig(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "*.config.yml")
	require.NoError(t, err)

	spew.Dump(tmp.Name())

	_, err = tmp.Write([]byte(testConfigFileContent))
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	var cfg fileConfig
	require.NoError(t, Load(&cfg, WithYAMLLoader(), WithConfig(func(c *Config) {
		c.Args = append(c.Args, "--config", tmp.Name())
	})))

	var yml fileConfig
	require.NoError(t, yaml.Unmarshal([]byte(testConfigFileContent), &yml))

	yml.ConfigFile = tmp.Name()
	require.Equal(t, yml, cfg)
}
