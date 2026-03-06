package gonfig

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

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

	expected := map[string]interface{}{
		"HELLO_WORLD":         "1",
		"TEST_VALUE_FOR_TEST": "42",
		"FOO_BAR_BAZ":         "100",

		"HELLO": map[string]interface{}{
			"WORLD": "1",
		},
		"TEST": map[string]interface{}{
			"VALUE_FOR_TEST": "42",
			"VALUE": map[string]interface{}{
				"FOR_TEST": "42",
				"FOR": map[string]interface{}{
					"TEST": "42",
				},
			},
		},
		"FOO": map[string]interface{}{
			"BAR_BAZ": "100",
			"BAR": map[string]interface{}{
				"BAZ": "100",
			},
		},
	}

	result := PrepareEnvs(envs, "")
	require.Equal(t, expected, result)
}

// Тестирование LoadEnvs
func TestLoadEnvs(t *testing.T) {
	envs := map[string]interface{}{
		"HELLO": map[string]interface{}{
			"WORLD": "1",
		},
		"FOO": map[string]interface{}{
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
	invalidEnvs := map[string]interface{}{
		"HELLO": "invalid structure", // Здесь должна быть вложенная карта, но передана строка
	}

	// Ожидаемая структура
	type Config struct {
		Hello struct {
			World string `env:"WORLD"`
		} `env:"HELLO"`
	}

	var config Config
	require.Error(t, LoadEnvs(invalidEnvs, &config))
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
