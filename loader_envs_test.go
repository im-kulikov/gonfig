package gonfig_test

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
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

	result := gonfig.PrepareEnvs(envs, "")
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
	err := gonfig.LoadEnvs(envs, &config)
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
	require.Error(t, gonfig.LoadEnvs(invalidEnvs, &config))
}

func TestLoadEnvs_custom(t *testing.T) {
	var config struct {
		SSH struct {
			AuthSock string `env:"AUTH_SOCK"`
		} `env:"SSH"`
	}

	envs := gonfig.PrepareEnvs([]string{
		"APP_SSH_AUTH_SOCK=aaaa",
		"ENV_WITH_UNKNOWN_PREFIX=bbb",
	}, "APP")

	require.NoError(t, gonfig.LoadEnvs(envs, &config))
	require.Equal(t, "aaaa", config.SSH.AuthSock)
}

func TestLoadEnvs_Errors(t *testing.T) {
	// not pointer
	require.Error(t, gonfig.LoadEnvs(nil, struct{}{}))
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
				envs := gonfig.PrepareEnvs([]string{"SOME_FIELD=" + anyToString(tt)}, "")

				example := reflect.New(reflect.StructOf([]reflect.StructField{{
					Name: "SOME_FIELD",
					Tag:  `env:"SOME_FIELD"`,
					Type: reflect.TypeOf(tt),
				}}))

				require.NoError(t, gonfig.LoadEnvs(envs, example.Interface()))
			})
		})
	}
}
