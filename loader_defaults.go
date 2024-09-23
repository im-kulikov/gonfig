package gonfig

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ErrEnvSetterBreak is a predefined constant of type constantError
// used to indicate an error or a condition where processing should stop.
const ErrEnvSetterBreak = constantError("break")

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
func newDefaultParser() Parser {
	return &parserFunc{name: ParserDefaults, call: SetDefaults}
}

// SetDefaults sets default values to the fields of the provided struct.
// It recursively processes struct fields and assigns default values based on
// the "default" tag. It supports setting values for basic types, slices, arrays, maps,
// and custom unmarshalling for types implementing encoding.TextUnmarshaler.
// Returns an error if the destination is not a pointer or if setting a default value fails.
func SetDefaults(dest interface{}) error {
	types := []reflect.Type{reflect.TypeOf(net.IPNet{})}
	for elem, err := range ReflectFieldsOf(dest, ReflectOptions{CanAddr: True(), AsField: types}) {
		if err != nil {
			return fmt.Errorf("(defaults) %w", err)
		}

		value := elem.Field.Tag.Get(defaultTagName)
		if err = tryCustomTypes(elem.Value, value); errors.Is(err, ErrEnvSetterBreak) {
			continue
		} else if err != nil {
			return fmt.Errorf("(defaults) failed to set field %q: %w", elem.Field.Name, err)
		}

		if err = setDefaultValue(elem.Value, value); err != nil {
			return fmt.Errorf("(defaults) failed to set field %q: %w", elem.Field.Name, err)
		}
	}

	return nil
}

// tryCustomTypes attempts to set the value of a reflect.Value field based on its type.
// It handles specific types like time.Duration, net.IP, net.IPMask, and net.IPNet.
// If the value is not empty and the field is not already set (IsZero), it processes the value.
func tryCustomTypes(field reflect.Value, value interface{}) error {
	// If the value is empty or the field already has a value, return early with no error.
	if value == "" || !field.IsZero() {
		return nil
	}

	// Switch on the underlying type of the field, and handle specific custom types.
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
	case net.IP:
		// If the field is net.IP, attempt to parse the value as an IP address.
		val := net.ParseIP(value.(string))
		if val == nil && value.(string) != "" {
			return fmt.Errorf("invalid IP address %q", value.(string)) // Return error for invalid IP.
		}
		// Set the parsed IP address to the field.
		field.Set(reflect.ValueOf(val))
	case net.IPMask:
		// If the field is net.IPMask, trim the leading '/' from the CIDR mask string.
		mask := strings.TrimPrefix(value.(string), "/")
		// Convert the mask string to an integer (CIDR prefix length).
		prefix, err := strconv.Atoi(mask)
		if err != nil {
			return err // Return error if conversion fails.
		}
		// Set the corresponding IP mask using net.CIDRMask with a 32-bit IPv4 mask.
		field.Set(reflect.ValueOf(net.CIDRMask(prefix, 32)))
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
			return err
		}

		field.SetUint(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		if v, err = strconv.ParseInt(value, 10, field.Type().Bits()); err != nil {
			return err
		}

		field.SetInt(v)
	case reflect.Float32, reflect.Float64:
		var v float64
		if v, err = strconv.ParseFloat(value, field.Type().Bits()); err != nil {
			return err
		}

		field.SetFloat(v)
	case reflect.Bool:
		var v bool
		if v, err = strconv.ParseBool(value); err != nil {
			return err
		}

		field.SetBool(v)
	case reflect.Complex64, reflect.Complex128:
		var v complex128
		if v, err = strconv.ParseComplex(value, field.Type().Bits()); err != nil {
			return err
		}

		field.SetComplex(v)
	case reflect.Slice:
		items := strings.Split(value, ",")
		slice := reflect.MakeSlice(field.Type(), 0, len(items))
		for _, item := range items {
			if item == "" {
				continue
			}

			elem := reflect.New(field.Type().Elem()).Elem()
			if err = setDefaultValue(elem, item); err != nil {
				return err
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

			elem := reflect.New(field.Type().Elem()).Elem()
			if err = setDefaultValue(elem, item); err != nil {
				return err
			}

			array.Index(i).Set(elem)
		}

		field.Set(array)
	case reflect.Map:
		items := strings.Split(value, ",")
		maper := reflect.MakeMap(field.Type())
		for _, item := range items {
			pair := strings.Split(item, ":")
			if len(pair) != 2 {
				continue
			}

			key := reflect.New(field.Type().Key()).Elem()
			if err = setDefaultValue(key, pair[0]); err != nil {
				return err
			}

			val := reflect.New(field.Type().Elem()).Elem()
			if err = setDefaultValue(val, pair[1]); err != nil {
				return err
			}

			maper.SetMapIndex(key, val)
		}

		field.Set(maper)
	case reflect.Ptr:
		elem := reflect.New(field.Type().Elem())
		if err = setDefaultValue(elem.Elem(), value); err != nil {
			return err
		}

		field.Set(elem)
	default:
		return fmt.Errorf("unsupported type: %s", field.Type())
	}

	return nil
}
