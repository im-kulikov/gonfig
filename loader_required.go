package gonfig

import (
	"errors"
	"fmt"
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
	var missingFields []ErrMissingField // nolint:prealloc
	for elem, err := range ReflectFieldsOf(input, ReflectOptions{CanInterface: new(true), Pointers: true}) {
		if err != nil {
			return fmt.Errorf("(require) %w", err)
		}

		options := ParseTagOptions(elem.Field.Tag)
		if !options.FieldRequired || !elem.Value.IsZero() {
			continue
		}

		var path string
		for owner := elem; owner != nil; owner = owner.Owner {
			if owner.Field.Name == "" {
				continue
			}

			if path == "" {
				path = owner.Field.Name

				continue
			}

			path = fmt.Sprintf("%s.%s", owner.Field.Name, path)
		}

		missingFields = append(missingFields, ErrMissingField{
			Field: elem.Field.Name,
			Type:  elem.Field.Type.String(),
			Path:  path,
		})
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

		if value.Kind() != reflect.Struct {
			continue
		}

		name := path
		if !field.Anonymous { // embedded fields are promoted: no segment in the path
			name = strings.TrimPrefix(path+"."+field.Name, ".")
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
