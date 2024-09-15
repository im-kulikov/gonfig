package gonfig

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

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
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("(defaults) dest must be a pointer, got %T", dest)
	}

	v = v.Elem()
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("(defaults) expected struct type, got %T", dest)
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		value := t.Field(i).Tag.Get("default")
		if !field.CanSet() {
			continue
		}

		if field.CanAddr() && value != "" {
			elem := field.Addr().Interface()

			if text, ok := elem.(encoding.TextUnmarshaler); ok {
				return text.UnmarshalText([]byte(value))
			}
		}

		if field.Kind() == reflect.Struct && field.CanAddr() {
			elem := field.Addr().Interface()
			if err := SetDefaults(elem); err != nil {
				return fmt.Errorf("(defaults) failed to set field %q: %w", t.Field(i).Name, err)
			}

			continue
		}

		if err := setDefaultValue(field, value); err != nil {
			return fmt.Errorf("(defaults) failed to set field %q: %w", t.Field(i).Name, err)
		}
	}

	return nil
}

// setDefaultValue parses and sets the default value to the provided struct field.
// It supports various types including strings, integers, floats, booleans, complex numbers,
// slices, arrays, maps, and pointers. For complex types, the value is split by commas
// and for maps, by colons. Returns an error if parsing or setting the value fails.
func setDefaultValue(field reflect.Value, value string) error {
	var err error
	if value == "" {
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
