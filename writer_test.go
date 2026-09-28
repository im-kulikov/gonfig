package gonfig

import (
	"bytes"
	"encoding"
	"errors"
	"log/slog"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rtEmbedded struct {
	Level slog.Level `yaml:"level" json:"level" toml:"level" env:"LEVEL"`
}

type rtInner struct {
	Name string        `yaml:"name" json:"name" toml:"name" env:"NAME"`
	Wait time.Duration `yaml:"wait" json:"wait" toml:"wait" env:"WAIT"`
}

type rtItem struct {
	Host  string  `yaml:"host"  json:"host"  toml:"host"`
	Port  int     `yaml:"port"  json:"port"  toml:"port"`
	Inner rtInner `yaml:"inner" json:"inner" toml:"inner"`
}

// fileFormatsUnderTest maps the output formats to the loaders that read them.
var fileFormatsUnderTest = map[Format]ParserType{FormatYAML: ParserYAML, FormatJSON: ParserJSON, FormatTOML: ParserTOML}

// roundTrip has a field of every kind the loader supports.
type roundTrip struct {
	rtEmbedded // inlined without a tag

	Inlined rtInner `yaml:",inline" json:",inline" toml:",inline" env:",squash"`

	Text     string            `yaml:"text"     json:"text"     toml:"text"     env:"TEXT" default:"x" usage:"a text"`
	Int      int               `yaml:"int"      json:"int"      toml:"int"      env:"INT"`
	Int8     int8              `yaml:"int8"     json:"int8"     toml:"int8"     env:"INT8"`
	Uint     uint64            `yaml:"uint"     json:"uint"     toml:"uint"     env:"UINT"`
	Float    float64           `yaml:"float"    json:"float"    toml:"float"    env:"FLOAT"`
	Bool     bool              `yaml:"bool"     json:"bool"     toml:"bool"     env:"BOOL"`
	Duration time.Duration     `yaml:"duration" json:"duration" toml:"duration" env:"DURATION"`
	Time     time.Time         `yaml:"time"     json:"time"     toml:"time"     env:"TIME"`
	IP       net.IP            `yaml:"ip"       json:"ip"       toml:"ip"       env:"IP"`
	Network  net.IPNet         `yaml:"network"  json:"network"  toml:"network"  env:"NETWORK"`
	Strings  []string          `yaml:"strings"  json:"strings"  toml:"strings"  env:"STRINGS"`
	Ints     []int             `yaml:"ints"     json:"ints"     toml:"ints"     env:"INTS"`
	Waits    []time.Duration   `yaml:"waits"    json:"waits"    toml:"waits"    env:"WAITS"`
	Labels   map[string]string `yaml:"labels"   json:"labels"   toml:"labels"   env:"LABELS"`
	Counts   map[string]int    `yaml:"counts"   json:"counts"   toml:"counts"   env:"COUNTS"`
	Nested   rtInner           `yaml:"nested"   json:"nested"   toml:"nested"   env:"NESTED"`
	Pointer  *rtInner          `yaml:"pointer"  json:"pointer"  toml:"pointer"  env:"POINTER"`
	Items    []rtItem          `yaml:"items"    json:"items"    toml:"items"    env:"-"` // env cannot hold structs
	Password string            `yaml:"password" json:"password" toml:"password" env:"PASSWORD" secret:"true"`
	Since    *time.Time        `yaml:"since"    json:"since"    toml:"since"    env:"SINCE"`
	Never    *time.Time        `yaml:"never"    json:"never"    toml:"never"    env:"NEVER"`
	IPs      []net.IP          `yaml:"ips"      json:"ips"      toml:"ips"      env:"IPS"`
	Levels   []slog.Level      `yaml:"levels"   json:"levels"   toml:"levels"   env:"LEVELS"`
	Pair     [2]int            `yaml:"pair"     json:"pair"     toml:"pair"     env:"PAIR"`
	Seed     uint64            `yaml:"seed"     json:"seed"     toml:"seed"     env:"SEED" usage:"a seed\nAPP_INT=1"`
	Complex  complex128        `yaml:"complex"  json:"complex"  toml:"complex"  env:"COMPLEX"`
	Routes   map[string]string `yaml:"routes"   json:"routes"   toml:"routes"   env:"ROUTES"`

	Servers map[string]rtInner `yaml:"servers"  json:"servers"  toml:"servers"  env:"SERVERS"`
	Empty   map[string]int     `yaml:"empty"    json:"empty"    toml:"empty"    env:"EMPTY"`

	Hooks []map[string]string `yaml:"hooks" json:"hooks" toml:"hooks" env:"-"` // env cannot hold maps in a list
}

func newRoundTrip(t *testing.T) roundTrip {
	t.Helper()

	_, network, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	return roundTrip{
		rtEmbedded: rtEmbedded{Level: slog.LevelWarn},
		Inlined:    rtInner{Name: "inlined", Wait: time.Second},
		Text:       "a&b <c> \"q\" \\ ünï = x \x7f",
		Int:        -42,
		Int8:       -8,
		Uint:       42,
		Float:      1.5,
		Bool:       true,
		Duration:   90 * time.Second,
		Time:       time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
		IP:         net.ParseIP("192.168.1.1"),
		Network:    *network,
		Strings:    []string{"a", "b c"},
		Ints:       []int{1, 2},
		Waits:      []time.Duration{time.Second, time.Minute},
		Labels:     map[string]string{"team": "core", "tier": "1", "team_name": "x"},
		Counts:     map[string]int{"a": 1, "b": 2},
		Nested:     rtInner{Name: "nested", Wait: time.Hour},
		Pointer:    &rtInner{Name: "pointer", Wait: time.Millisecond},
		Items:      []rtItem{{Host: "a", Port: 1, Inner: rtInner{Name: "i"}}, {Host: "b", Port: 2}},
		Password:   "hunter2",
		Since:      new(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		IPs:        []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")},
		Levels:     []slog.Level{slog.LevelDebug, slog.LevelError},
		Pair:       [2]int{3, 4},
		Seed:       math.MaxUint64,
		Complex:    complex(1.5, -2),
		Routes:     map[string]string{"404": "a", "true": "b", "null": "c", "<<": "d", "1.10": "e"},
		Empty:      map[string]int{},
		Servers:    map[string]rtInner{"main": {Name: "m", Wait: time.Second}, "replica": {Name: "r"}},
		Hooks:      []map[string]string{{"url": "http://x"}, {"url": "http://y", "404": "z"}},
	}
}

func write(t *testing.T, v any, format Format) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, Write(&buf, v, format, WithEnvPrefix("APP")))

	return buf.String()
}

// loadEnvOutput loads the output of FormatEnv as the env loader would.
func loadEnvOutput(t *testing.T, output string, dest any) {
	t.Helper()

	var envs []string
	for line := range strings.Lines(output) {
		if line = strings.TrimSuffix(line, "\n"); !strings.HasPrefix(line, "#") {
			envs = append(envs, line)
		}
	}

	require.NoError(t, LoadEnvs(PrepareEnvs(envs, "APP"), dest))
}

func TestWrite_RoundTrip(t *testing.T) {
	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			want := newRoundTrip(t)
			output := write(t, &want, format)

			var got roundTrip
			require.NoError(t, loadFile(t, kind, output, &got), output)

			want.Password = "" // secrets are not written
			assert.Equal(t, want, got, output)
		})
	}

	t.Run(string(FormatEnv), func(t *testing.T) {
		want := newRoundTrip(t)
		output := write(t, &want, FormatEnv)

		var got roundTrip
		loadEnvOutput(t, output, &got)

		want.Password, want.Items, want.Hooks, want.Empty = "", nil, nil, nil // env has no empty map
		assert.Equal(t, want, got, output)
	})
}

func TestWrite_SpecialFloats(t *testing.T) {
	type floats struct {
		NaN float64 `yaml:"nan" json:"nan" toml:"nan" env:"NAN"`
		Inf float64 `yaml:"inf" json:"inf" toml:"inf" env:"INF"`
		Neg float64 `yaml:"neg" json:"neg" toml:"neg" env:"NEG"`
	}

	check := func(t *testing.T, got floats) {
		t.Helper()
		assert.True(t, math.IsNaN(got.NaN))
		assert.True(t, math.IsInf(got.Inf, 1))
		assert.True(t, math.IsInf(got.Neg, -1))
	}

	in := floats{NaN: math.NaN(), Inf: math.Inf(1), Neg: math.Inf(-1)}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			var got floats
			require.NoError(t, loadFile(t, kind, write(t, &in, format), &got))
			check(t, got)
		})
	}

	t.Run(string(FormatEnv), func(t *testing.T) {
		var got floats
		loadEnvOutput(t, write(t, &in, FormatEnv), &got)
		check(t, got)
	})
}

func TestWrite_Comments(t *testing.T) {
	v := newRoundTrip(t)

	yml := write(t, &v, FormatYAML)
	assert.Contains(t, yml, "# a text (env: APP_TEXT, default: x)\ntext: ")
	assert.Contains(t, yml, "# env: APP_PASSWORD, secret\npassword: \"\"")
	assert.NotContains(t, yml, "hunter2")
	assert.Less(t, strings.Index(yml, "level:"), strings.Index(yml, "text:"), "fields keep their order")

	toml := write(t, &v, FormatTOML)
	assert.Contains(t, toml, "# a text (env: APP_TEXT, default: x)\ntext = ")
	assert.Less(t, strings.Index(toml, "password ="), strings.Index(toml, "[labels]"), "values go before tables")
	assert.Contains(t, toml, `items = [{ host = "a", port = 1, inner = { name = "i", wait = "0s" } }, `)

	env := write(t, &v, FormatEnv)
	assert.Contains(t, env, "# a text (default: x)\nAPP_TEXT=")
	assert.Contains(t, env, "# secret\nAPP_PASSWORD=\n")
	assert.Contains(t, env, "APP_NESTED_WAIT=1h0m0s\n")
	assert.Contains(t, env, "APP_LABELS_team=core\n")
	assert.NotContains(t, env, "ITEMS")

	assert.NotContains(t, write(t, &v, FormatJSON), "hunter2")
}

type badText struct{}

func (badText) MarshalText() ([]byte, error) { return nil, errors.New("cannot marshal") }

func (*badText) UnmarshalText([]byte) error { return nil }

type left struct {
	Nil       *rtInner          `yaml:"nil"`
	Network   net.IPNet         `yaml:"network"`
	Secret    map[string]string `yaml:"secret" secret:"true"`
	Func      func()            `yaml:"func"`
	Complex   complex128        `yaml:"complex"`
	Bad       badText           `yaml:"bad"`
	Skipped   string            `yaml:"-"`
	Empty     struct{}          `yaml:"empty"`
	unexposed string

	Kept    string         `yaml:"kept"`
	Mixed   []any          `yaml:"mixed"`
	Pointer []*int         `yaml:"pointers"`
	Maps    []map[int]int  `yaml:"maps"`
	Keys    map[string]int `yaml:"keys"`
	Untag   string
}

func TestWrite_LeavesOut(t *testing.T) {
	one := 1
	v := left{
		Secret:    map[string]string{"github": "token"},
		Func:      func() {},
		Complex:   1i,
		Skipped:   "skipped",
		unexposed: "unexposed",
		Kept:      "kept",
		Mixed:     []any{"a", 1, nil},
		Pointer:   []*int{&one},
		Maps:      []map[int]int{{1: 1}},
		Keys:      map[string]int{"a b": 1},
		Untag:     "untagged",
	}

	assert.Equal(t, "complex: (0+1i)\nkept: kept\nmixed:\n  - a\n  - 1\npointers:\n  - 1\nmaps:\n  - \"1\": 1\n"+
		"keys:\n  a b: 1\nUntag: untagged\n", write(t, &v, FormatYAML))
	assert.Equal(t, "\n[Keys]\n\"a b\" = 1\n", write(t, &struct {
		Keys map[string]int
	}{Keys: map[string]int{"a b": 1}}, FormatTOML))
	assert.Empty(t, write(t, &v, FormatEnv), "no field has an env tag")
	assert.Equal(t, "APP_PLAIN=a,b\n", write(t, &struct {
		Plain []string `env:"PLAIN"`
		Items []rtItem `env:"ITEMS"`
	}{Plain: []string{"a", "b"}, Items: []rtItem{{Host: "a"}}}, FormatEnv), "env cannot hold a list of structs")
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWrite_Errors(t *testing.T) {
	var v roundTrip

	require.ErrorIs(t, Write(&bytes.Buffer{}, &v, "xml"), ErrUnknownFormat)
	require.ErrorIs(t, Write(&bytes.Buffer{}, v, FormatYAML), ErrExpectPointer)
	require.ErrorIs(t, Write(&bytes.Buffer{}, new(int), FormatYAML), ErrExpectStruct)
	require.EqualError(t, Write(failWriter{}, &v, FormatYAML), "disk full")
}

func TestWrite_EnvCannotHold(t *testing.T) {
	for name, v := range map[string]any{
		"a line break": &struct {
			Cert string `env:"CERT"`
		}{Cert: "-----BEGIN\nAPP_ADMIN=true"},
		"a NUL byte": &struct {
			Token string `env:"TOKEN"`
		}{Token: "a\x00b"},
		"a comma": &struct {
			Args []string `env:"ARGS"`
		}{Args: []string{"--opt=a,b"}},
		"= in a map key": &struct {
			Labels map[string]string `env:"LABELS"`
		}{Labels: map[string]string{"a=b": "c"}},
		"NUL in a map key": &struct {
			Labels map[string]string `env:"LABELS"`
		}{Labels: map[string]string{"a\x00b": "c"}},
		"_ in a key of a map of structs": &struct {
			Servers map[string]rtInner `env:"SERVERS"`
		}{Servers: map[string]rtInner{"team_name": {Name: "x"}}},
		"no key in a map of maps": &struct {
			Nested map[string]map[string]int `env:"NESTED"`
		}{Nested: map[string]map[string]int{"": {"a": 1}}},
		"no map key": &struct {
			Labels map[string]string `env:"LABELS"`
		}{Labels: map[string]string{"": "c"}},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			require.ErrorIs(t, Write(&buf, v, FormatEnv), ErrEnvValue)
			assert.Empty(t, buf.String(), "nothing is written")
			require.NoError(t, Write(&buf, v, FormatYAML), "other formats hold it")
		})
	}

	t.Run("a prefix with a line break or =", func(t *testing.T) {
		v := &struct {
			Name string `env:"NAME"`
		}{Name: "x"}

		for _, prefix := range []string{"A=B", "A\nADMIN", "A\x00B"} {
			var buf bytes.Buffer
			require.ErrorIs(t, Write(&buf, v, FormatEnv, WithEnvPrefix(prefix)), ErrEnvValue, "%q", prefix)
			assert.Empty(t, buf.String(), "nothing is written")
		}
	})
}

type shadowBase struct {
	Name string `yaml:"name" json:"name" toml:"name"`
	Port int    `yaml:"port" json:"port" toml:"port"`
}

// marshalOnly has no UnmarshalText: the loader reads its fields, not a text.
type marshalOnly struct {
	A int `yaml:"a" json:"a" toml:"a"`
}

func (marshalOnly) MarshalText() ([]byte, error) { return []byte("text"), nil }

func TestWrite_MarshalTextOnly(t *testing.T) {
	type config struct {
		V marshalOnly `yaml:"v" json:"v" toml:"v"`
	}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			output := write(t, &config{V: marshalOnly{A: 1}}, format)

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.Equal(t, 1, got.V.A)
		})
	}
}

func TestWrite_EmbeddedTextStruct(t *testing.T) {
	type wrap struct {
		net.IPNet // read from text, not inlined: env cannot set it without a tag

		Name string `env:"NAME"`
	}

	_, network, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	v := struct {
		Net wrap `env:"NET"`
	}{Net: wrap{IPNet: *network, Name: "x"}}

	assert.Equal(t, "APP_NET_NAME=x\n", write(t, &v, FormatEnv), "no APP_NET=10.0.0.0/8 the loader would not read")
}

func TestWrite_TextMapKeys(t *testing.T) {
	type config struct {
		Times map[time.Time]string `yaml:"times" json:"times" toml:"times" env:"TIMES"`
	}

	want := config{Times: map[time.Time]string{time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC): "x"}}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			output := write(t, &want, format)

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.Equal(t, want, got)
		})
	}

	var got config
	loadEnvOutput(t, write(t, &want, FormatEnv), &got)
	assert.Equal(t, want, got)
}

func TestWrite_EmptySections(t *testing.T) {
	type config struct {
		Feature *struct{}           `yaml:"feature" json:"feature" toml:"feature"`
		Off     *struct{}           `yaml:"off"     json:"off"     toml:"off"`
		Set     map[string]struct{} `yaml:"set"     json:"set"     toml:"set"     env:"SET"`
	}

	env := write(t, &config{Set: map[string]struct{}{"a_b": {}}}, FormatEnv)
	assert.Empty(t, env, "env cannot hold an empty entry, whatever its key")

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			output := write(t, &config{Feature: &struct{}{}, Set: map[string]struct{}{"a": {}}}, format)

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.NotNil(t, got.Feature, "a section set to an empty struct stays set:\n%s", output)
			assert.Nil(t, got.Off, output)
			assert.Equal(t, map[string]struct{}{"a": {}}, got.Set, output)
		})
	}
}

func TestWrite_Shadowed(t *testing.T) {
	type config struct {
		shadowBase

		Name string `yaml:"NAME" json:"NAME" toml:"NAME"` // shadows shadowBase.Name, keys match case-insensitively
	}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			output := write(t, &config{shadowBase: shadowBase{Name: "base", Port: 1}, Name: "app"}, format)
			assert.NotContains(t, output, "base", "the shadowed field is left out")

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.Equal(t, "app", got.Name)
			assert.Equal(t, 1, got.Port)
		})
	}
}

func TestWrite_ShadowedByNil(t *testing.T) {
	type config struct {
		shadowBase

		Name *string `yaml:"name" json:"name" toml:"name"` // nil: still shadows shadowBase.Name
	}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			output := write(t, &config{shadowBase: shadowBase{Name: "base", Port: 1}}, format)
			assert.NotContains(t, output, "base", "a nil field keeps its key: the value would read into it")

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.Nil(t, got.Name)
			assert.Equal(t, 1, got.Port)
		})
	}
}

func TestWrite_PointerToMapNotInlined(t *testing.T) {
	type config struct {
		Name  string             `yaml:"name" json:"name" toml:"name"`
		Extra *map[string]string `yaml:",inline" json:",inline" toml:",inline"` // a map is inlined, a pointer to one is not
	}

	for format, kind := range fileFormatsUnderTest {
		t.Run(string(format), func(t *testing.T) {
			want := config{Name: "x", Extra: &map[string]string{"foo": "bar"}}
			output := write(t, &want, format)

			var got config
			require.NoError(t, loadFile(t, kind, output, &got), output)
			assert.Equal(t, want, got, output)
		})
	}
}

func TestWrite_Interfaces(t *testing.T) {
	level := slog.LevelWarn

	type config struct {
		Level encoding.TextMarshaler `yaml:"level"`
		Any   any                    `yaml:"any"`
	}

	assert.Equal(t, "level: WARN\nany: 1\n", write(t, &config{Level: &level, Any: 1}, FormatYAML),
		"an interface is written as the value it holds")
	assert.Equal(t, "{}\n", write(t, &config{}, FormatYAML), "a nil interface is left out")
}

func TestWrite_MapAndListCycles(t *testing.T) {
	m := map[string]any{"a": 1}
	m["self"] = m

	l := []any{1, nil}
	l[1] = l

	n := map[string]any{} // a map in a list in the map
	n["list"] = []any{n}

	type config struct {
		Map    map[string]any `yaml:"map"`
		List   []any          `yaml:"list"`
		Nested map[string]any `yaml:"nested"`
		Inline map[string]any `yaml:",inline"`
	}

	assert.Equal(t, "map:\n  a: 1\nlist:\n  - 1\nnested:\n  list: []\na: 1\n",
		write(t, &config{Map: m, List: l, Nested: n, Inline: m}, FormatYAML),
		"a map or a list inside itself is left out, not written forever")
}
