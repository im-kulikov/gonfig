package gonfig_test

import (
	"errors"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
)

// Sample struct for testing
type TestStruct struct {
	StringField     string         `default:"defaultString"`
	IntField        int            `default:"42"`
	FloatField      float64        `default:"3.14"`
	BoolField       bool           `default:"true"`
	UintField       uint           `default:"64"`
	ComplexField64  complex64      `default:"(64+3i)"`
	ComplexField128 complex128     `default:"(128+3i)"`
	SliceField      []int          `default:"1,2,3,,"`
	ArrayField      [3]int         `default:",5,6"`
	SliceBytes      []byte         `default:"255"`
	MapField        map[string]int `default:"key1:100,key2:200"`
	PointerField    *int           `default:"99"`

	DurationField  time.Duration `default:"10s"`
	NetIPNetField  net.IPNet     `default:"192.168.0.0/24"`
	NetIPField     net.IP        `default:"192.168.20.100"`
	NetIPMaskField net.IPMask    `default:"/30"`

	StructField NestedStruct
}

// Nested struct for testing
type NestedStruct struct {
	NestedStringField  string `default:"nestedString"`
	TextUnmarshalField custom `default:"textUnmarshal"`
}

type custom struct{ inner string }

func (c *custom) UnmarshalText(text []byte) error {
	c.inner = string(text)

	return nil
}

// Test to validate default setting logic
func TestSetDefaults(t *testing.T) {
	dest := &TestStruct{}
	require.NoError(t, gonfig.SetDefaults(dest))

	// Check each field
	if dest.StringField != "defaultString" {
		t.Errorf("Expected StringField to be 'defaultString', got '%s'", dest.StringField)
	}

	if dest.IntField != 42 {
		t.Errorf("Expected IntField to be 42, got %d", dest.IntField)
	}

	if dest.FloatField != 3.14 {
		t.Errorf("Expected FloatField to be 3.14, got %f", dest.FloatField)
	}

	if !dest.BoolField {
		t.Errorf("Expected BoolField to be true, got false")
	}

	if dest.UintField != 64 {
		t.Errorf("Expected UintField to be 64, got %d", dest.UintField)
	}
	if dest.ComplexField64 != complex(64, 3) {
		t.Errorf("Expected ComplexField to be (64+3i), got %v", dest.ComplexField64)
	}

	if dest.ComplexField128 != complex(128, 3) {
		t.Errorf("Expected ComplexField to be (128+3i), got %v", dest.ComplexField128)
	}

	if len(dest.SliceField) != 3 || dest.SliceField[0] != 1 || dest.SliceField[1] != 2 || dest.SliceField[2] != 3 {
		t.Errorf("Expected SliceField to be [1,2,3], got %v", dest.SliceField)
	}

	if dest.ArrayField[0] != 0 || dest.ArrayField[1] != 5 || dest.ArrayField[2] != 6 {
		t.Errorf("Expected ArrayField to be [4,5,6], got %v", dest.ArrayField)
	}

	if dest.MapField["key1"] != 100 || dest.MapField["key2"] != 200 {
		t.Errorf("Expected MapField to be map[key1:100 key2:200], got %v", dest.MapField)
	}

	if dest.StructField.NestedStringField != "nestedString" {
		t.Errorf("Expected NestedStringField to be 'nestedString', got '%s'", dest.StructField.NestedStringField)
	}

	if dest.PointerField == nil || *dest.PointerField != 99 {
		t.Errorf("Expected PointerField to be 99, got %v", dest.PointerField)
	}

	if dest.DurationField != 10*time.Second {
		t.Errorf("Expected DurationField to be 10s, got %v", dest.DurationField)
	}

	if dest.NetIPField.String() != "192.168.20.100" {
		t.Errorf("Expected NetIPField to be 192.168.20.100, got %v", dest.NetIPField)
	}

	if one, size := dest.NetIPMaskField.Size(); one != 30 && size != 32 {
		t.Errorf("Expected NetIPMaskField to be /30, got %v (%d/%d)", dest.NetIPMaskField, one, size)
	}

	if dest.NetIPNetField.String() != "192.168.0.0/24" {
		t.Errorf("Expected NetIPNetField to be 192.168.0.0/24, got %v", dest.NetIPNetField)
	}
}

// Additional test cases for error scenarios
func TestSetDefaultValueErrors(t *testing.T) {
	cases := []any{
		0, int8(2), int16(3), int32(4), int64(5),
		uint(6), uint8(7), uint16(8), uint32(9), uint64(10),
		float32(11), float64(12),
		complex64(13), complex128(14),
		true,
		new(int),
		[]int{},
		[3]int{},
		map[int]int{},
		map[string]int{},
	}

	for _, tt := range cases {
		t.Run(reflect.TypeOf(tt).String(), func(t *testing.T) {

			kind := reflect.StructOf([]reflect.StructField{{
				Name: "SomeField",
				Type: reflect.TypeOf(tt),
				Tag:  `default:"1:1,2,str:invalid"`,
			}})

			example := reflect.New(kind).Interface()
			err := gonfig.SetDefaults(example)
			require.Error(t, err)

			var out *strconv.NumError
			require.True(t, errors.As(err, &out), err)
			require.EqualError(t, out.Err, "invalid syntax")
		})
	}

	t.Run("array", func(t *testing.T) {
		kind := reflect.StructOf([]reflect.StructField{{
			Name: "SomeField",
			Type: reflect.TypeOf([3]int{}),
			Tag:  `default:"1,2,3,4"`,
		}})

		example := reflect.New(kind).Interface()
		err := gonfig.SetDefaults(example)
		require.Error(t, err)
		require.ErrorContains(t, err, "array length exceeds 3 elements")
	})

	t.Run("duration", func(t *testing.T) {
		kind := reflect.StructOf([]reflect.StructField{{
			Name: "SomeField",
			Type: reflect.TypeOf(time.Second),
			Tag:  `default:"70invalid"`,
		}})

		example := reflect.New(kind).Interface()
		err := gonfig.SetDefaults(example)
		require.Error(t, err)
		require.ErrorContains(t, err, `unknown unit "invalid" in duration "70invalid"`)
	})

	t.Run("net-types", func(t *testing.T) {
		t.Run("net.IPMask", func(t *testing.T) {
			kind := reflect.StructOf([]reflect.StructField{{
				Name: "SomeField",
				Type: reflect.TypeOf(net.IPMask{}),
				Tag:  `default:"invalid"`,
			}})

			example := reflect.New(kind).Interface()
			err := gonfig.SetDefaults(example)
			require.Error(t, err)
			require.ErrorContains(t, err, `parsing "invalid": invalid syntax`)
		})

		t.Run("net.IP", func(t *testing.T) {
			kind := reflect.StructOf([]reflect.StructField{{
				Name: "SomeField",
				Type: reflect.TypeOf(net.IP{}),
				Tag:  `default:"invalid"`,
			}})

			example := reflect.New(kind).Interface()
			err := gonfig.SetDefaults(example)
			require.Error(t, err)
			require.ErrorContains(t, err, `invalid IP address "invalid"`)
		})

		t.Run("net.IP", func(t *testing.T) {
			kind := reflect.StructOf([]reflect.StructField{{
				Name: "SomeField",
				Type: reflect.TypeOf(net.IPNet{}),
				Tag:  `default:"invalid"`,
			}})

			example := reflect.New(kind).Interface()
			err := gonfig.SetDefaults(example)
			require.Error(t, err)
			require.ErrorContains(t, err, `invalid CIDR address: invalid`)
		})
	})

	t.Run("unknown type", func(t *testing.T) {
		var out struct {
			Field any `default:"1:100,2:200,3"`
		}

		require.ErrorContains(t, gonfig.SetDefaults(&out), "unsupported type")
	})

	t.Run("expect errors", func(t *testing.T) {
		require.EqualError(t, gonfig.SetDefaults(struct{}{}), "(defaults) expect pointer, got \"struct\"")
		require.EqualError(t, gonfig.SetDefaults(new(int)), "(defaults) expect struct field, got \"int\"")
	})
}
