package gonfig

import (
	"encoding"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ErrEnvSetterBreak is a predefined constant of type Error
// used to indicate an error or a condition where processing should stop.
const ErrEnvSetterBreak = Error("break")

// defaultTagName defines the struct tag key used to specify default values for struct fields.
// When parsing struct tags, this key indicates the default value to be used if no value is provided
// (e.g., from an environment variable or configuration).
//
// Example usage: `default:"localhost"`
// This would set the field to "localhost" if no other value is provided.
const defaultTagName = "default"

// newDefaultParser creates a new parser for handling default values.
// It returns a Parser implementation that sets default values to struct fields
// based on the "default" struct tags.
func newDefaultParser() *parserFunc {
	return &parserFunc{name: ParserDefaults, call: SetDefaults}
}

// SetDefaults sets default values to the fields of the provided struct.
// It recursively processes struct fields and assigns default values based on
// the "default" tag. It supports setting values for basic types, slices, arrays, maps,
// and custom unmarshalling for types implementing encoding.TextUnmarshaler.
// Returns an error if the destination is not a pointer or if setting a default value fails.
func SetDefaults(dest any) error {
	types := []reflect.Type{reflect.TypeFor[net.IPNet]()}
	for elem, err := range ReflectFieldsOf(dest, ReflectOptions{CanAddr: True(), AsField: types}) {
		if err != nil {
			return fmt.Errorf("(defaults) %w", err)
		}

		if err = applyDefault(elem.Value, elem.Field.Tag.Get(defaultTagName)); err != nil {
			return fmt.Errorf("(defaults) failed to set field %q: %w", elem.Field.Name, err)
		}
	}

	return nil
}

// setDefaultMap parses a map from key:value pairs separated by commas. The first ":"
// splits a pair, so a value may contain it: api:http://host:8080.
func setDefaultMap(field reflect.Value, value string) error {
	result := reflect.MakeMap(field.Type())

	for item := range strings.SplitSeq(value, ",") {
		if item == "" {
			continue
		}

		name, text, ok := strings.Cut(item, ":")
		if !ok {
			return fmt.Errorf("could not parse %q: a map entry is key:value", item)
		}

		key, err := parseDefault(field.Type().Key(), name)
		if err != nil {
			return err
		}

		val, err := parseDefault(field.Type().Elem(), text)
		if err != nil {
			return fmt.Errorf("value of key %q: %w", name, err)
		}

		result.SetMapIndex(key, val)
	}

	field.Set(result)

	return nil
}

// parseDefault parses an item of a list or a map, of type t, with the rules of the
// `default` tag: durations, IP types and encoding.TextUnmarshaler included.
func parseDefault(t reflect.Type, item string) (reflect.Value, error) {
	value := reflect.New(t).Elem()
	if err := applyDefault(value, item); err != nil {
		return value, fmt.Errorf("could not parse %q: %w", item, err)
	}

	return value, nil
}

// applyDefault sets the value of a `default` tag to an empty field.
func applyDefault(field reflect.Value, value string) error {
	if err := tryCustomTypes(field, value); errors.Is(err, ErrEnvSetterBreak) {
		return nil
	} else if err != nil {
		return err
	}

	return setDefaultValue(field, value)
}

// getTextUnmarshaler checks if the field can be converted to encoding.TextUnmarshaler.
// It first tries to get the addressable version of the field, and if that's not possible,
// it attempts to assert the interface directly from the field's value.
func getTextUnmarshaler(field reflect.Value) (encoding.TextUnmarshaler, bool) {
	if ca, ci := field.CanAddr(), field.CanInterface(); ca && ci {
		if textUnmarshaler, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return textUnmarshaler, true
		}
	} else if !ci {
		return nil, false
	}

	textUnmarshaler, ok := field.Interface().(encoding.TextUnmarshaler)

	return textUnmarshaler, ok
}

// tryCustomTypes attempts to set the value of a `reflect.Value` field based on its type.
// It handles encoding.TextUnmarshaler (net.IP, slog.Level, ...), time.Duration, net.IPMask and net.IPNet.
// If the value is not empty and the field is not yet set (IsZero), it processes the value.
func tryCustomTypes(field reflect.Value, value any) error {
	// If the value is empty or the field already has a value, return early with no error.
	if value == "" || !field.IsZero() {
		return nil
	}

	if setter, ok := getTextUnmarshaler(field); ok {
		if err := setter.UnmarshalText([]byte(value.(string))); err != nil {
			return err
		}

		// Done: a value parsed to zero (slog.LevelInfo) must not be parsed again as a plain type.
		return ErrEnvSetterBreak
	}

	// Switch on the underlying type of the field and handle specific custom types.
	switch field.Interface().(type) {
	default:
		// For unsupported types, return nil without any changes.
		return nil
	case time.Duration:
		// If the field is time.Duration, parse the value as a duration string.
		val, err := time.ParseDuration(value.(string))
		if err != nil {
			return err // Return error if parsing fails.
		}
		// Set the parsed duration to the field.
		field.Set(reflect.ValueOf(val))
	case net.IPMask:
		// A prefix length, "/24" or "24": up to 32 an IPv4 mask, up to 128 an IPv6 one.
		prefix, err := strconv.Atoi(strings.TrimPrefix(value.(string), "/"))
		if err != nil {
			return err
		}

		bits := 8 * net.IPv4len
		if prefix > bits {
			bits = 8 * net.IPv6len
		}

		mask := net.CIDRMask(prefix, bits)
		if mask == nil {
			return fmt.Errorf("invalid mask %q: the prefix is 0 to 128", value)
		}

		field.Set(reflect.ValueOf(mask))
	case net.IPNet:
		// If the field is net.IPNet, parse the value as a CIDR notation string.
		_, val, err := net.ParseCIDR(value.(string))
		if err != nil {
			return err // Return error if parsing fails.
		}
		// Set the parsed IP network (CIDR) to the field.
		field.Set(reflect.ValueOf(*val))
	}

	// Return ErrEnvSetterBreak to indicate that the setter has finished processing.
	return ErrEnvSetterBreak
}

// setDefaultValue parses and sets the default value to the provided struct field.
// It supports various types including strings, integers, floats, booleans, complex numbers,
// slices, arrays, maps, and pointers. For complex types, the value is split by commas
// and for maps, by colons. Returns an error if parsing or setting the value fails.
//
//nolint:gocognit,funlen
func setDefaultValue(field reflect.Value, value string) error {
	var err error
	if value == "" || !field.IsZero() {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var v uint64
		if v, err = strconv.ParseUint(value, 10, field.Type().Bits()); err != nil {
			return fmt.Errorf("could not parse %q: %w", value, err)
		}

		field.SetUint(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		if v, err = strconv.ParseInt(value, 10, field.Type().Bits()); err != nil {
			return fmt.Errorf("could not parse %q: %w", value, err)
		}

		field.SetInt(v)
	case reflect.Float32, reflect.Float64:
		var v float64
		if v, err = strconv.ParseFloat(value, field.Type().Bits()); err != nil {
			return fmt.Errorf("could not parse %q: %w", value, err)
		}

		field.SetFloat(v)
	case reflect.Bool:
		var v bool
		if v, err = strconv.ParseBool(value); err != nil {
			return fmt.Errorf("could not parse %q as a bool: %w", value, err)
		}

		field.SetBool(v)
	case reflect.Complex64, reflect.Complex128:
		var v complex128
		if v, err = strconv.ParseComplex(value, field.Type().Bits()); err != nil {
			return fmt.Errorf("could not parse %q: %w", value, err)
		}

		field.SetComplex(v)
	case reflect.Slice:
		items := strings.Split(value, ",")
		slice := reflect.MakeSlice(field.Type(), 0, len(items))
		for _, item := range items {
			if item == "" {
				continue
			}

			elem, errElem := parseDefault(field.Type().Elem(), item)
			if errElem != nil {
				return errElem
			}

			slice = reflect.Append(slice, elem)
		}

		field.Set(slice)
	case reflect.Array:
		items := strings.Split(value, ",")
		array := reflect.New(field.Type()).Elem()
		if array.Len() < len(items) {
			return fmt.Errorf("array length exceeds %d elements", field.Len())
		}

		for i, item := range items {
			if item == "" {
				continue
			}

			elem, errElem := parseDefault(field.Type().Elem(), item)
			if errElem != nil {
				return errElem
			}

			array.Index(i).Set(elem)
		}

		field.Set(array)
	case reflect.Map:
		return setDefaultMap(field, value)
	case reflect.Pointer:
		elem := reflect.New(field.Type().Elem())
		if err = setDefaultValue(elem.Elem(), value); err != nil {
			return fmt.Errorf("could not set default %q: %w", elem, err)
		}

		field.Set(elem)
	default:
		return fmt.Errorf("unsupported type: %s", field.Type())
	}

	return nil
}
