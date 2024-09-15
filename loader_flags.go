package gonfig

import (
	"fmt"
	"net"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

const (
	FlagB64      = "b64"        // FlagB64 indicating base64 encoding for byte slices.
	FlagHEX      = "hex"        // FlagHEX indicating hexadecimal encoding for byte slices.
	FlagTag      = "flag"       // FlagTag is tag used to specify the flag name for a field.
	FlagTagUsage = "usage"      // FlagTagUsage is tag used to specify the usage description for a flag.
	FlagTagShort = "flag-short" // FlagTagShort is tag used to specify a short flag name (single-character) for a field.
	FlagSetName  = "cli"        // FlagSetName is name of the flag set for the command-line interface.
)

// newFlagsLoader creates a new parser that loads configuration from command-line flags.
// It uses the provided arguments to populate the configuration by preparing and parsing the flags.
// Returns a Parser that processes command-line flags.
func newFlagsLoader(args []string) Parser {
	return &parserFunc{name: ParserFlags, call: func(val interface{}) error {
		set := pflag.NewFlagSet(FlagSetName, pflag.ContinueOnError)
		if err := PrepareFlags(set, val); err != nil {
			return err
		}

		return set.Parse(args)
	}}
}

// PrepareFlags prepares flags for the given flag set based on the fields of the destination struct.
// It inspects the struct fields and creates corresponding flags in the flag set using the specified tags.
// Returns an error if the preparation of flags fails.
func PrepareFlags(flagSet *pflag.FlagSet, dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("(flags) dest must be a pointer, got %T", dest)
	}

	v = v.Elem()
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("(flags) expected struct type, got %T", dest)
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		value := t.Field(i).Tag.Get(FlagTag)
		if !field.CanSet() {
			continue
		}

		// net.IPNet is struct, so we should prepare it as flag
		if _, ok := field.Addr().Interface().(*net.IPNet); ok && value != "" && value != "-" {
			if err := prepareFlag(flagSet, field, t.Field(i)); err != nil {
				return err
			}

			continue
		}

		// Recursively prepare flags for nested structs.
		if field.Kind() == reflect.Struct && field.CanAddr() {
			elem := field.Addr().Interface()

			if err := PrepareFlags(flagSet, elem); err != nil {
				return fmt.Errorf("(flags) failed to prepare flag for %q: %w", t.Field(i).Name, err)
			}

			continue
		}

		if value == "" || value == "-" {
			continue
		}

		if !field.CanAddr() {
			return fmt.Errorf("(flags) field %q is not addressable", t.Field(i).Name)
		} else if err := prepareFlag(flagSet, field, t.Field(i)); err != nil {
			return fmt.Errorf("(flags) failed to prepare flag for %q: %w", t.Field(i).Name, err)
		}
	}

	return nil
}

// prepareFlag sets up a flag in the given flag set based on the field's type and the provided struct field information.
// It configures the flag with its name, short name, and usage description, and binds it to the field's value.
// Returns an error if the flag setup fails.
func prepareFlag(flagSet *pflag.FlagSet, field reflect.Value, info reflect.StructField) error {
	flagName := info.Tag.Get(FlagTag)
	flagShort := info.Tag.Get(FlagTagShort)
	flagUsage := info.Tag.Get(FlagTagUsage)

	options := strings.Split(flagName, ",")
	flagName = options[0]

	switch val := field.Addr().Interface().(type) {
	// Handle boolean flags
	case *bool:
		if flagShort != "" && flagShort != "-" {
			flagSet.BoolVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.BoolVar(val, flagName, *val, flagUsage)
		}

	// Handle integer flags
	case *int:
		if flagShort != "" && flagShort != "-" {
			flagSet.IntVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IntVar(val, flagName, *val, flagUsage)
		}
	case *int32:
		if flagShort != "" && flagShort != "-" {
			flagSet.Int32VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Int32Var(val, flagName, *val, flagUsage)
		}
	case *int64:
		if flagShort != "" && flagShort != "-" {
			flagSet.Int64VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Int64Var(val, flagName, *val, flagUsage)
		}

	// Handle unsigned integer flags
	case *uint:
		if flagShort != "" && flagShort != "-" {
			flagSet.UintVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.UintVar(val, flagName, *val, flagUsage)
		}
	case *uint32:
		if flagShort != "" && flagShort != "-" {
			flagSet.Uint32VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Uint32Var(val, flagName, *val, flagUsage)
		}
	case *uint64:
		if flagShort != "" && flagShort != "-" {
			flagSet.Uint64VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Uint64Var(val, flagName, *val, flagUsage)
		}

	// Handle float flags
	case *float32:
		if flagShort != "" && flagShort != "-" {
			flagSet.Float32VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Float32Var(val, flagName, *val, flagUsage)
		}
	case *float64:
		if flagShort != "" && flagShort != "-" {
			flagSet.Float64VarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Float64Var(val, flagName, *val, flagUsage)
		}

	// Handle string flags
	case *string:
		if flagShort != "" && flagShort != "-" {
			flagSet.StringVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.StringVar(val, flagName, *val, flagUsage)
		}

	// Handle time.Duration flags
	case *time.Duration:
		if flagShort != "" && flagShort != "-" {
			flagSet.DurationVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.DurationVar(val, flagName, *val, flagUsage)
		}

	// Handle network-related flags
	case *net.IP:
		if flagShort != "" && flagShort != "-" {
			flagSet.IPVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IPVar(val, flagName, *val, flagUsage)
		}
	case *net.IPNet:
		if flagShort != "" && flagShort != "-" {
			flagSet.IPNetVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IPNetVar(val, flagName, *val, flagUsage)
		}
	case *net.IPMask:
		if flagShort != "" && flagShort != "-" {
			flagSet.IPMaskVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IPMaskVar(val, flagName, *val, flagUsage)
		}

	// Handle slice flags
	case *[]int:
		if flagShort != "" && flagShort != "-" {
			flagSet.IntSliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IntSliceVar(val, flagName, *val, flagUsage)
		}
	case *[]int32:
		if flagShort != "" && flagShort != "-" {
			flagSet.Int32SliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Int32SliceVar(val, flagName, *val, flagUsage)
		}
	case *[]int64:
		if flagShort != "" && flagShort != "-" {
			flagSet.Int64SliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Int64SliceVar(val, flagName, *val, flagUsage)
		}

	// Handle float slices
	case *[]float32:
		if flagShort != "" && flagShort != "-" {
			flagSet.Float32SliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Float32SliceVar(val, flagName, *val, flagUsage)
		}
	case *[]float64:
		if flagShort != "" && flagShort != "-" {
			flagSet.Float64SliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.Float64SliceVar(val, flagName, *val, flagUsage)
		}

	case *[]net.IP:
		if flagShort != "" && flagShort != "-" {
			flagSet.IPSliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.IPSliceVar(val, flagName, *val, flagUsage)
		}
	case *[]time.Duration:
		if flagShort != "" && flagShort != "-" {
			flagSet.DurationSliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.DurationSliceVar(val, flagName, *val, flagUsage)
		}
	case *[]byte:
		switch {
		case slices.Contains(options, FlagHEX):
			if flagShort != "" && flagShort != "-" {
				flagSet.BytesHexVarP(val, flagName, flagShort, *val, flagUsage)
			} else {
				flagSet.BytesHexVar(val, flagName, *val, flagUsage)
			}
		case slices.Contains(options, FlagB64):
			if flagShort != "" && flagShort != "-" {
				flagSet.BytesBase64VarP(val, flagName, flagShort, *val, flagUsage)
			} else {
				flagSet.BytesBase64Var(val, flagName, *val, flagUsage)
			}
		default:
			return fmt.Errorf("unknown []byte decoding type: %v", options)
		}
	case *[]bool:
		if flagShort != "" && flagShort != "-" {
			flagSet.BoolSliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.BoolSliceVar(val, flagName, *val, flagUsage)
		}
	case *[]string:
		if flagShort != "" && flagShort != "-" {
			flagSet.StringSliceVarP(val, flagName, flagShort, *val, flagUsage)
		} else {
			flagSet.StringSliceVar(val, flagName, *val, flagUsage)
		}
	default:
		return fmt.Errorf("unknown type: %T", val)
	}

	return nil
}
