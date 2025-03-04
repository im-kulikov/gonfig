package gonfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ReflectStruct struct {
	StringField string `env:"string_field"`

	unexportedField int
	ExportedPointer *int

	NestedReflectField struct {
		NestedStringField string `env:"nested_string_field"`
	} `env:"nested_field"`

	EmbedReflect `env:"embed"`

	unexportedStruct reflectNestedStruct
}

type EmbedReflect struct {
	EmbedStringField string `env:"embed_string_field"`
}

type reflectNestedStruct struct {
	AnotherField int
}

func TestReflectFieldsOf(t *testing.T) {
	t.Run("non-pointer", func(t *testing.T) {
		for _, err := range ReflectFieldsOf(ReflectStruct{}, ReflectOptions{}) {
			require.ErrorContains(t, err, ErrExpectPointer.Error())
		}
	})

	t.Run("non-struct", func(t *testing.T) {
		for _, err := range ReflectFieldsOf(new(int), ReflectOptions{}) {
			require.ErrorContains(t, err, ErrExpectStruct.Error())
		}
	})

	t.Run("fields", func(t *testing.T) {
		cases := []struct {
			Count   int
			Options ReflectOptions
		}{
			{Count: 4, Options: ReflectOptions{CanSet: True()}},
			{Count: 6, Options: ReflectOptions{CanAddr: True()}},
			{Count: 4, Options: ReflectOptions{CanInterface: True()}},
			{Count: 0, Options: ReflectOptions{CanSet: True(), CanAddr: False()}},
			{Count: 0, Options: ReflectOptions{CanSet: True(), CanInterface: False()}},
		}

		for i, tt := range cases {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				var example ReflectStruct

				var output []*ReflectValue
				for elem, err := range ReflectFieldsOf(&example, tt.Options) {
					assert.NoError(t, err)

					output = append(output, elem)
				}

				require.Len(t, output, tt.Count)
			})
		}
	})
}
