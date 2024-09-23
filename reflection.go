package gonfig

import (
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
)

// ReflectValue represents a structure for working with a field of a struct in reflection.
// It contains the reflect.Value of the field, its associated struct field metadata, and
// a reference to its owner (the parent struct field, if applicable).
type ReflectValue struct {
	Value reflect.Value       // The reflected value of the field.
	Field reflect.StructField // Metadata about the struct field (name, type, etc.).
	Owner *ReflectValue       // Pointer to the owner (parent) ReflectValue, if applicable.
}

// ReflectOptions defines options for reflecting on fields of a struct.
// These options specify conditions that determine which fields to include in the reflection process.
type ReflectOptions struct {
	CanAddr      *bool // Only include fields that can be addressed (pointer to the field can be taken).
	CanSet       *bool // Only include fields that can be set (modifiable).
	CanInterface *bool // Only include fields that can be interfaced (exposed as an interface{}).

	AsField []reflect.Type
}

// TagOptions represents the configuration options for processing struct tags and flags.
// It is used to define metadata for struct fields, including encoding details, flag names, and validation rules.
//
// Fields:
// - FlagEncodeBase: Defines the base encoding format for the field (e.g., base64, base32).
// - FlagFullName: Specifies the full name of the flag associated with the field.
// - FlagShortName: Specifies the short or abbreviated name of the flag.
// - FlagConfig: Indicates whether the field should be loaded from a configuration file or environment variable.
// - FieldRequired: Specifies if the field is mandatory and should not be left empty.
// - FieldUsage: Provides a description of the field’s purpose, typically used for generating usage/help information.
// - tag: Internal representation of the field's struct tag, used for reflection operations.
//
// This struct is typically used for flag parsing, configuration, and validation purposes in applications.
type TagOptions struct {
	FlagEncodeBase string
	FlagFullName   string
	FlagShortName  string
	FlagConfig     bool
	FieldRequired  bool
	FieldUsage     string

	tag reflect.StructTag
}

// Error constants for reflection-related operations.
const (
	// ErrExpectStruct is returned when a struct field is expected but the provided value is not a struct.
	ErrExpectStruct = constantError("expect struct field")

	// ErrExpectPointer is returned when a pointer is expected but the provided value is not a pointer.
	ErrExpectPointer = constantError("expect pointer")
)

// ParseTagOptions parses a reflect.StructTag and extracts relevant options into a TagOptions struct.
// It processes the tag string to identify various flag configurations such as the full name, short name,
// encoding base, and whether the field is required or configurable via a config file.
//
// The function expects certain tag formats, such as:
// - Full flag name as the first element in the tag, separated by a comma.
// - Optional "base:" prefix to define the encoding format (e.g., base64, base32).
// - Optional "short:" prefix to define a short flag name.
// - "config:true" to indicate that the flag can be loaded from a configuration file.
// - Required status is determined by the "RequiredTag" with the value "true".
//
// Parameters:
// - tag: A `reflect.StructTag` representing the field's tag in the struct.
//
// Returns:
// - A TagOptions struct populated with the parsed information.
//
// Example tag format: `flag:"flagName,base:base64,short:f,config:true" usage:"field usage" required:"true"`
func ParseTagOptions(tag reflect.StructTag) TagOptions {
	flag := tag.Get(FlagTag)

	tmp := strings.Split(flag, ",")

	opt := TagOptions{
		FlagFullName:  tmp[0],
		FieldUsage:    tag.Get(FlagTagUsage),
		FieldRequired: tag.Get(RequiredTag) == "true",
		tag:           tag,
	}

	for _, elem := range tmp {
		if strings.HasPrefix(elem, "base:") {
			opt.FlagEncodeBase = strings.TrimSpace(elem[len("base:"):])
			continue
		}

		if strings.HasPrefix(elem, "short:") {
			opt.FlagShortName = strings.TrimSpace(elem[len("short:"):])
			continue
		}

		if strings.EqualFold(elem, "config:true") {
			opt.FlagConfig = true
		}
	}

	return opt
}

// Ptr takes a value of any comparable type and returns a pointer to that value.
// This function is useful for quickly obtaining a pointer to a literal or value,
// especially in situations where you need a pointer for use in data structures or APIs.
//
// Example usage:
//
//	i := Ptr(42)       // Returns a pointer to an integer 42
//	s := Ptr("hello")  // Returns a pointer to the string "hello"
//
// E must be a type that supports the comparable constraint.
func Ptr[E comparable](v E) *E { return &v }

// True returns a pointer to a boolean value set to true.
// It utilizes the Ptr function to quickly generate a pointer for the boolean literal true.
//
// Example usage:
//
//	b := True()  // Returns *bool pointing to true
func True() *bool { return Ptr(true) }

// False returns a pointer to a boolean value set to false.
// Like True, it uses the Ptr function to generate a pointer to the boolean literal false.
//
// Example usage:
//
//	b := False()  // Returns *bool pointing to false
func False() *bool { return Ptr(false) }

// IsValid checks whether a reflect.Value satisfies the conditions specified in the ReflectOptions.
// It validates whether the value's CanSet, CanAddr, and CanInterface properties match the corresponding
// constraints set in the ReflectOptions. If any constraint is not met, the method returns false.
//
// Parameters:
// - v: A `reflect.Value` representing the field or value to be validated.
//
// Returns:
// - true if the reflect.Value meets all the conditions specified in the ReflectOptions.
// - false otherwise.
//
// Example usage:
//
//	options := ReflectOptions{CanSet: Ptr(true)}
//	valid := options.IsValid(reflect.ValueOf(someField))  // returns true/false based on validation.
func (o *ReflectOptions) IsValid(v reflect.Value) bool {
	if o.CanSet != nil && v.CanSet() != *o.CanSet {
		return false
	}

	if o.CanAddr != nil && v.CanAddr() != *o.CanAddr {
		return false
	}

	if o.CanInterface != nil && v.CanInterface() != *o.CanInterface {
		return false
	}

	return true
}

// IsField checks whether the provided reflect.Value represents a valid field for reflection operations
// based on the ReflectOptions. It compares the value's type against the AsField slice in the ReflectOptions.
//
// The method returns true if the value's type is explicitly listed in the AsField slice or if it is not a struct.
// If the value is a struct and does not match any type in AsField, the method returns false.
//
// Parameters:
// - v: A `reflect.Value` to check if it should be treated as a field.
//
// Returns:
// - true if the value is considered a field, or false if it's a struct that should not be treated as a field.
//
// Example usage:
//
//	options := ReflectOptions{AsField: []reflect.Type{reflect.TypeOf(int(0))}}
//	isField := options.IsField(reflect.ValueOf(someField))  // returns true/false based on field type.
func (o *ReflectOptions) IsField(v reflect.Value) bool {
	if slices.Contains(o.AsField, v.Type()) {
		return true
	}

	switch v.Kind() {
	case reflect.Struct:
		return false
	default:
		return true
	}
}

// ReflectFieldsOf returns an iterator that reflects over all fields of a given struct (or struct-like type).
// `in` must be a pointer to a struct, and `options` controls which fields are included in the reflection.
// The iterator yields `*ReflectValue` for each field, or an error if a problem occurs.
func ReflectFieldsOf(in any, options ReflectOptions) iter.Seq2[*ReflectValue, error] {
	return func(yield func(*ReflectValue, error) bool) {
		v := reflect.ValueOf(in)

		// Check if the input is a pointer. If not, yield an error.
		if v.Kind() != reflect.Ptr {
			yield(nil, fmt.Errorf("%w, got %q", ErrExpectPointer, v.Kind()))

			return
		}

		// Check if the underlying type is a struct. If not, yield an error.
		if v.Elem().Kind() != reflect.Struct {
			yield(nil, fmt.Errorf("%w, got %q", ErrExpectStruct, v.Elem().Kind()))

			return
		}

		structs := []*ReflectValue{{Value: v.Elem()}}

	loop: // Start reflecting over the structs fields recursively.
		for j := 0; j < len(structs); j++ {
			elem := structs[j]

			for i := range elem.Value.NumField() {
				fv := elem.Value.Field(i) // Get the field's reflect.Value.

				// Apply filtering based on the provided ReflectOptions.
				if !options.IsValid(fv) {
					continue
				}

				if !options.IsField(fv) {
					// Recursively handle nested structs. If yielding returns false, stop iteration.
					structs = append(structs, &ReflectValue{Value: fv, Field: elem.Value.Type().Field(i), Owner: elem})

					continue
				}

				if !yield(&ReflectValue{Value: fv, Owner: elem, Field: elem.Value.Type().Field(i)}, nil) {
					break loop
				}
			}
		}
	}
}
