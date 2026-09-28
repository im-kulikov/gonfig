package gonfig

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Тестирование PrepareEnvs
func TestPrepareEnvs(t *testing.T) {
	envs := []string{
		"HELLO_WORLD=1",
		"TEST_VALUE_FOR_TEST=42",
		"FOO_BAR_BAZ=100",
		"INVALID_FORMAT", // Этот элемент будет пропущен
	}

	expected := map[string]any{
		"HELLO_WORLD":         "1",
		"TEST_VALUE_FOR_TEST": "42",
		"FOO_BAR_BAZ":         "100",

		"HELLO": map[string]any{
			"WORLD": "1",
		},
		"TEST": map[string]any{
			"VALUE_FOR_TEST": "42",
			"VALUE": map[string]any{
				"FOR_TEST": "42",
				"FOR": map[string]any{
					"TEST": "42",
				},
			},
		},
		"FOO": map[string]any{
			"BAR_BAZ": "100",
			"BAR": map[string]any{
				"BAZ": "100",
			},
		},
	}

	result := PrepareEnvs(envs, "")
	require.Equal(t, expected, result)
}

// Тестирование LoadEnvs
func TestLoadEnvs(t *testing.T) {
	envs := map[string]any{
		"HELLO": map[string]any{
			"WORLD": "1",
		},
		"FOO": map[string]any{
			"BAR": "test-value",
		},
		"TIMEOUT": (time.Second * 15).String(),
	}

	// Ожидаемая структура, в которую будут загружены данные
	type Config struct {
		Hello struct {
			World int `env:"WORLD"`
		} `env:"HELLO"`
		Foo struct {
			Bar string `env:"BAR"`
		} `env:"FOO"`
		Timeout time.Duration `env:"TIMEOUT"`
	}

	var config Config
	err := LoadEnvs(envs, &config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expected := Config{
		Hello: struct {
			World int `env:"WORLD"`
		}{World: 1},
		Foo: struct {
			Bar string `env:"BAR"`
		}{Bar: "test-value"},
		Timeout: time.Second * 15,
	}

	require.Equal(t, expected, config)
}

// Тестирование случая с ошибкой в LoadEnvs
func TestLoadEnvs_Error(t *testing.T) {
	type Config struct {
		Port int `env:"PORT"`
	}

	var config Config
	require.ErrorIs(t, LoadEnvs(map[string]any{"PORT": "not a number"}, &config), ErrDecode)
}

func TestLoadEnvs_custom(t *testing.T) {
	var config struct {
		SSH struct {
			AuthSock string `env:"AUTH_SOCK"`
		} `env:"SSH"`
	}

	envs := PrepareEnvs([]string{
		"APP_SSH_AUTH_SOCK=aaaa",
		"ENV_WITH_UNKNOWN_PREFIX=bbb",
	}, "APP")

	require.NoError(t, LoadEnvs(envs, &config))
	require.Equal(t, "aaaa", config.SSH.AuthSock)
}

func TestLoadEnvs_Errors(t *testing.T) {
	// not pointer
	require.Error(t, LoadEnvs(nil, struct{}{}))
}

func anyToString(v any) string {
	if vs, ok := v.(fmt.Stringer); ok {
		return vs.String()
	}

	if value := reflect.ValueOf(v); value.Kind() == reflect.Slice {
		var items []string
		for i := 0; i < value.Len(); i++ {
			items = append(items, anyToString(value.Index(i).Interface()))
		}

		return strings.Join(items, ",")
	} else if value.Kind() == reflect.Struct {
		ptr := reflect.New(value.Type())
		ptr.Elem().Set(value)
		if stringer, ok := ptr.Interface().(fmt.Stringer); ok {
			return stringer.String()
		}
	}

	return fmt.Sprint(v)
}

func TestEnvPrimitives(t *testing.T) {
	cases := []any{
		0, int8(1), int16(2), int32(3), int64(4),
		uint(5), uint8(6), uint16(7), uint32(8), uint64(9),
		float32(10), float64(11),
		complex64(12), complex128(13),
		true,
		net.ParseIP("127.0.0.1"),
		net.IPNet{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(16, 32)},
		[]string{"a", "b", "c"},
		[]int{1, 2, 3, 4, 5},
	}

	for _, tt := range cases {
		t.Run(reflect.TypeOf(tt).String(), func(t *testing.T) {
			require.NotPanics(t, func() {
				envs := PrepareEnvs([]string{"SOME_FIELD=" + anyToString(tt)}, "")

				example := reflect.New(reflect.StructOf([]reflect.StructField{{
					Name: "SOME_FIELD",
					Tag:  `env:"SOME_FIELD"`,
					Type: reflect.TypeOf(tt),
				}}))

				require.NoError(t, LoadEnvs(envs, example.Interface()))
			})
		})
	}
}

func Test_setMultipleFields(t *testing.T) {
	var example struct {
		FieldOne string `env:"SOME_FIELD"`
		FieldTwo string `env:"SOME_FIELD"`
		Nested   struct {
			FieldThree string `env:"SOME_FIELD"`
		} `env:",squash"`
	}

	envs := PrepareEnvs([]string{"SOME_FIELD=value"}, "")
	require.NoError(t, LoadEnvs(envs, &example))
	require.Equal(t, "value", example.FieldOne)
	require.Equal(t, "value", example.FieldTwo)
	require.Equal(t, "value", example.Nested.FieldThree)
}

func Test_tryToDecodeInvalidSlice(t *testing.T) {
	var example struct {
		FieldOne string `env:"SOME_FIELD"`
		FieldTwo string `env:"SOME_FIELD"`
		Nested   struct {
			FieldThree []int `env:"SOME_FIELD"`
		} `env:",squash"`
	}

	envs := PrepareEnvs([]string{"SOME_FIELD=1,2,3,b"}, "")
	err := LoadEnvs(envs, &example)
	require.ErrorIs(t, err, strconv.ErrSyntax)
}

func Test_EmptyForShowUsageOfEnvsWithErrors(t *testing.T) {
	var example struct{}
	require.Empty(t, UsageOfEnvs(example))
}

func Test_validParseSlice(t *testing.T) {
	cases := []struct {
		name string
		envs []string
		want any
	}{
		{name: "empty strings array", envs: []string{"TEST="}, want: []string(nil)},
		{name: "empty ints array", envs: []string{"TEST="}, want: []int(nil)},
		{name: "empty floats array", envs: []string{"TEST="}, want: []float64(nil)},
		{name: "empty booleans array", envs: []string{"TEST="}, want: []bool(nil)},
		{name: "multiple ints array", envs: []string{"TEST=1,2,3"}, want: []int{1, 2, 3}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			target := reflect.New(reflect.StructOf([]reflect.StructField{
				{
					Name: "Test",
					Type: reflect.TypeOf(tt.want),
					Tag:  `env:"TEST"`,
				},
			}))

			envs := PrepareEnvs(tt.envs, "")
			require.NoError(t, LoadEnvs(envs, target.Interface()))

			// Use reflection to get the value of the "Test" field
			val := target.Elem().FieldByName("Test")
			if reflect.ValueOf(tt.want).Len() == 0 {
				require.Empty(t, val.Interface())
			} else {
				require.Equal(t, tt.want, val.Interface())
			}
		})
	}
}

func TestLoadEnvs_CustomStringType(t *testing.T) {
	type CustomString string
	var config struct {
		List []CustomString `env:"LIST"`
	}

	envs := PrepareEnvs([]string{"LIST=a,b,c"}, "")
	require.NoError(t, LoadEnvs(envs, &config))
	require.Equal(t, []CustomString{"a", "b", "c"}, config.List)
}

func TestLoadEnvs_CustomStringTypeSource(t *testing.T) {
	type CustomString string
	var config struct {
		List []string `env:"LIST"`
	}

	envs := map[string]any{"LIST": CustomString("a,b,c")}
	require.NoError(t, LoadEnvs(envs, &config))
	require.Equal(t, []string{"a", "b", "c"}, config.List)
}

func TestLoadEnvs_EmptySlice(t *testing.T) {
	var config struct {
		List []string `env:"LIST"`
	}

	envs := PrepareEnvs([]string{"LIST="}, "")
	require.NoError(t, LoadEnvs(envs, &config))
	require.Empty(t, config.List)
}
func TestUsageOfEnvs_Nested(t *testing.T) {
	type APIConfig struct {
		Address string `env:"ADDRESS" usage:"API address"`
	}

	t.Run("unreachable nested fields should be hidden", func(t *testing.T) {
		type unreachableConfig struct {
			Activator struct {
				API APIConfig `env:""`
			} `env:"ACT"`
		}
		var cfg unreachableConfig
		usage := UsageOfEnvs(&cfg)

		require.NotContains(t, usage, "ACT_ADDRESS")
	})

	t.Run("squashed nested fields should be visible", func(t *testing.T) {
		type squashedConfig struct {
			Activator struct {
				API APIConfig `env:",squash"`
			} `env:"ACT"`
		}
		var cfg squashedConfig
		usage := UsageOfEnvs(&cfg)

		require.Contains(t, usage, "ACT_ADDRESS")
	})

	t.Run("anonymous nested fields should be visible", func(t *testing.T) {
		type anonConfig struct {
			Activator struct {
				APIConfig
			} `env:"ACT"`
		}
		var cfg anonConfig
		usage := UsageOfEnvs(&cfg)

		require.Contains(t, usage, "ACT_ADDRESS")
	})

	t.Run("leading underscores should be removed for empty root tags", func(t *testing.T) {
		type rootConfig struct {
			Field string `env:"FIELD"`
		}
		var cfg rootConfig
		usage := UsageOfEnvs(&cfg)

		require.Contains(t, usage, "- 'FIELD' <string>")
		require.NotContains(t, usage, "- '_FIELD'")
	})
}

// M-13: env uses TextUnmarshaler, like defaults and files.
func TestLoadEnvs_TextUnmarshaler(t *testing.T) {
	var config struct {
		Level slog.Level `env:"LEVEL"`
	}

	require.NoError(t, LoadEnvs(map[string]any{"LEVEL": "WARN"}, &config))
	assert.Equal(t, slog.LevelWarn, config.Level)
}

// M-07: the prefix matches whole segments and may be given with or without "_".
func TestPrepareEnvs_Prefix(t *testing.T) {
	envs := []string{"APP_Y=2", "APPLE_X=1", "APPX=3", "APP=4", "OTHER_Y=5"}

	for _, prefix := range []string{"APP", "APP_"} {
		assert.Equal(t, map[string]any{"Y": "2"}, PrepareEnvs(envs, prefix), prefix)
	}

	assert.Len(t, PrepareEnvs(envs, ""), 7, "no prefix: every variable, nested keys included")
}

func TestEnvPrefix_Consistent(t *testing.T) {
	var v struct {
		Name string `env:"NAME"`
	}

	for _, prefix := range []string{"APP", "APP_"} {
		assert.Contains(t, UsageOfEnvs(&v, EnvUsageWithPrefix(prefix)), "'APP_NAME'", prefix)

		var out strings.Builder
		require.NoError(t, Write(&out, &v, FormatEnv, WithEnvPrefix(prefix)))
		assert.Equal(t, "APP_NAME=\n", out.String(), prefix)
	}
}

// M-08: a variable named like a field with nested names broke the whole loading,
// depending on the order of the environment.
func TestLoadEnvs_NameClash(t *testing.T) {
	type settings struct {
		DB struct {
			Host string `env:"HOST"`
		} `env:"DB"`
		Name   string            `env:"NAME"`
		Other  string            `env:"OTHER"`
		Hosts  []string          `env:"HOSTS"`
		Labels map[string]string `env:"LABELS"`
		Since  time.Time         `env:"SINCE"`
	}

	envs := []string{
		"DB=postgres://db", "DB_HOST=h", // a struct and a variable of its name
		"NAME=n", "NAME_SUFFIX=x", // a string and an unrelated variable below it
		"OTHER_SUFFIX=x", // only the unrelated one
		"HOSTS=a,b", "HOSTS_EXTRA=x",
		"LABELS=x", "LABELS_team=core",
		"SINCE=2026-09-28T00:00:00Z", "SINCE_EPOCH=x", // a TextUnmarshaler struct
	}

	reversed := slices.Clone(envs)
	slices.Reverse(reversed)

	for _, order := range [][]string{envs, reversed} {
		var v settings
		require.NoError(t, LoadEnvs(PrepareEnvs(order, ""), &v))

		assert.Equal(t, "h", v.DB.Host)
		assert.Equal(t, "n", v.Name)
		assert.Empty(t, v.Other)
		assert.Equal(t, []string{"a", "b"}, v.Hosts)
		assert.Equal(t, map[string]string{"team": "core"}, v.Labels)
		assert.Equal(t, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), v.Since)
	}
}

type clashEmbedded struct {
	Level string `env:"LEVEL"`
}

func TestLoad_NameClashKeepsOtherSources(t *testing.T) {
	var v struct {
		clashEmbedded // its fields read from the same level

		Other   string `env:"OTHER" default:"from-default"`
		Mode    string `env:"MODE"`
		Pointer *struct {
			Host string `env:"HOST"`
		} `env:"PTR"`
	}

	require.NoError(t, Load(&v, WithConfig(func(c *Config) {
		c.Args = []string{}
		c.Envs = []string{"OTHER_SUFFIX=x", "LEVEL=debug", "LEVEL_X=x", "PTR=x", "PTR_HOST=h", "mode=m", "mode_x=x"}
	})))

	assert.Equal(t, "from-default", v.Other, "an unrelated OTHER_SUFFIX does not reset OTHER")
	assert.Equal(t, "debug", v.Level)
	assert.Equal(t, "m", v.Mode, "names match case-insensitively, as in mapstructure")
	require.NotNil(t, v.Pointer)
	assert.Equal(t, "h", v.Pointer.Host)
}

// A variable named like a struct or a map, with no nested names at all: ignored.
func TestLoadEnvs_NameOfContainerAlone(t *testing.T) {
	var v struct {
		DB struct {
			Host string `env:"HOST" default:"localhost"`
		} `env:"DB"`
		Labels map[string]string `env:"LABELS"`
	}

	require.NoError(t, SetDefaults(&v))
	require.NoError(t, LoadEnvs(PrepareEnvs([]string{"DB=postgres://db", "LABELS=x"}, ""), &v))

	assert.Equal(t, "localhost", v.DB.Host)
	assert.Nil(t, v.Labels)
}

func TestLoadEnvs_ListItems(t *testing.T) {
	var config struct {
		IPs    []net.IP         `env:"IPS"`
		Addrs  []netip.Addr     `env:"ADDRS"`
		Levels []slog.Level     `env:"LEVELS"`
		Times  []time.Time      `env:"TIMES"`
		Pair   [2]int           `env:"PAIR" default:"1,2"`
		Waits  [2]time.Duration `env:"WAITS"`
	}

	require.NoError(t, SetDefaults(&config))
	require.NoError(t, LoadEnvs(PrepareEnvs([]string{
		"IPS=10.0.0.1,::1", "ADDRS=10.0.0.2", "LEVELS=INFO,debug", "TIMES=2026-01-02T03:04:05Z", "PAIR=7", "WAITS=1s,2m",
	}, ""), &config))

	assert.Equal(t, []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")}, config.IPs)
	assert.Equal(t, []netip.Addr{netip.MustParseAddr("10.0.0.2")}, config.Addrs)
	assert.Equal(t, []slog.Level{slog.LevelInfo, slog.LevelDebug}, config.Levels)
	assert.Equal(t, []time.Time{time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, config.Times)
	assert.Equal(t, [2]int{7, 0}, config.Pair, "a variable replaces the default of an array")
	assert.Equal(t, [2]time.Duration{time.Second, 2 * time.Minute}, config.Waits)

	require.ErrorContains(t, LoadEnvs(PrepareEnvs([]string{"PAIR=5,6,7"}, ""), &config), "length less or equal to 2")
	require.ErrorContains(t, LoadEnvs(PrepareEnvs([]string{"ADDRS=nope"}, ""), &config), "ADDRS")
}

func TestLoadEnvs_MapKeys(t *testing.T) {
	type db struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}

	var config struct {
		Labels map[string]string         `env:"LABELS"`
		DBs    map[string]db             `env:"DBS"`
		Ptrs   map[string]*db            `env:"PTRS"`
		Nested map[string]map[string]int `env:"NESTED"`
	}

	require.NoError(t, LoadEnvs(PrepareEnvs([]string{
		"LABELS_team_name=core", "LABELS_team=x", "LABELS_a_b_c=1", "LABELS=ignored",
		"DBS_main_HOST=h", "DBS_main_PORT=5432", "DBS_replica_HOST=r", "DBS_main=ignored",
		"PTRS_x_HOST=p",
		"NESTED_a_b_c=1",
	}, ""), &config))

	assert.Equal(t, map[string]string{"team_name": "core", "team": "x", "a_b_c": "1"}, config.Labels,
		"a map of values takes the rest of the name as the key")
	assert.Equal(t, map[string]db{"main": {Host: "h", Port: 5432}, "replica": {Host: "r"}}, config.DBs,
		"a map of structs takes the first segment, the rest names a field")
	assert.Equal(t, map[string]*db{"x": {Host: "p"}}, config.Ptrs)
	assert.Equal(t, map[string]map[string]int{"a": {"b_c": 1}}, config.Nested)
}
