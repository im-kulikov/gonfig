package gonfig

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"slices"
	"time"

	"github.com/spf13/pflag"
)

const (
	// FlagB64 indicating base64 encoding for byte slices.
	FlagB64 = "b64"
	// FlagHEX indicating hexadecimal encoding for byte slices.
	FlagHEX = "hex"
	// FlagTag is a tag used to specify the flag name for a field.
	FlagTag = "flag"
	// FlagTagUsage is a tag used to specify the usage description for a flag.
	FlagTagUsage = "usage"
	// FlagSetName is a name of the flag set for the command-line interface.
	FlagSetName = "flags"

	// ErrFlagRedefined is returned when two fields, or a field and DefaultConfigFlag
	// or PrintConfigFlag, define the same flag or shorthand.
	ErrFlagRedefined Error = "flag redefined"
)

var (
	markerType      = reflect.TypeFor[DefaultConfigMarker]()
	printMarkerType = reflect.TypeFor[PrintConfigMarker]()
)

// printConfigFlag is the name of the flag enabled by PrintConfigFlag.
const printConfigFlag = "print-config"

// DefaultConfigMarker is an interface used to identify structures that
// provide default configuration flag support.
type DefaultConfigMarker interface {
	IsDefaultConfig()
}

// DefaultConfigFlag is a zero-size structure that can be embedded into
// configuration structures to automatically enable default --config and -c flags.
type DefaultConfigFlag struct{}

// IsDefaultConfig is a marker method that satisfies the DefaultConfigMarker interface.
func (DefaultConfigFlag) IsDefaultConfig() {}

// PrintConfigMarker is an interface used to identify structures that
// provide the --print-config flag.
type PrintConfigMarker interface {
	IsPrintConfig()
}

// PrintConfigFlag is a zero-size structure that can be embedded into configuration
// structures to enable the --print-config[=yaml|json|toml|env] flag. With it the
// loader writes the loaded configuration with Write (secrets left empty) and exits
// with code 0, like --help. It runs before required fields are checked, so it also
// prints a template of the configuration. Without a value the format is the one of
// the file loader, or YAML without one.
type PrintConfigFlag struct{}

// IsPrintConfig is a marker method that satisfies the PrintConfigMarker interface.
func (PrintConfigFlag) IsPrintConfig() {}

// newFlagsLoader creates a new parser that loads configuration from command-line flags.
// It uses the provided arguments to populate the configuration by preparing and parsing the flags.
// Returns a Parser that processes command-line flags.
func newFlagsLoader(l *loader) *parserFunc {
	return &parserFunc{name: ParserFlags, call: func(val any) error {
		set := pflag.NewFlagSet(FlagSetName, pflag.ContinueOnError)
		if containsDefaultConfigFlag(val) {
			set.StringVarP(&l.config, "config", "c", l.config, "path to config file")
		}

		if err := PrepareFlags(set, val); err != nil {
			return err
		}

		if containsMarker(val, printMarkerType) {
			if set.Lookup(printConfigFlag) != nil {
				return fmt.Errorf("(flags) %w: --%s, by PrintConfigFlag and a field", ErrFlagRedefined, printConfigFlag)
			}

			set.StringVar(&l.printConfig, printConfigFlag, "",
				"print the loaded config in `format` (yaml, json, toml or env) without secrets and exit")
			set.Lookup(printConfigFlag).NoOptDefVal = string(l.fileFormat())
		}

		set.SetOutput(l.buffer)

		if err := set.Parse(l.Args); err != nil {
			return err
		}

		return setPositionalArgs(val, set.Args())
	}}
}

// setPositionalArgs gives the arguments left after the flags (all after "--" too) to
// the []string fields tagged `flag:",args"`. Without arguments the field keeps the
// value of the other sources, a `default` tag for example.
func setPositionalArgs(dest any, args []string) error {
	for elem := range ReflectFieldsOf(dest, ReflectOptions{CanSet: new(true)}) { // PrepareFlags has checked dest
		switch {
		case !ParseTagOptions(elem.Field.Tag).FlagArgs:
		case elem.Value.Type() != reflect.TypeFor[[]string]():
			return fmt.Errorf("(flags) field %s: `flag:\",args\"` needs []string, got %s", elem.Field.Name, elem.Value.Type())
		case len(args) > 0:
			elem.Value.Set(reflect.ValueOf(args))
		}
	}

	return nil
}

// PrepareFlags prepares flags for the given flag set based on the fields of the destination struct.
// It inspects the struct fields and creates corresponding flags in the flag set using the specified tags.
// Returns an error if the preparation of flags fails.
func PrepareFlags(flagSet *pflag.FlagSet, dest any) error {
	types := []reflect.Type{reflect.TypeFor[net.IPNet]()}

	for elem, err := range ReflectFieldsOf(dest, ReflectOptions{CanSet: new(true), AsField: types}) {
		if err != nil {
			return fmt.Errorf("(flags) %w", err)
		}

		options := ParseTagOptions(elem.Field.Tag)
		if options.FlagFullName == "" || options.FieldIgnored {
			continue
		}

		if err = checkFlag(flagSet, elem.Field.Name, options); err != nil {
			return fmt.Errorf("(flags) %w", err)
		}

		if err = prepareFlag(flagSet, elem.Value, options); err != nil {
			return fmt.Errorf("(flags) %w", err)
		}

		flagSet.Lookup(options.FlagFullName).DefValue = flagDefault(elem, options)
	}

	return nil
}

// checkFlag returns the error pflag would panic with when the flag of a field is added.
func checkFlag(flagSet *pflag.FlagSet, field string, options TagOptions) error {
	if len(options.FlagShortName) > 1 {
		return fmt.Errorf("shorthand is more than one ASCII character %q", options.FlagShortName)
	}

	if flagSet.Lookup(options.FlagFullName) != nil {
		return fmt.Errorf("field %s: %w: --%s", field, ErrFlagRedefined, options.FlagFullName)
	}

	if short := options.shorthand(); short != "" && flagSet.ShorthandLookup(short) != nil {
		return fmt.Errorf("field %s: %w: -%s", field, ErrFlagRedefined, short)
	}

	return nil
}

// flagDefault is what --help shows as the default of a flag: the `default` tag,
// formatted by pflag for the flag's type. The flag itself is registered with the
// current value, which may already come from env or a file and hold a secret.
// A secret shows no default at all.
func flagDefault(elem *ReflectValue, options TagOptions) string {
	value := reflect.New(elem.Value.Type()).Elem()
	if !isSecret(elem) {
		// An invalid default has already failed the defaults parser.
		_ = applyDefault(value, elem.Field.Tag.Get(defaultTagName))
	}

	scratch := pflag.NewFlagSet(FlagSetName, pflag.ContinueOnError)
	_ = prepareFlag(scratch, value, options) // the same type has just been registered

	flag := scratch.Lookup(options.FlagFullName)

	// pflag hides a zero default by comparing it with the zero text of the flag type,
	// and knows "[]" only for these lists: other lists and maps would show
	// "(default [])". For them an empty default is the zero text.
	if value.IsZero() && flag.DefValue == "[]" && !slices.Contains(pflagZeroLists, flag.Value.Type()) {
		return ""
	}

	return flag.DefValue
}

// pflagZeroLists are the flag types whose zero default pflag recognizes as "[]".
var pflagZeroLists = []string{"intSlice", "stringSlice", "stringArray"}

// parseConfigPath returns the parser of the ParserConfigSet step: it pre-scans the
// arguments for the config path, from --config/-c with DefaultConfigFlag or from the
// string field tagged `flag:"...,config:true"`, and stores it in l.config for the
// parsers that implement ParserConfigSetter. Unknown flags are left to the flags step.
func parseConfigPath(l *loader) *parserFunc {
	return &parserFunc{name: ParserConfigSet, call: parseDefaultConfigPath(l, func(val any) error {
		flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.ParseErrorsAllowlist.UnknownFlags = true

		for elem, err := range ReflectFieldsOf(val, ReflectOptions{CanSet: new(true)}) {
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

			if err = checkFlag(flags, elem.Field.Name, opts); err != nil {
				return fmt.Errorf("(config-path) %w", err)
			}

			flags.StringVarP(&l.config, opts.FlagFullName, opts.shorthand(), "", opts.FieldUsage)
		}

		if err := flags.Parse(l.Args); err != nil && !errors.Is(err, pflag.ErrHelp) {
			return fmt.Errorf("(config-path) could not parse flags: %w", err)
		}

		return nil
	})}
}

// parseDefaultConfigPath is a decorator that wraps a configuration path parser.
// If the DefaultConfigFlag is present in the destination struct, it pre-scans
// the command-line arguments for the --config and -c flags.
func parseDefaultConfigPath(l *loader, call func(val any) error) func(any) error {
	return func(val any) error {
		if !containsDefaultConfigFlag(val) {
			return call(val)
		}

		flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.ParseErrorsAllowlist.UnknownFlags = true

		flags.StringVarP(&l.config, "config", "c", l.config, "path to config file")
		if err := flags.Parse(l.Args); err != nil && !errors.Is(err, pflag.ErrHelp) {
			return fmt.Errorf("(config-path) could not parse flags: %w", err)
		}

		return nil
	}
}

// containsDefaultConfigFlag checks if the provided value or any of its embedded fields
// satisfy the DefaultConfigMarker interface.
func containsDefaultConfigFlag(val any) bool {
	return containsMarker(val, markerType)
}

// containsMarker checks if the provided value or any of its embedded fields implement
// the marker interface, recursing through nested and pointer embeddings.
func containsMarker(val any, marker reflect.Type) bool {
	t := reflect.TypeOf(val)

	return t != nil && hasMarker(t, marker, nil)
}

// hasMarker checks t and its embedded fields; way holds the structs it is in, as a
// struct may embed a pointer to itself.
func hasMarker(t, marker reflect.Type, way []reflect.Type) bool {
	if t.Implements(marker) || reflect.PointerTo(t).Implements(marker) {
		return true
	}

	if t = derefType(t); t.Kind() != reflect.Struct || slices.Contains(way, t) {
		return false
	}

	for f := range t.Fields() {
		if f.Type.Implements(marker) || reflect.PointerTo(f.Type).Implements(marker) ||
			f.Anonymous && hasMarker(f.Type, marker, append(slices.Clip(way), t)) {
			return true
		}
	}

	return false
}

// prepareFlag registers the flag of a field. A type pflag has a flag for gets it,
// with pflag's parsing: a bool flag needs no value, a repeated slice flag appends.
// Any other type the `default` tag can parse, such as a named type (type Port int),
// an encoding.TextUnmarshaler (slog.Level), a pointer or a map, gets a fieldValue.
//
//nolint:gocyclo,funlen // one line per type pflag supports
func prepareFlag(flagSet *pflag.FlagSet, field reflect.Value, info TagOptions) error {
	name, short, usage := info.FlagFullName, info.shorthand(), info.FieldUsage

	switch p := field.Addr().Interface().(type) {
	case *bool:
		flagSet.BoolVarP(p, name, short, *p, usage)
	case *string:
		flagSet.StringVarP(p, name, short, *p, usage)
	case *int:
		flagSet.IntVarP(p, name, short, *p, usage)
	case *int8:
		flagSet.Int8VarP(p, name, short, *p, usage)
	case *int16:
		flagSet.Int16VarP(p, name, short, *p, usage)
	case *int32:
		flagSet.Int32VarP(p, name, short, *p, usage)
	case *int64:
		flagSet.Int64VarP(p, name, short, *p, usage)
	case *uint:
		flagSet.UintVarP(p, name, short, *p, usage)
	case *uint8:
		flagSet.Uint8VarP(p, name, short, *p, usage)
	case *uint16:
		flagSet.Uint16VarP(p, name, short, *p, usage)
	case *uint32:
		flagSet.Uint32VarP(p, name, short, *p, usage)
	case *uint64:
		flagSet.Uint64VarP(p, name, short, *p, usage)
	case *float32:
		flagSet.Float32VarP(p, name, short, *p, usage)
	case *float64:
		flagSet.Float64VarP(p, name, short, *p, usage)
	case *time.Duration:
		flagSet.DurationVarP(p, name, short, *p, usage)
	case *net.IP:
		flagSet.IPVarP(p, name, short, *p, usage)
	case *net.IPNet:
		flagSet.IPNetVarP(p, name, short, *p, usage)
	case *net.IPMask:
		flagSet.IPMaskVarP(p, name, short, *p, usage)
	case *[]bool:
		flagSet.BoolSliceVarP(p, name, short, *p, usage)
	case *[]string:
		flagSet.StringSliceVarP(p, name, short, *p, usage)
	case *[]int:
		flagSet.IntSliceVarP(p, name, short, *p, usage)
	case *[]int32:
		flagSet.Int32SliceVarP(p, name, short, *p, usage)
	case *[]int64:
		flagSet.Int64SliceVarP(p, name, short, *p, usage)
	case *[]uint:
		flagSet.UintSliceVarP(p, name, short, *p, usage)
	case *[]float32:
		flagSet.Float32SliceVarP(p, name, short, *p, usage)
	case *[]float64:
		flagSet.Float64SliceVarP(p, name, short, *p, usage)
	case *[]net.IP:
		flagSet.IPSliceVarP(p, name, short, *p, usage)
	case *[]time.Duration:
		flagSet.DurationSliceVarP(p, name, short, *p, usage)
	case *map[string]string:
		flagSet.StringToStringVarP(p, name, short, *p, usage)
	case *map[string]int:
		flagSet.StringToIntVarP(p, name, short, *p, usage)
	case *map[string]int64:
		flagSet.StringToInt64VarP(p, name, short, *p, usage)
	case *[]byte:
		return prepareBytesFlag(flagSet, p, name, short, usage, info.FlagEncodeBase)
	default:
		if !parsable(field.Type()) {
			return fmt.Errorf("unknown type: %T", p)
		}

		flagSet.VarP(fieldValue{field}, name, short, usage)

		if field.Kind() == reflect.Bool { // like a bool flag: --enabled means true
			flagSet.Lookup(name).NoOptDefVal = "true"
		}
	}

	return nil
}

// prepareBytesFlag registers a []byte flag; its encoding is required: `flag:"key,base:hex"`.
func prepareBytesFlag(flagSet *pflag.FlagSet, p *[]byte, name, short, usage, base string) error {
	switch base {
	case FlagHEX:
		flagSet.BytesHexVarP(p, name, short, *p, usage)
	case FlagB64:
		flagSet.BytesBase64VarP(p, name, short, *p, usage)
	default:
		return fmt.Errorf("unknown []byte decoding type: %v", base)
	}

	return nil
}

// unmarshalType is encoding.TextUnmarshaler.
var unmarshalType = reflect.TypeFor[encoding.TextUnmarshaler]()

// parsable reports whether the `default` tag parser reads a value of t from a string.
func parsable(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(unmarshalType) {
		return true
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Pointer:
		return parsable(t.Elem())
	case reflect.Map:
		return parsable(t.Key()) && parsable(t.Elem())
	case reflect.Struct, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Uintptr:
		return false
	default: // numbers, strings, booleans
		return true
	}
}

// fieldValue is the flag of a field whose type pflag has no flag for. It parses a value
// like a `default` tag does: lists and maps are comma-separated, `k:v` for map entries.
type fieldValue struct{ field reflect.Value }

func (f fieldValue) Set(value string) error {
	parsed := reflect.New(f.field.Type()).Elem()
	if err := applyDefault(parsed, value); err != nil {
		return err
	}

	f.field.Set(parsed)

	return nil
}

// String is shown as the default in --help; pflag shows no default for "".
func (f fieldValue) String() string {
	value := f.field
	for value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}

	switch {
	case value.IsZero():
		return ""
	case isTextValue(value.Type()):
		text, _ := textOf(value)

		return text
	default:
		return fmt.Sprint(value.Interface())
	}
}

// Type is the name of the value in --help: `--port Port`.
func (f fieldValue) Type() string {
	if name := f.field.Type().Name(); name != "" {
		return name
	}

	return f.field.Type().String()
}
