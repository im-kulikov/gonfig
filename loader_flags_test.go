package gonfig_test

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
)

type TestFlagConfig struct {
	Debug      bool          `flag:"debug" flag-short:"d" usage:"enable debug mode"`
	Port       int           `flag:"port" usage:"port number"`
	Timeout    time.Duration `flag:"timeout" usage:"request timeout"`
	Name       string        `flag:"name" flag-short:"n" usage:"service name"`
	Tags       []string      `flag:"tags" usage:"list of tags"`
	IP         net.IP        `flag:"ip" usage:"server IP"`
	SkipDash   string        `flag:"-"`
	SkipEmpty  string
	unexported string
}

type NestedFlagConfig struct {
	TestFlagConfig
	IPMask net.IPMask `flag:"ip-mask" usage:"IP mask"`
}

const testField = "SomeField"

func TestPrepareFlag_Primitives(t *testing.T) {
	cases := []struct {
		item any
		name string
		tags reflect.StructTag

		error error
	}{
		{name: "bool", item: true, tags: reflect.StructTag(`flag:"bool"`)},
		{name: "bool-short", item: true, tags: reflect.StructTag(`flag:"bool-short" flag-short:"s"`)},
		{name: "string", item: "foo", tags: reflect.StructTag(`flag:"string"`)},
		{name: "string-short", item: "foo", tags: reflect.StructTag(`flag:"string-short" flag-short:"s"`)},
		{name: "int", item: 123, tags: reflect.StructTag(`flag:"int"`)},
		{name: "int-short", item: 321, tags: reflect.StructTag(`flag:"int-short" flag-short:"s"`)},
		{name: "int32", item: int32(123), tags: reflect.StructTag(`flag:"int32"`)},
		{name: "int32-short", item: int32(123), tags: reflect.StructTag(`flag:"int32-short" flag-short:"s"`)},
		{name: "int64", item: int64(123), tags: reflect.StructTag(`flag:"int64"`)},
		{name: "int64-short", item: int64(123), tags: reflect.StructTag(`flag:"int64-short" flag-short:"s"`)},
		{name: "uint", item: uint(123), tags: reflect.StructTag(`flag:"uint"`)},
		{name: "uint-short", item: uint(123), tags: reflect.StructTag(`flag:"uint-short" flag-short:"s"`)},
		{name: "uint32", item: uint32(123), tags: reflect.StructTag(`flag:"uint32"`)},
		{name: "uint32-short", item: uint32(123), tags: reflect.StructTag(`flag:"uint32-short" flag-short:"s"`)},
		{name: "uint64", item: uint64(123), tags: reflect.StructTag(`flag:"uint64"`)},
		{name: "uint64-short", item: uint64(123), tags: reflect.StructTag(`flag:"uint64-short" flag-short:"s"`)},
		{name: "float32", item: float32(123), tags: reflect.StructTag(`flag:"float32"`)},
		{name: "float32-short", item: float32(123), tags: reflect.StructTag(`flag:"float32-short" flag-short:"s"`)},
		{name: "float64", item: float64(123), tags: reflect.StructTag(`flag:"float64"`)},
		{name: "float64-short", item: float64(123), tags: reflect.StructTag(`flag:"float64-short" flag-short:"s"`)},
		{name: "duration", item: time.Second * 123, tags: reflect.StructTag(`flag:"duration"`)},
		{name: "duration-short", item: time.Second * 15, tags: reflect.StructTag(`flag:"duration-short" flag-short:"s"`)},
		{name: "ip", item: net.ParseIP("127.0.0.1"), tags: reflect.StructTag(`flag:"ip"`)},
		{name: "ip-short", item: net.ParseIP("128.0.0.1"), tags: reflect.StructTag(`flag:"ip" flag-short:"s"`)},
		{name: "ipnet", item: net.IPNet{IP: net.ParseIP("128.0.0.0"), Mask: net.CIDRMask(24, 32)}, tags: reflect.StructTag(`flag:"ipnet"`)},
		{name: "ipnet-short", item: net.IPNet{IP: net.ParseIP("128.0.0.0"), Mask: net.CIDRMask(24, 32)}, tags: reflect.StructTag(`flag:"ipnet" flag-short:"s"`)},
		{name: "ip-mask", item: net.CIDRMask(16, 32), tags: reflect.StructTag(`flag:"ip-mask"`)},
		{name: "ip-mask-short", item: net.CIDRMask(16, 32), tags: reflect.StructTag(`flag:"ip-mask" flag-short:"s"`)},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			example := reflect.New(reflect.StructOf([]reflect.StructField{{
				Name: testField,
				Tag:  tt.tags,
				Type: reflect.TypeOf(tt.item),
			}}))

			field := reflect.New(reflect.TypeOf(tt.item)).Elem()
			field.Set(reflect.ValueOf(tt.item))
			info := reflect.StructField{Name: tt.name, Tag: tt.tags}

			flagName := info.Tag.Get(gonfig.FlagTag)
			flagValue := fmt.Sprintf("%v", tt.item)
			if str, ok := tt.item.(fmt.Stringer); ok {
				flagValue = str.String()
			} else if str, ok = field.Addr().Interface().(fmt.Stringer); ok {
				flagValue = str.String()
			}

			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			require.NoError(t, gonfig.PrepareFlags(flags, example.Interface()), flagName)

			var args []string
			if short := info.Tag.Get(gonfig.FlagTagShort); short != "" {
				args = append(args, "-"+short, flagValue)
			} else {
				args = append(args, "--"+flagName, flagValue)
			}

			field.SetZero()

			require.NoError(t, flags.Parse(args), "HELP:\n"+flags.FlagUsages())
			if str, ok := example.Elem().FieldByName(testField).Addr().Interface().(fmt.Stringer); ok {
				tmp := reflect.New(field.Type()).Elem()
				tmp.Set(reflect.ValueOf(tt.item))

				out := tmp.Addr().Interface().(fmt.Stringer)
				require.Equal(t, out.String(), str.String())
			} else {
				require.Equal(t, tt.item, example.Elem().FieldByName(testField).Interface())
			}
		})
	}
}

func TestPrepareFlag_Slices(t *testing.T) {
	ip1 := net.ParseIP("127.0.0.1")
	ip2 := net.ParseIP("127.0.0.2")
	ip3 := net.ParseIP("127.0.0.3")

	sec1 := time.Second * 123
	sec2 := time.Second * 456
	sec3 := time.Second * 789

	cases := []struct {
		item any
		name string
		tags reflect.StructTag
	}{
		{name: "slice-bool", item: []bool{true, false, true}, tags: reflect.StructTag(`flag:"slice-bool"`)},
		{name: "slice-bool-short", item: []bool{true, false, true}, tags: reflect.StructTag(`flag:"slice-bool-short" flag-short:"s"`)},
		{name: "slice-string", item: []string{"foo", "bar"}, tags: reflect.StructTag(`flag:"slice-string"`)},
		{name: "slice-string-short", item: []string{"bar", "foo"}, tags: reflect.StructTag(`flag:"slice-string" flag-short:"s"`)},
		{name: "slice-int", item: []int{1, 2, 3}, tags: reflect.StructTag(`flag:"slice-int"`)},
		{name: "slice-int-short", item: []int{3, 4, 5}, tags: reflect.StructTag(`flag:"slice-int" flag-short:"s"`)},
		{name: "slice-int32", item: []int32{5, 6, 7}, tags: reflect.StructTag(`flag:"slice-int32"`)},
		{name: "slice-int32-short", item: []int32{5, 6, 7}, tags: reflect.StructTag(`flag:"slice-int32" flag-short:"s"`)},
		{name: "slice-int64", item: []int64{5, 6, 7}, tags: reflect.StructTag(`flag:"slice-int64"`)},
		{name: "slice-int64-short", item: []int64{5, 6, 7}, tags: reflect.StructTag(`flag:"slice-int64" flag-short:"s"`)},
		{name: "slice-float32", item: []float32{5.1, 6.2, 7.3}, tags: reflect.StructTag(`flag:"slice-float32"`)},
		{name: "slice-float32-short", item: []float32{5.2, 6.3, 7.4}, tags: reflect.StructTag(`flag:"slice-float32" flag-short:"s"`)},
		{name: "slice-float64", item: []float64{5.3123, 6.4123, 7.5123}, tags: reflect.StructTag(`flag:"slice-float64"`)},
		{name: "slice-float64-short", item: []float64{5.51234, 6.612345, 7.7123456}, tags: reflect.StructTag(`flag:"slice-float64" flag-short:"s"`)},
		{name: "slice-ip", item: []net.IP{ip1, ip2, ip3}, tags: reflect.StructTag(`flag:"slice-ip"`)},
		{name: "slice-ip-short", item: []net.IP{ip2, ip3, ip1}, tags: reflect.StructTag(`flag:"slice-ip" flag-short:"s"`)},
		{name: "slice-duration", item: []time.Duration{sec1, sec3, sec2}, tags: reflect.StructTag(`flag:"slice-duration"`)},
		{name: "slice-duration-short", item: []time.Duration{sec3, sec1, sec2}, tags: reflect.StructTag(`flag:"slice-duration" flag-short:"s"`)},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			example := reflect.New(reflect.StructOf([]reflect.StructField{{
				Name: testField,
				Tag:  tt.tags,
				Type: reflect.TypeOf(tt.item),
			}}))

			field := reflect.New(reflect.TypeOf(tt.item)).Elem()
			field.Set(reflect.ValueOf(tt.item))
			info := reflect.StructField{Name: tt.name, Tag: tt.tags}

			flagName := info.Tag.Get(gonfig.FlagTag)
			var args []string
			{ // prepare args
				slice := reflect.New(reflect.TypeOf(tt.item)).Elem()
				slice.Set(reflect.ValueOf(tt.item))

				for i := 0; i < slice.Len(); i++ {
					flagValue := fmt.Sprintf("%v", slice.Index(i))
					if str, ok := slice.Index(i).Addr().Interface().(fmt.Stringer); ok {
						flagValue = str.String()
					}

					if short := info.Tag.Get(gonfig.FlagTagShort); short != "" {
						args = append(args, "-"+short, flagValue)
					} else {
						args = append(args, "--"+flagName, flagValue)
					}
				}
			}

			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			require.NoError(t, gonfig.PrepareFlags(flags, example.Interface()), flagName)

			field.SetZero()
			require.NoError(t, flags.Parse(args), "args: %v", args)
			require.Equal(t, tt.item, example.Elem().FieldByName(testField).Interface())
		})
	}

	t.Run("slice-bytes", func(t *testing.T) {
		encoders := map[string]func([]byte) string{
			gonfig.FlagHEX: hex.EncodeToString,
			gonfig.FlagB64: base64.StdEncoding.EncodeToString,
		}

		for _, base := range []string{"hex", "b64"} {
			t.Run(base, func(t *testing.T) {
				for _, flag := range []string{"flag", "flag-short"} {
					t.Run(flag, func(t *testing.T) {
						var short string
						if flag == gonfig.FlagTagShort {
							short = gonfig.FlagTagShort + `:"s"`
						}

						item := []byte("hello world")

						field := reflect.New(reflect.TypeOf(item)).Elem()
						field.Set(reflect.ValueOf(item))
						info := reflect.StructField{
							Name: "slice-bytes",
							Tag:  reflect.StructTag(`flag:"slice-bytes,` + base + `" ` + short)}

						flagName := info.Tag.Get(gonfig.FlagTag)
						options := strings.Split(flagName, ",")
						flagName = options[0]

						example := reflect.New(reflect.StructOf([]reflect.StructField{{
							Name: testField,
							Tag:  info.Tag,
							Type: field.Type(),
						}}))

						var args []string
						if shorter := info.Tag.Get(flag); flag == gonfig.FlagTagShort {
							args = append(args, "-"+shorter, encoders[base](item))
						} else {
							args = append(args, "--"+flagName, encoders[base](item))
						}

						t.Logf("run [%s][%s] => %v", flag, base, args)

						flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
						require.NoError(t, gonfig.PrepareFlags(flags, example.Interface()), flagName)

						field.SetZero()
						require.NoError(t, flags.Parse(args), "args: %v", args)
						require.Equal(t, item, example.Elem().FieldByName(testField).Interface())
					})
				}
			})
		}
	})
}

func TestPrepareFlags_Nested(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)

	config := NestedFlagConfig{}
	require.NoError(t, gonfig.PrepareFlags(flagSet, &config))

	require.NoError(t, flagSet.Parse([]string{
		"--debug",
		"--port", "8080",
		"--name", "test-service",
		"--ip", "127.0.0.1",
		"--ip-mask", "255.255.255.0"}))

	// check flags
	if !config.Debug {
		t.Errorf("expected debug to be true, got %v", config.Debug)
	}
	if config.Port != 8080 {
		t.Errorf("expected port to be 8080, got %d", config.Port)
	}
	if config.Name != "test-service" {
		t.Errorf("expected name to be 'test-service', got %s", config.Name)
	}
	if config.IP.String() != "127.0.0.1" {
		t.Errorf("expected IP to be '127.0.0.1', got %s", config.IP.String())
	}
	if config.IPMask.String() != "ffffff00" {
		t.Errorf("expected IPMask to be '255.255.255.0', got %v", config.IPMask.String())
	}
}

func TestPrepareFlags_Errors(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)

	// wrong dest (not pointer)
	require.Error(t, gonfig.PrepareFlags(flagSet, TestFlagConfig{}))

	// wrong dest (not structure)
	require.Error(t, gonfig.PrepareFlags(flagSet, new(int)))

	// unknown type
	require.Error(t, gonfig.PrepareFlags(flagSet, &struct {
		UnknownType []complex64 `flag:"complex"`
	}{}))

	// unknown []byte decoder
	require.Error(t, gonfig.PrepareFlags(flagSet, &struct {
		UnknownType []byte `flag:"slice-byte,unknown"`
	}{}))

	// error in nested struct
	require.Error(t, gonfig.PrepareFlags(flagSet, &struct {
		ErrorType struct {
			Field []byte `flag:"slice-byte,unknown"`
		}
	}{}))
}
