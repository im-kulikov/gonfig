package gonfig

import (
	"errors"
	"fmt"
	"strings"
)

// ErrMissingField represents an error for a missing required field.
// It contains information about the field's name, its type, and the full path to the field.
type ErrMissingField struct {
	Field string // Name of the field.
	Type  string // Type of the field.
	Path  string // Path is full path to the field in the nested structure.
}

// RequiredTag defines the struct tag key used to specify if a field is required.
// When parsing struct tags, this key is used to indicate that a field must be provided
// (e.g., from an environment variable, configuration, or command-line argument).
//
// If a field is tagged with `required:"true"`, it signifies that the field is mandatory.
// Example usage: `required:"true"`
//
// This tag is commonly used for validation purposes to ensure necessary fields are populated.
const RequiredTag = "required"

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
	var missingFields []ErrMissingField
	for elem, err := range ReflectFieldsOf(input, ReflectOptions{CanInterface: True()}) {
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

	lines := []string{"missing required fields:"}
	for _, e := range missingFields {
		lines = append(lines, e.Error())
	}

	return errors.New(strings.Join(lines, "\n\t- "))
}
