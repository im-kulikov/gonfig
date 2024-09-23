package gonfig_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
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
		for _, err := range gonfig.ReflectFieldsOf(ReflectStruct{}, gonfig.ReflectOptions{}) {
			require.ErrorContains(t, err, gonfig.ErrExpectPointer.Error())
		}
	})

	t.Run("non-struct", func(t *testing.T) {
		for _, err := range gonfig.ReflectFieldsOf(new(int), gonfig.ReflectOptions{}) {
			require.ErrorContains(t, err, gonfig.ErrExpectStruct.Error())
		}
	})

	t.Run("fields", func(t *testing.T) {
		cases := []struct {
			Count   int
			Options gonfig.ReflectOptions
		}{
			{Count: 4, Options: gonfig.ReflectOptions{CanSet: gonfig.True()}},
			{Count: 6, Options: gonfig.ReflectOptions{CanAddr: gonfig.True()}},
			{Count: 4, Options: gonfig.ReflectOptions{CanInterface: gonfig.True()}},
			{Count: 0, Options: gonfig.ReflectOptions{CanSet: gonfig.True(), CanAddr: gonfig.False()}},
			{Count: 0, Options: gonfig.ReflectOptions{CanSet: gonfig.True(), CanInterface: gonfig.False()}},
		}

		for i, tt := range cases {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				var example ReflectStruct

				var output []*gonfig.ReflectValue
				for elem, err := range gonfig.ReflectFieldsOf(&example, tt.Options) {
					assert.NoError(t, err)

					output = append(output, elem)
				}

				require.Len(t, output, tt.Count)
			})
		}
	})
}
