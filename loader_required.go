package gonfig

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
)

// ErrMissingField represents an error for a missing required field.
// It contains information about the field's name, its type, and the full path to the field.
type ErrMissingField struct {
	Field string // Name of the field.
	Type  string // Type of the field.
	Path  string // Path is a full path to the field in the nested structure.
}

// RequiredTag defines the struct tag key used to specify if a field is required.
// When parsing struct tags, this key is used to indicate that a field must be provided
// (e.g., from an environment variable, configuration, or command-line argument).
//
// If a field is tagged with `required:"true"`, it signifies that the field is mandatory.
// Example usage: `required:"true"`
//
// This tag is commonly used for validation to ensure the necessary fields are populated.
const RequiredTag = "required"

// ErrMissingFields is an error returned when required fields are missing in input data.
const ErrMissingFields Error = "missing required fields"

// Error formats the ErrMissingField into a descriptive error message.
func (e ErrMissingField) Error() string {
	if e.Field == e.Path {
		return fmt.Sprintf("field `%s` <%s> is required", e.Field, e.Type)
	}

	return fmt.Sprintf("field `%s` <%s> in path `%s` is required", e.Field, e.Type, e.Path)
}

// ValidateRequiredFields checks whether all fields marked with the "required" tag are set.
// It traverses the provided struct, including nested structs, to identify any missing required fields.
// It returns detailed error messages for all missing fields.
func ValidateRequiredFields(input any) error {
	missingFields, err := missingFields(input, "", nil)
	if err != nil {
		return fmt.Errorf("(require) %w", err)
	}

	if len(missingFields) == 0 {
		return nil
	}

	lines := make([]string, 0, len(missingFields))
	for _, e := range missingFields {
		lines = append(lines, fmt.Sprintf("\n\t- %s", e))
	}

	return fmt.Errorf("%w:%s", ErrMissingFields, strings.Join(lines, ""))
}

// missingFields returns the required fields of input that are empty, with the path
// after prefix; the structs in lists and maps are checked too, Items[0].Host.
func missingFields(input any, prefix string, way []visit) ([]ErrMissingField, error) {
	var missing []ErrMissingField

	for elem, err := range ReflectFieldsOf(input, ReflectOptions{CanInterface: new(true), Pointers: true}) {
		if err != nil {
			return nil, err
		}

		var names []string
		for owner := elem; owner.Owner != nil; owner = owner.Owner {
			names = append(names, owner.Field.Name)
		}

		slices.Reverse(names)
		path := prefix + strings.Join(names, ".")

		if ParseTagOptions(elem.Field.Tag).FieldRequired && elem.Value.IsZero() {
			missing = append(missing, ErrMissingField{Field: elem.Field.Name, Type: elem.Field.Type.String(), Path: path})

			continue
		}

		for key, item := range structElements(elem.Value, way) {
			inner, _ := missingFields(item.Interface(), path+"["+key+"].", append(slices.Clip(way), visitOf(item))) // a *struct
			missing = append(missing, inner...)
		}
	}

	return missing, nil
}

// structElements yields the structs in a list or a map v, by index or key, as pointers:
// a map value is a copy, it cannot be addressed. It skips nil pointers and pointers
// back to a struct on the way, a cycle.
func structElements(v reflect.Value, way []visit) iter.Seq2[string, reflect.Value] {
	return func(yield func(string, reflect.Value) bool) {
		if !isList(v.Kind()) && v.Kind() != reflect.Map || !isContainer(v.Type().Elem()) ||
			derefType(v.Type().Elem()).Kind() != reflect.Struct {
			return
		}

		keys, items := make([]string, 0, v.Len()), make(map[string]reflect.Value, v.Len())
		for key, item := range v.Seq2() {
			name := fmt.Sprint(key.Interface())
			keys, items[name] = append(keys, name), item
		}

		if v.Kind() == reflect.Map {
			slices.Sort(keys) // a stable order of errors
		}

		for _, key := range keys {
			item := items[key]

			switch {
			case item.Kind() == reflect.Pointer && (item.IsNil() || slices.Contains(way, visitOf(item))):
				continue
			case item.Kind() == reflect.Struct && item.CanAddr():
				item = item.Addr()
			case item.Kind() == reflect.Struct:
				copied := reflect.New(item.Type())
				copied.Elem().Set(item)
				item = copied
			}

			if !yield(key, item) {
				return
			}
		}
	}
}

// validate calls Validate of every struct in v that implements LoaderValidator, nested
// structs first and v itself last, and joins their errors. An error of v alone is
// returned as is. v is a pointer to a struct: ValidateRequiredFields has checked it.
func validate(v any) error {
	errs := validateFields(reflect.ValueOf(v).Elem(), "", nil)

	if root, ok := v.(LoaderValidator); ok {
		if err := root.Validate(); err != nil && len(errs) == 0 {
			return err
		} else if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateFields validates the nested structs of v, depth first, and the sections
// behind non-nil pointers to structs.
func validateFields(v reflect.Value, path string, way []visit) []error {
	var errs []error

	way = append(slices.Clip(way), visit{v.Addr().Pointer(), v.Type()})

	for field, value := range v.Fields() {
		if value.Kind() == reflect.Pointer && !value.IsNil() {
			if slices.Contains(way, visitOf(value)) { // a pointer back: a cycle
				continue
			}

			value = value.Elem()
		}

		name := path
		if !field.Anonymous { // embedded fields are promoted: no segment in the path
			name = strings.TrimPrefix(path+"."+field.Name, ".")
		}

		if value.Kind() != reflect.Struct {
			if value.CanInterface() {
				for key, item := range structElements(value, way) {
					errs = append(errs, validateElement(item, name+"["+key+"]", way)...)
				}
			}

			continue
		}

		errs = append(errs, validateFields(value, name, way)...)

		if field.Anonymous { // its Validate, if any, is promoted to the parent
			continue
		}

		if !value.Addr().CanInterface() { // an unexported field
			continue
		}

		if validator, ok := value.Addr().Interface().(LoaderValidator); ok {
			if err := validator.Validate(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}

	return errs
}

// validateElement validates a struct in a list or a map, item a pointer to it.
func validateElement(item reflect.Value, name string, way []visit) []error {
	errs := validateFields(item.Elem(), name, way)

	if validator, ok := item.Interface().(LoaderValidator); ok {
		if err := validator.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	return errs
}
