package gonfig

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
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

// Regression tests for tmp/REVIEW.md: H-27, H-28, M-13, M-30, H-02, M-32.

type fileEmbedded struct {
	Level slog.Level `yaml:"level" json:"level" toml:"level"`
}

type fileSettings struct {
	fileEmbedded // no tag: every format inlines it, as env does (H-27)

	Interval time.Duration `yaml:"interval" json:"interval" toml:"interval"`
	Network  net.IPNet     `yaml:"network"  json:"network"  toml:"network"`
	Big      int64         `yaml:"big"      json:"big"      toml:"big"`
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func loadFile(t *testing.T, kind ParserType, content string, dest any) error {
	t.Helper()

	parser := testFileLoader(t, kind)
	parser.SetConfigPath(writeTempFile(t, "config", content))

	return parser.Load(dest)
}

func TestFileLoader_SameRulesAsEnv(t *testing.T) {
	_, network, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	expect := fileSettings{
		fileEmbedded: fileEmbedded{Level: slog.LevelWarn},
		Interval:     3 * time.Second,
		Network:      *network,
		Big:          9007199254740993, // 2^53+1: lost if JSON numbers go through float64
	}

	cases := map[ParserType]string{
		ParserYAML: "level: WARN\ninterval: 3s\nnetwork: 10.0.0.0/8\nbig: 9007199254740993\n",
		ParserJSON: `{"level": "WARN", "interval": "3s", "network": "10.0.0.0/8", "big": 9007199254740993}`,
		ParserTOML: "level = \"WARN\"\ninterval = \"3s\"\nnetwork = \"10.0.0.0/8\"\nbig = 9007199254740993\n",
	}

	for kind, content := range cases {
		t.Run(string(kind), func(t *testing.T) {
			var got fileSettings
			require.NoError(t, loadFile(t, kind, content, &got))
			assert.Equal(t, expect, got)
		})
	}

	t.Run("duration as nanoseconds still works", func(t *testing.T) {
		var got fileSettings
		require.NoError(t, loadFile(t, ParserJSON, `{"interval": 3000000000}`, &got))
		assert.Equal(t, 3*time.Second, got.Interval)
	})
}

func TestFileLoader_EmptyFile(t *testing.T) {
	cases := map[string]struct {
		kind    ParserType
		content string
	}{
		"yaml empty":         {ParserYAML, ""},
		"yaml comments only": {ParserYAML, "# nothing here yet\n"},
		"json empty":         {ParserJSON, ""},
		"json whitespace":    {ParserJSON, " \n"},
		"toml comments only": {ParserTOML, "# nothing here yet\n"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			got := fileSettings{Interval: time.Minute}
			require.NoError(t, loadFile(t, tt.kind, tt.content, &got))
			assert.Equal(t, time.Minute, got.Interval, "an empty file must not reset values")
		})
	}
}

type trackingFile struct {
	io.Reader

	closed bool
}

func (f *trackingFile) Close() error {
	f.closed = true

	return nil
}

func TestFileLoader_ClosesFileOnParseError(t *testing.T) {
	file := &trackingFile{Reader: strings.NewReader("invalid: [")}
	parser := testFileLoader(t, ParserYAML, func(options *fileLoaderOptions) {
		options.open = func(string) (io.ReadCloser, error) { return file, nil }
	})
	parser.SetConfigPath("config.yaml")

	var v fileSettings
	require.ErrorIs(t, parser.Load(&v), ErrCantParse)
	assert.True(t, file.closed)
}

func TestFileLoader_ErrorIsOneLineWithPath(t *testing.T) {
	parser := testFileLoader(t, ParserYAML)
	path := writeTempFile(t, "broken.yaml", "interval: [1, 2]\nnetwork: 300.0.0.0/8\n")
	parser.SetConfigPath(path)

	var v fileSettings
	err := parser.Load(&v)
	require.ErrorIs(t, err, ErrCantParse)
	assert.Contains(t, err.Error(), path)
	assert.NotContains(t, err.Error(), "\n")
}

func TestFileLoader_JSONNumbers(t *testing.T) {
	var got struct {
		Uint  uint64  `json:"uint"`
		Float float64 `json:"float"`
	}

	require.NoError(t, loadFile(t, ParserJSON, `{"uint": 18446744073709551615, "float": 1.5}`, &got))
	assert.Equal(t, uint64(18446744073709551615), got.Uint)
	assert.InDelta(t, 1.5, got.Float, 0)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func (failingReader) Close() error { return nil }

func TestFileLoader_ReadError(t *testing.T) {
	parser := testFileLoader(t, ParserYAML, func(options *fileLoaderOptions) {
		options.open = func(string) (io.ReadCloser, error) { return failingReader{}, nil }
	})
	parser.SetConfigPath("config.yaml")

	var v fileSettings
	assert.ErrorIs(t, parser.Load(&v), ErrCantParse)
}

// M-31: typos in file keys are reported in strict mode.
func TestWithStrict(t *testing.T) {
	path := writeTempFile(t, "config.yaml", "levels: WARN\ninterval: 3s\n")
	load := func(options ...LoaderOption) error {
		var v struct {
			DefaultConfigFlag
			fileSettings
		}

		return Load(&v, append(options, WithYAMLLoader(), WithConfig(func(c *Config) {
			c.Args = []string{"--config", path}
		}))...)
	}

	require.NoError(t, load(), "unknown keys are ignored by default")

	err := load(WithStrict())
	require.ErrorIs(t, err, ErrCantParse)
	assert.Contains(t, err.Error(), "levels")

	require.Error(t, load(WithConfig(func(c *Config) { c.Strict = true })))
}

// M-11: several file loaders all read the same path.
func TestFileLoaders_ByExtension(t *testing.T) {
	type settings struct {
		DefaultConfigFlag

		Name string `yaml:"name" json:"name" toml:"name"`
	}

	all := []LoaderOption{WithJSONLoader(), WithYAMLLoader(), WithTOMLLoader()}
	cases := []struct {
		file, content string
		loaders       []LoaderOption
	}{
		{"config.yaml", "name: yaml\n", all},
		{"config.YML", "name: yaml\n", all},
		{"config.json", `{"name": "json"}`, all},
		{"config.toml", "name = \"toml\"\n", all},
		{"config.conf", `{"name": "json"}`, all},                              // unknown: the first loader
		{"config.conf", "name: yaml\n", []LoaderOption{WithYAMLLoader()}},     // a single loader reads any path
		{"config.json", "name: yaml\n", []LoaderOption{WithYAMLLoader()}},     // even another extension
		{"config.yaml", `{"name": "json"}`, []LoaderOption{WithJSONLoader()}}, // as before
	}

	for _, tt := range cases {
		path := writeTempFile(t, tt.file, tt.content)

		var v settings
		require.NoError(t, Load(&v, append(tt.loaders, WithConfig(func(c *Config) {
			c.Args, c.Envs = []string{"--config", path}, []string{}
		}))...), tt.file)
		assert.NotEmpty(t, v.Name, tt.file)
	}
}
