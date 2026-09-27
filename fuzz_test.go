package gonfig

import (
	"fmt"
	"log/slog"
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

// Run one target with: go test -run '^$' -fuzz '^FuzzPrepareEnvs$' -fuzztime 1m

type fuzzEnvs struct {
	Name  string            `env:"NAME"`
	Port  int               `env:"PORT"`
	Level slog.Level        `env:"LEVEL"`
	Wait  time.Duration     `env:"WAIT"`
	List  []int             `env:"LIST"`
	Tags  map[string]string `env:"TAGS"`
	DB    struct {
		Host string `env:"HOST" default:"localhost"`
		Port uint16 `env:"PORT"`
	} `env:"DB"`
	TLS *struct {
		Cert string `env:"CERT"`
	} `env:"TLS"`
}

// FuzzPrepareEnvs: the variables, one per line, never panic the env loader, and their
// order does not matter while every name is set once.
func FuzzPrepareEnvs(f *testing.F) {
	f.Add("APP_NAME=x\nAPP_DB=dsn\nAPP_DB_HOST=db\nAPP_TLS_CERT=c", "APP")
	f.Add("NAME=x\nNAME_SUFFIX=y\nTAGS=a:1,b:2\nLIST=1,2", "")
	f.Add("A=1\nA__B=2\n_A=3\nA_=4\n=5\nLEVEL=warn\nWAIT=3s", "")

	f.Fuzz(func(t *testing.T, lines, prefix string) {
		var envs []string
		seen := map[string]bool{}

		for line := range strings.Lines(lines) {
			line = strings.TrimSuffix(line, "\n")
			if name, _, _ := strings.Cut(line, "="); !seen[name] {
				seen[name] = true
				envs = append(envs, line)
			}
		}

		reversed := slices.Clone(envs)
		slices.Reverse(reversed)
		require.Equal(t, PrepareEnvs(envs, prefix), PrepareEnvs(reversed, prefix), "order of %q", envs)

		var cfg fuzzEnvs
		_ = Load(&cfg, WithConfig(func(c *Config) {
			c.Envs, c.EnvPrefix, c.Args, c.SkipFlags = envs, prefix, []string{}, true
		}))
	})
}

var fuzzDefaultTypes = []reflect.Type{
	reflect.TypeFor[string](),
	reflect.TypeFor[int8](),
	reflect.TypeFor[uint16](),
	reflect.TypeFor[float32](),
	reflect.TypeFor[bool](),
	reflect.TypeFor[complex64](),
	reflect.TypeFor[time.Duration](),
	reflect.TypeFor[slog.Level](),
	reflect.TypeFor[net.IP](),
	reflect.TypeFor[net.IPMask](),
	reflect.TypeFor[net.IPNet](),
	reflect.TypeFor[[]int](),
	reflect.TypeFor[[2]string](),
	reflect.TypeFor[[][]uint8](),
	reflect.TypeFor[[]time.Duration](),
	reflect.TypeFor[map[string]int](),
	reflect.TypeFor[map[int]net.IP](),
	reflect.TypeFor[map[string][]string](),
	reflect.TypeFor[struct{ A int }](),
}

// FuzzDefault: a `default` tag never panics, and a pointer takes what its element takes.
func FuzzDefault(f *testing.F) {
	for _, seed := range []string{"1", "-1", "5s", "info", "127.0.0.1", "/24", "10.0.0.0/8",
		"a,b", "a:1,b:2", "1,2,3", "1+2i", "true", "NaN"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		for _, typ := range fuzzDefaultTypes {
			plain := reflect.New(typ).Elem()
			errPlain := applyDefault(plain, value)

			ptr := reflect.New(reflect.PointerTo(typ)).Elem()
			errPtr := applyDefault(ptr, value)

			require.Equal(t, errPlain == nil, errPtr == nil, "%s from %q: %v, pointer: %v", typ, value, errPlain, errPtr)

			if errPlain == nil && !ptr.IsNil() {
				// as text: NaN != NaN
				require.Equal(t, fmt.Sprintf("%#v", plain), fmt.Sprintf("%#v", ptr.Elem()), "%s from %q", typ, value)
			}
		}
	})
}

// FuzzParseTagOptions: any tag that PrepareFlags lets through registers a flag without a panic.
func FuzzParseTagOptions(f *testing.F) {
	for _, seed := range []string{`flag:"name"`, `flag:"name,short:n,config:true"`, `flag:",args"`, `flag:"-"`,
		`flag:"data,base:hex,short:-" required:"true" secret:"true"`, `flag:"x,short:ab"`, `flag:"x,short:й"`} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, tag string) {
		opts := ParseTagOptions(reflect.StructTag(tag))

		require.Equal(t, opts.FlagFullName == "-", opts.FieldIgnored)
		require.False(t, opts.FlagArgs && opts.FlagFullName != "", "args with a name")

		// skipped or rejected by PrepareFlags
		if opts.FlagFullName == "" || opts.FieldIgnored || len(opts.FlagShortName) > 1 {
			return
		}

		var value string
		require.NoError(t, prepareFlag(pflag.NewFlagSet("fuzz", pflag.ContinueOnError), reflect.ValueOf(&value).Elem(), opts))
	})
}
