package gonfig

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrMissingField represents an error for a missing required field.
// It contains information about the field's name, its type, and the full path to the field.
type ErrMissingField struct {
	Field string // Name of the field.
	Type  string // Type of the field.
	Path  string // Path is full path to the field in the nested structure.
}

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
	v := reflect.ValueOf(input)

	// Ensure that the input is a pointer to a struct.
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("input must be a pointer to a struct")
	}

	var missingFields []ErrMissingField
	// Start collecting missing required fields from the root level of the struct.
	collectMissingFields(v.Elem(), "", &missingFields)

	// If there are any missing fields, format them into a readable error message.
	if len(missingFields) > 0 {
		return formatMissingFieldsError(missingFields)
	}

	return nil
}

// collectMissingFields recursively inspects the provided struct value and collects information
// about any missing required fields. It also handles nested structs.
func collectMissingFields(v reflect.Value, parentPath string, missingFields *[]ErrMissingField) {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip fields that are unexported (cannot be accessed).
		if !field.CanInterface() {
			continue
		}

		// Recursively handle nested structs to check for missing required fields.
		if field.Kind() == reflect.Struct {
			collectMissingFields(field, buildFieldPath(fieldType, parentPath), missingFields)

			continue
		}

		// Retrieve the "required" tag for the field.
		requiredTag := fieldType.Tag.Get("required")
		if requiredTag == "" || requiredTag == "false" || requiredTag == "-" {
			continue
		}

		// Check if the field is zero value (not set).
		if !field.IsZero() {
			continue
		}

		// Append an error entry for the missing field.
		*missingFields = append(*missingFields, ErrMissingField{
			Field: fieldType.Name,
			Type:  fieldType.Type.String(),
			Path:  buildFieldPath(fieldType, parentPath),
		})
	}
}

// buildFieldPath constructs the full path to a field by combining the parent path with the field name.
// If there is no parent path, it simply returns the field name.
func buildFieldPath(fieldType reflect.StructField, parentPath string) string {
	name := fieldType.Name

	if parentPath != "" {
		return fmt.Sprintf("%s.%s", parentPath, name)
	}

	return name
}

// formatMissingFieldsError creates a formatted error message listing all missing required fields.
// Each missing field is described with its name, type, and path in the nested structure.
func formatMissingFieldsError(missingFields []ErrMissingField) error {
	var lines []string

	lines = append(lines, "missing required fields:")
	for _, e := range missingFields {
		lines = append(lines, e.Error())
	}
	return errors.New(strings.Join(lines, "\n\t- "))
}
