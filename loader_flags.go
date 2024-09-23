package gonfig

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"time"

	"github.com/spf13/pflag"
)

const (
	FlagB64      = "b64"   // FlagB64 indicating base64 encoding for byte slices.
	FlagHEX      = "hex"   // FlagHEX indicating hexadecimal encoding for byte slices.
	FlagTag      = "flag"  // FlagTag is tag used to specify the flag name for a field.
	FlagTagUsage = "usage" // FlagTagUsage is tag used to specify the usage description for a flag.
	FlagSetName  = "flags" // FlagSetName is name of the flag set for the command-line interface.
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

		set.SetOutput(os.Stdout)

		return set.Parse(args)
	}}
}

// PrepareFlags prepares flags for the given flag set based on the fields of the destination struct.
// It inspects the struct fields and creates corresponding flags in the flag set using the specified tags.
// Returns an error if the preparation of flags fails.
func PrepareFlags(flagSet *pflag.FlagSet, dest any) error {
	types := []reflect.Type{reflect.TypeOf(net.IPNet{})}

	for elem, err := range ReflectFieldsOf(dest, ReflectOptions{CanSet: True(), AsField: types}) {
		if err != nil {
			return fmt.Errorf("(flags) %w", err)
		}

		options := ParseTagOptions(elem.Field.Tag)
		if options.FlagFullName == "" {
			continue
		}

		if len(options.FlagShortName) > 1 {
			return fmt.Errorf("(flags) shorthand is more than one ASCII character %q", options.FlagShortName)
		}

		if err = prepareFlag(flagSet, elem.Value, options); err != nil {
			return fmt.Errorf("(flags) %w", err)
		}
	}

	return nil
}

// parseConfigPath creates a Parser responsible for handling the "config-path" functionality.
// This parser reflects over the fields of the provided struct and parses flags related to the configuration path.
// It uses the pflag library to handle command-line flags and extracts flag metadata from struct tags.
//
// Parameters:
// - svc: A pointer to a loader struct that contains the arguments (`svc.Args`) and the config field (`svc.config`) to be populated.
//
// The function performs the following operations:
// 1. Reflects over the fields of the `val` argument using ReflectFieldsOf, filtering based on `ReflectOptions` (only settable fields are considered).
// 2. For each field, it checks if the field is tagged with `FlagConfig`, indicating it should be configured from the command line.
// 3. Ensures that only string fields are used for the configuration path, otherwise an error is returned.
// 4. Uses pflag to define and parse the configuration flag based on full name and short name from the `TagOptions`.
// 5. Parses the command-line arguments (`svc.Args`) to populate the `svc.config` field.
//
// If an error occurs during reflection or flag parsing, it returns a formatted error.
//
// Returns:
// - A Parser that is responsible for extracting and validating the "config-path" flag.
//
// Example usage:
//
//	parser := parseConfigPath(loaderInstance)
//	err := parser.Parse(configStruct)  // Parses the config path from the struct tags and command-line arguments.
func parseConfigPath(svc *loader) Parser {
	return &parserFunc{name: "config-path", call: func(val any) error {
		flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.ParseErrorsWhitelist.UnknownFlags = true

		var path string
		for elem, err := range ReflectFieldsOf(val, ReflectOptions{CanSet: True()}) {
			if err != nil {
				return fmt.Errorf("(config-path) could not fetch config flag: %w", err)
			}

			var opts TagOptions
			if opts = ParseTagOptions(elem.Field.Tag); !opts.FlagConfig {
				continue
			}
			if elem.Value.Kind() != reflect.String {
				return fmt.Errorf("(config-path) expect string, got %q", elem.Value.Kind())
			}

			if opts.FlagShortName != "" && opts.FlagShortName != "-" {
				flags.StringVarP(&svc.config, opts.FlagFullName, opts.FlagShortName, path, opts.FieldUsage)
			} else {
				flags.StringVar(&svc.config, opts.FlagFullName, path, opts.FieldUsage)
			}
		}

		if err := flags.Parse(svc.Args); err != nil && !errors.Is(err, pflag.ErrHelp) {
			return fmt.Errorf("(config-path) could not parse flags: %w", err)
		}

		return nil
	}}
}

// prepareFlag sets up a flag in the given flag set based on the field's type and the provided struct field information.
// It configures the flag with its name, short name, and usage description, and binds it to the field's value.
// Returns an error if the flag setup fails.
func prepareFlag(flagSet *pflag.FlagSet, field reflect.Value, info TagOptions) error {
	switch val := field.Addr().Interface().(type) {
	case *bool: // Handle boolean flags
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.BoolVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.BoolVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle integer flags
	case *int:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IntVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IntVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *int32:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Int32VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Int32Var(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *int64:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Int64VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Int64Var(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle unsigned integer flags
	case *uint:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.UintVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.UintVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *uint32:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Uint32VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Uint32Var(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *uint64:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Uint64VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Uint64Var(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle float flags
	case *float32:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Float32VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Float32Var(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *float64:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Float64VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Float64Var(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle string flags
	case *string:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.StringVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.StringVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle time.Duration flags
	case *time.Duration:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.DurationVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.DurationVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle network-related flags
	case *net.IP:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IPVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IPVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *net.IPNet:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IPNetVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IPNetVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *net.IPMask:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IPMaskVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IPMaskVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle slice flags
	case *[]int:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IntSliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IntSliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]int32:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Int32SliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Int32SliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]int64:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Int64SliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Int64SliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	// Handle float slices
	case *[]float32:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Float32SliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Float32SliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]float64:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.Float64SliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.Float64SliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}

	case *[]net.IP:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.IPSliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.IPSliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]time.Duration:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.DurationSliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.DurationSliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]byte:
		switch info.FlagEncodeBase {
		case FlagHEX:
			if info.FlagShortName != "" && info.FlagShortName != "-" {
				flagSet.BytesHexVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
			} else {
				flagSet.BytesHexVar(val, info.FlagFullName, *val, info.FieldUsage)
			}
		case FlagB64:
			if info.FlagShortName != "" && info.FlagShortName != "-" {
				flagSet.BytesBase64VarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
			} else {
				flagSet.BytesBase64Var(val, info.FlagFullName, *val, info.FieldUsage)
			}
		default:
			return fmt.Errorf("unknown []byte decoding type: %v", info.FlagEncodeBase)
		}
	case *[]bool:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.BoolSliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.BoolSliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	case *[]string:
		if info.FlagShortName != "" && info.FlagShortName != "-" {
			flagSet.StringSliceVarP(val, info.FlagFullName, info.FlagShortName, *val, info.FieldUsage)
		} else {
			flagSet.StringSliceVar(val, info.FlagFullName, *val, info.FieldUsage)
		}
	default:
		return fmt.Errorf("unknown type: %T", val)
	}

	return nil
}
