package gonfig_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/gonfig"
)

// Example nested structures used for testing
type Address struct {
	City    string `required:"true" json:"city"`
	Country string `required:"true" env:"COUNTRY"`
}

type User struct {
	Name    string  `required:"true" json:"name"`
	Email   string  `required:"true" flag:"email"`
	Age     int     `required:"false"`
	Address Address // Nested struct with required fields
	IP      net.IP  `required:"true" flag:"ip"`
}

type NoRequiredFields struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

type EmptyStruct struct {
	Field1 string `required:"true" json:"field1"`
	Field2 int    `required:"true" json:"field2"`
}

type NestedAnonymous struct {
	Field1    string `required:"true" json:"field1"`
	Anonymous struct {
		Field2 string `required:"true" json:"field2"`
	} `required:"true"`
}

// TestValidateRequiredFields tests ValidateRequiredFields function using various test cases.
func TestValidateRequiredFields(t *testing.T) {
	testCases := []struct {
		name    string
		input   any
		wantErr bool
		errMsg  string
	}{
		{
			name: "All required fields are filled",
			input: &User{
				Name:  "John",
				Email: "john@example.com",
				Age:   25,
				Address: Address{
					City:    "New York",
					Country: "USA",
				},
				IP: net.ParseIP("127.0.0.1"),
			},
			wantErr: false,
		},
		{
			name:    "Missing required fields",
			input:   &User{},
			wantErr: true,
			errMsg:  "missing required fields:\n\t- field `Name` <string> is required\n\t- field `Email` <string> is required\n\t- field `IP` <net.IP> is required\n\t- field `City` <string> in path `Address.City` is required\n\t- field `Country` <string> in path `Address.Country` is required",
		},
		{
			name: "Missing nested structure required fields",
			input: &User{
				Name:    "Alice",
				Email:   "alice@example.com",
				Address: Address{Country: "Canada"},
				IP:      net.ParseIP("127.0.0.1"),
			},
			wantErr: true,
			errMsg:  "missing required fields:\n\t- field `City` <string> in path `Address.City` is required",
		},
		{
			name: "Struct with no required fields",
			input: &NoRequiredFields{
				Field1: "test",
				Field2: 123,
			},
			wantErr: false,
		},
		{
			name:    "Empty struct",
			input:   &EmptyStruct{},
			wantErr: true,
			errMsg:  "missing required fields:\n\t- field `Field1` <string> is required\n\t- field `Field2` <int> is required",
		},
		{
			name: "Required field in nested anonymous struct",
			input: &NestedAnonymous{
				Field1: "test",
				Anonymous: struct {
					Field2 string `required:"true" json:"field2"`
				}{},
			},
			wantErr: true,
			errMsg:  "missing required fields:\n\t- field `Field2` <string> in path `Anonymous.Field2` is required",
		},
		{
			name:    "Non-pointer input",
			input:   User{Name: "John"},
			wantErr: true,
			errMsg:  "(require) expect pointer, got \"struct\"",
		},
		{
			name:    "Non-struct pointer",
			input:   new(int),
			wantErr: true,
			errMsg:  "(require) expect struct field, got \"int\"",
		},
		{
			name:    "Pointer to a nil struct",
			input:   (*User)(nil),
			wantErr: true,
			errMsg:  "(require) expect struct field, got \"invalid\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := gonfig.ValidateRequiredFields(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, tc.errMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
