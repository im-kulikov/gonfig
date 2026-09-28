package gonfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
)

// EnvUsageOption defines a function type used to configure options for environment variable usage.
type EnvUsageOption func(*envUsageOptions)

// envUsageOptions holds configuration options for generating environment variable usage information.
type envUsageOptions struct {
	prefix string // Optional prefix to be added to environment variable names.
}

// envUsage represents metadata about an environment variable, including its name, usage description, and type.
// This struct is typically used to store and display information about environment variables in a user-friendly format.
//
// Fields:
// - Usage: A description of how the environment variable is intended to be used.
// - Name: The name of the environment variable.
// - Type: The expected data type of the environment variable (e.g., string, int, bool).
//
// This struct is useful when documenting or parsing environment variables in an application.
type envUsage struct {
	Usage string
	Name  string
	Type  string
}

const (
	// envPairDelim is the delimiter used to separate the environment variable name from its value.  Example: "KEY=value".
	envPairDelim = "="

	// envDelimiter is the delimiter used to separate different parts of a composite environment variable name.
	// It's typically used in multipart names where sections are separated by underscores.
	// Example: "APP_CONFIG_PATH".
	envDelimiter = "_"

	// envTag defines the struct tag key used to specify environment variable names for struct fields.
	// When parsing struct tags, this key indicates that a field should be populated from an environment variable.
	// Example usage: `env:"DB_HOST"`.
	envTag = "env"

	// ErrTestExit is returned by Load after --help or --print-config when the exit
	// function set by WithCustomExit returns instead of ending the process, as in tests.
	ErrTestExit = Error("exit code")

	// ErrPrepareDecoder is returned when the decoder initialization fails.
	// This error typically occurs when setting up a configuration decoder
	// encounters an issue, such as invalid decoder settings or unsupported types.
	ErrPrepareDecoder = Error("could not prepare decoder")

	// ErrDecode is returned when decoding a configuration fails.
	// This error indicates that the process of converting configuration data
	// into the expected structure was unsuccessful, possibly due to type mismatches
	// or missing required fields.
	ErrDecode = Error("could not decode")
)

// newEnvLoader creates a new parser that loads configuration from environment variables.
// It uses the provided environment variable slice and prefix to populate the configuration.
// Returns a Parser that processes environment variables with the specified prefix.
func newEnvLoader(l *loader) *parserFunc {
	return &parserFunc{name: ParserEnv, call: func(v any) error {
		return decodeMap(v, PrepareEnvs(l.Envs, l.EnvPrefix), decodeOptions{
			tag: envTag, inline: envInlineOption, env: true, noTags: l.SkipDefaults, prefix: l.EnvPrefix,
		})
	}}
}

// EnvUsageWithPrefix creates an EnvUsageOption that sets a prefix for environment variables.
// This prefix is applied to each environment variable name when generating usage information.
//
// Parameters:
//   - prefix: The string prefix to add to environment variable names.
//
// Returns:
//   - EnvUsageOption: A function that modifies the prefix in envUsageOptions.
func EnvUsageWithPrefix(prefix string) EnvUsageOption {
	return func(opts *envUsageOptions) { opts.prefix = prefix }
}

// UsageOfEnvs generates a human-readable string that describes the environment variables
// expected by a given structure, based on struct tags (e.g., "env" and "usage").
//
// Parameters:
//   - dest: A pointer to a struct that defines the expected environment variables.
//     The struct fields must use the "env" tag to define environment variable names
//     and the "usage" tag to describe their purpose.
//   - opts: Optional EnvUsageOption(s) to configure behavior, such as adding a prefix to environment variable names.
//
// Returns:
//   - A string describing the environment variables and their usage, or an empty string if there are none
//     or the input is not valid.
//
// The function ensures that the input is a pointer to a struct. It traverses the struct fields,
// generating usage information based on the tags. If a struct field is another struct, it recurses
// into the nested fields.
//
//nolint:funlen,gocognit
func UsageOfEnvs(dest any, opts ...EnvUsageOption) string {
	output := make([]envUsage, 0)
	exists := make(map[string]struct{})
	for field, err := range ReflectFieldsOf(withSections(dest), ReflectOptions{CanSet: new(true), Pointers: true}) {
		if err != nil {
			return ""
		}

		name, ok := envNameOf(field)
		if value := derefType(field.Value.Type()); !ok || value.Kind() == reflect.Struct && !textStruct(value) {
			continue // not set by env, or a section of a type that contains itself: no single variable
		}

		if field.Value.Kind() == reflect.Map { // its entries are variables of their own
			name += envDelimiter + "<key>"
		}

		if _, ok := exists[name]; ok {
			continue
		}

		exists[name] = struct{}{}

		var usage string
		if usage = field.Field.Tag.Get(FlagTagUsage); usage != "" {
			usage = " — " + strings.ReplaceAll(usage, "\n", "\n    ")
		}

		if isSecret(field) {
			usage += " (secret)"
		} else if tmp := field.Field.Tag.Get(defaultTagName); tmp != "" {
			usage += fmt.Sprintf(" (default: %s)", tmp)
		}

		output = append(output, envUsage{Usage: usage, Name: name, Type: field.Value.Type().String()})
	}

	var options envUsageOptions
	for _, opt := range opts {
		opt(&options)
	}

	prefix := envPrefix(options.prefix)

	if len(output) == 0 {
		return ""
	}

	out := make([]string, 0, len(output))
	for _, item := range output {
		out = append(out, fmt.Sprintf("  - '%s%s' <%s>%s", prefix, item.Name, item.Type, item.Usage))
	}

	return fmt.Sprintf("Environment variables:\n%s", strings.Join(out, "\n"))
}

// envNameOf returns the environment variable of a field, without the prefix: the env
// tags from the top struct down, joined with "_". An embedded struct adds no segment,
// as mapstructure inlines it, and nor does a struct tagged `env:",squash"`. False if env
// cannot set the field: no env tag, or one on the way, or `env:"-"`.
func envNameOf(field *ReflectValue) (string, bool) {
	var name string

	for parent := field; parent.Owner != nil; parent = parent.Owner {
		parts := strings.Split(parent.Field.Tag.Get(envTag), ",")

		switch {
		case parts[0] == "-":
			return "", false
		case parent != field && parent.Field.Anonymous,
			parts[0] == "" && slices.Contains(parts[1:], envInlineOption):
		case parts[0] == "":
			return "", false
		case name == "":
			name = parts[0]
		default:
			name = parts[0] + envDelimiter + name
		}
	}

	return name, name != ""
}

// lookupEnv returns the value of the variable name in envs, the last one set, as the
// env loader takes it; the name matches case-insensitively, as a field does.
func lookupEnv(envs []string, name, otherwise string) string {
	for _, env := range slices.Backward(envs) {
		if key, value, ok := strings.Cut(env, envPairDelim); ok && strings.EqualFold(key, name) {
			return value
		}
	}

	return otherwise
}

// envPrefix returns the prefix of environment variable names with exactly one
// trailing "_", so "APP" and "APP_" both mean APP_NAME, and APPLE_X is not included.
func envPrefix(prefix string) string {
	if prefix = strings.TrimSuffix(prefix, envDelimiter); prefix == "" {
		return ""
	}

	return prefix + envDelimiter
}

// withSections returns a new zero value of the struct dest points to, with every
// section (a pointer to a struct) allocated, so that the help lists the variables
// inside sections too: env creates them. dest itself is not changed. A section of
// a type that contains itself is left nil. Anything but a pointer to a struct is
// returned as is, for ReflectFieldsOf to report.
func withSections(dest any) any {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return dest
	}

	template := reflect.New(v.Elem().Type())
	allocateSections(template.Elem(), []reflect.Type{v.Elem().Type()})

	return template.Interface()
}

func allocateSections(v reflect.Value, path []reflect.Type) {
	for field, value := range v.Fields() {
		elem := field.Type
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		switch {
		case elem.Kind() != reflect.Struct || textStruct(elem):
		case field.Type.Kind() == reflect.Struct:
			allocateSections(value, path)
		case value.CanSet() && !slices.Contains(path, elem):
			value.Set(reflect.New(elem))
			allocateSections(value.Elem(), append(slices.Clip(path), elem))
		}
	}
}

// wrapUsageLoader wraps the provided loader function to add additional functionality
// for handling help flags and printing environment variable usage. It ensures that when
// the help flag (`--help`) is provided, the program prints the environment variable usage
// and exits gracefully. This function is typically used to augment the configuration loading
// mechanism.
//
// The wrapped handler function behaves as follows:
//  1. If the handler returns an error equal to `pflag.ErrHelp`, it prints environment variable
//     usage (with an optional prefix) and terminates the program.
//  2. If any other error occurs during the handler execution, the error is returned.
//  3. On successful execution of the handler without errors, it proceeds normally.
//
// Params:
// - l: The *loader, which contains the `EnvPrefix`, the output and the exit function.
// - handler: The function responsible for loading the configuration (e.g., from flags or envs).
//
// Returns:
// - A new function that wraps the original handler with additional error handling and help output logic.
func wrapUsageLoader(l *loader, handler func(any) error) func(any) error {
	return func(v any) error {
		// Attempt to load the configuration
		if err := handler(v); errors.Is(err, pflag.ErrHelp) {
			// If the error is the help flag, print the variables the env loader reads
			if usage := UsageOfEnvs(v, EnvUsageWithPrefix(l.EnvPrefix)); usage != "" && !l.SkipEnv {
				writeln(l.buffer)
				writeln(l.buffer, usage)
			}

			// Handle program exit for tests or production
			l.exit(0)

			// allows tests to proceed without terminating the program
			return ErrTestExit
		} else if err != nil {
			// Return any other errors from the loader
			return err
		}

		return nil
	}
}

func writeln(out io.Writer, a ...any) {
	_, _ = fmt.Fprintln(out, a...)
}

// PrepareEnvs prepares a map from the given environment variable slice.
// It filters and parses the environment variables based on the provided prefix.
// The resulting map has a nested structure based on the environment variable names,
// using the specified delimiter for nesting.
func PrepareEnvs(envs []string, prefix string) map[string]any {
	prefix = envPrefix(prefix)

	out := make(map[string]any, len(envs))
	for _, env := range envs {
		var ok bool
		if env, ok = strings.CutPrefix(env, prefix); !ok {
			continue
		}

		parts := strings.SplitN(env, envPairDelim, 2)
		if len(parts) != 2 {
			continue
		}

		keys := strings.Split(parts[0], envDelimiter)

		// Insert into a map with the correct nesting
		insertIntoMap(out, keys, parts[1])
	}

	return out
}

// insertIntoMap inserts the value into the map with the specified keys.
// The keys define the nesting level of the map. If the keys are exhausted, the value is set.
// This function creates nested maps as needed to match the structure defined by the keys.
//
// A name can be a value and the start of other names at once (DB and DB_HOST):
// then it holds the nested map and keeps its own value under the "" key, whatever
// the order of the variables. envClashHook picks what the field needs.
func insertIntoMap(m map[string]any, keys []string, value any) {
	setLeaf(m, strings.Join(keys, envDelimiter), value)

	// A name with an empty segment (A__B, _A, A_) is not nested: "" is the key of the own value.
	if len(keys) == 1 || slices.Contains(keys, "") {
		return
	}

	nested, ok := m[keys[0]].(map[string]any)
	if !ok {
		nested = make(map[string]any)
		if own, exists := m[keys[0]]; exists {
			nested[""] = own
		}

		m[keys[0]] = nested
	}

	insertIntoMap(nested, keys[1:], value)
}

// setLeaf sets the value of a name, next to the nested names under it, if any.
func setLeaf(m map[string]any, key string, value any) {
	if nested, ok := m[key].(map[string]any); ok {
		nested[""] = value

		return
	}

	m[key] = value
}

// envClashHook resolves a name that is both a variable and the start of other ones
// (see insertIntoMap) before a struct or a map is decoded: a field of a struct or
// map type takes the nested names and ignores the variable (DB and DB_HOST), any
// other field takes the variable and ignores the nested names, which belong to
// something else (NAME and NAME_SUFFIX). Without it the whole loading failed.
func envClashHook(_, target reflect.Type, data any) (any, error) {
	tree, ok := data.(map[string]any)
	if !ok {
		return data, nil
	}

	switch target.Kind() {
	case reflect.Struct:
		return resolveClashes(maps.Clone(tree), target), nil
	case reflect.Map:
		return mapEntries(tree, target.Elem()), nil
	}

	return tree, nil
}

// mapEntries picks the entries of a map from the names under its own (see
// insertIntoMap). A map of values takes the rest of a name as the key, so a key may
// hold "_": LABELS_team_name=core is team_name. A map of structs or maps takes the
// first segment, the rest names a field: DBS_main_HOST=db is main, HOST=db.
func mapEntries(tree map[string]any, elem reflect.Type) map[string]any {
	container := isContainer(elem)
	entries := make(map[string]any, len(tree))

	for key, value := range tree {
		nested, isTree := value.(map[string]any)
		own, hasOwn := nested[""]

		switch {
		case key == "": // the value of the map's own name, next to its entries
		case container && isTree:
			entries[key] = nested
		case container: // a joined name: its segments are in the tree of the first one
		case !isTree:
			entries[key] = value
		case hasOwn: // LABELS_team next to LABELS_team_name
			entries[key] = own
		}
	}

	return entries
}

// isContainer reports whether env sets a value of t through the names under its own:
// a struct (not one read from text) or a map, or a pointer to one.
func isContainer(t reflect.Type) bool {
	t = derefType(t)

	return t.Kind() == reflect.Map || t.Kind() == reflect.Struct && !textStruct(t)
}

// resolveClashes fixes the values of the fields of t in tree, see envClashHook.
func resolveClashes(tree map[string]any, t reflect.Type, inlined ...reflect.Type) map[string]any {
	if slices.Contains(inlined, t) { // a struct that embeds a pointer to itself
		return tree
	}

	for field := range t.Fields() {
		parts := strings.Split(field.Tag.Get(envTag), ",")

		kind := derefType(field.Type)
		container := isContainer(kind)

		switch key, found := lookupKey(tree, parts[0]); {
		case envInlined(field, parts, kind):
			resolveClashes(tree, kind, append(inlined, t)...) // inlined: its fields read from this level
		case !found:
		case container:
			nested, isTree := tree[key].(map[string]any)
			if !isTree || field.Type.Kind() == reflect.Pointer && !setsField(nested, kind) {
				delete(tree, key) // DB=x next to the struct DB; TLS_OTHER, no field of the section TLS
			}
		default:
			if nested, isTree := tree[key].(map[string]any); isTree {
				tree[key] = nested[""] // NAME next to NAME_SUFFIX
				if tree[key] == nil {
					delete(tree, key) // NAME_SUFFIX alone
				}
			}
		}
	}

	return tree
}

// envInlined reports whether env reads the fields of a struct field from the level of
// its parent: an embedded struct, whatever its tag, or one tagged `env:",squash"`.
func envInlined(field reflect.StructField, parts []string, kind reflect.Type) bool {
	return kind.Kind() == reflect.Struct && !textStruct(kind) &&
		(field.Anonymous || parts[0] == "" && slices.Contains(parts[1:], envInlineOption))
}

// setsField reports whether the names under the name of a struct or a map t set
// anything in it: a section, a pointer to a struct, is created only then.
func setsField(tree map[string]any, t reflect.Type, inlined ...reflect.Type) bool {
	if t.Kind() == reflect.Map {
		return len(tree) > 0
	}

	if slices.Contains(inlined, t) { // a struct that embeds a pointer to itself
		return false
	}

	for field := range t.Fields() {
		parts := strings.Split(field.Tag.Get(envTag), ",")
		kind := derefType(field.Type)
		key, found := lookupKey(tree, parts[0])
		nested, isTree := tree[key].(map[string]any)
		_, own := nested[""]

		switch {
		case envInlined(field, parts, kind):
			if setsField(tree, kind, append(inlined, t)...) {
				return true
			}
		case !found:
		case isContainer(kind) && isTree && setsField(nested, kind),
			!isContainer(kind) && (!isTree || own):
			return true
		}
	}

	return false
}

// lookupKey finds the key of a field as mapstructure does: exact, else case-insensitive.
func lookupKey(tree map[string]any, name string) (string, bool) {
	if _, ok := tree[name]; ok || name == "" {
		return name, ok
	}

	for key := range tree {
		if strings.EqualFold(key, name) {
			return key, true
		}
	}

	return "", false
}

// sectionDefaultsHook gives a section a source is about to create, a nil pointer to a
// struct, the values of its `default` tags first, as the root of the config has them;
// the source then overrides what it sets. A section no source sets stays nil.
func sectionDefaultsHook(from, to reflect.Value) (any, error) {
	if to.Kind() == reflect.Pointer && to.IsNil() && to.CanSet() &&
		to.Type().Elem().Kind() == reflect.Struct && !textStruct(to.Type().Elem()) {
		section := reflect.New(to.Type().Elem())
		if err := SetDefaults(section.Interface()); err != nil {
			return nil, err
		}

		to.Set(section)
	}

	return from.Interface(), nil
}

// textStruct reports whether a struct is read from a string: net.IPNet, time.Time
// or any other encoding.TextUnmarshaler.
func textStruct(t reflect.Type) bool {
	return t == ipNetType || reflect.PointerTo(t).Implements(unmarshalType)
}

// decodeHooks converts the provided data into the target type using type-specific parsing.
// It supports basic types, time.Duration, IP-related types and encoding.TextUnmarshaler.
// It returns the parsed value or an error if the conversion fails.
//
//nolint:ireturn
func decodeHooks() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		plainHook,
		yamlScalarHook,
		jsonNumberHook,
		numberRangeHook,
		tomlTimeHook,
		mapstructure.TextUnmarshallerHookFunc(),
		splitListHook,
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToBasicTypeHookFunc(),

		// decode net-values
		mapstructure.StringToIPHookFunc(),
		mapstructure.StringToIPNetHookFunc(),

		replaceListHook)
}

// splitListHook reads a list or an array from a comma-separated string ("a,b" from env):
// mapstructure then decodes each item into the element type with these same hooks.
func splitListHook(from, to reflect.Value) (any, error) {
	if from.Kind() != reflect.String || !isList(to.Kind()) {
		return from.Interface(), nil
	}

	if from.String() == "" {
		return []string{}, nil
	}

	return strings.Split(from.String(), ","), nil
}

// replaceListHook empties a list or an array before a source writes it, so the source
// replaces it, as a file did with yaml.v3: mapstructure would keep the old items past
// the new ones, write into the backing array of the caller, and skip the length check
// of an array that is not empty.
func replaceListHook(from, to reflect.Value) (any, error) {
	if isList(to.Kind()) && isList(from.Kind()) && to.CanSet() {
		to.Set(reflect.Zero(to.Type()))
	}

	return from.Interface(), nil
}

func isList(kind reflect.Kind) bool { return kind == reflect.Slice || kind == reflect.Array }

// Tag options that inline a named struct field into its parent: `env:",squash"`
// for environment variables, `yaml:",inline"` (and json/toml alike) for files.
// Embedded structs are inlined without any option.
const (
	envInlineOption  = "squash"
	fileInlineOption = "inline"
)

// decodeOptions describe how a generic map is decoded into a struct.
type decodeOptions struct {
	tag      string // struct tag with the key names
	inline   string // tag option that inlines a named struct field
	untagged bool   // match fields without the tag by their name (files); env skips them
	strict   bool   // keys that match no field are an error (files only: env holds every variable)
	env      bool   // resolve a name that is both a variable and the start of others (envClashHook)
	noTags   bool   // Config.SkipDefaults: a section a source creates does not get its `default` tags
	prefix   string // Config.EnvPrefix, to name the variables in errors (env only)
}

func decodeMapToStruct(dest any, from map[string]any, tag string, skipDefaults bool) error {
	return decodeMap(dest, from, decodeOptions{tag: tag, inline: envInlineOption, noTags: skipDefaults})
}

// decodeMap is the single decoder behind environment variables, files and
// WithDefaults, so all of them treat embedding and value types the same way.
func decodeMap(dest any, from map[string]any, options decodeOptions) error {
	conf := &mapstructure.DecoderConfig{
		Result:               dest,
		TagName:              options.tag,
		Squash:               true,
		SquashTagOption:      options.inline,
		IgnoreUntaggedFields: !options.untagged,
		ErrorUnused:          options.strict,
		DecodeHook:           decodeHooks(),
	}

	if !options.noTags {
		conf.DecodeHook = mapstructure.ComposeDecodeHookFunc(sectionDefaultsHook, conf.DecodeHook)
	}

	if options.env {
		conf.DecodeHook = mapstructure.ComposeDecodeHookFunc(envClashHook, conf.DecodeHook)
	}

	if options.inline == fileInlineOption { // mapstructure cannot inline a map: inlineHook inlines
		conf.SquashTagOption = noTagSquash
		conf.DecodeHook = mapstructure.ComposeDecodeHookFunc(inlineHook(options.tag), conf.DecodeHook)
	}

	dec, err := mapstructure.NewDecoder(conf)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPrepareDecoder, err)
	}

	if err = dec.Decode(from); err != nil {
		// mapstructure prefixes the list of errors with a multi-line header.
		if inner := errors.Unwrap(err); inner != nil {
			err = inner
		}

		if options.env {
			err = envNames(err, envPrefix(options.prefix))
		}

		return fmt.Errorf("%w: %w", ErrDecode, singleLine{err})
	}

	return nil
}

// LoadEnvs decodes the provided environment variables map into the destination object.
// It uses mapstructure to map the environment variables to the fields of the destination
// object based on the "env" tag. It returns an error if decoding fails.
func LoadEnvs(envs map[string]any, dest any) error {
	return decodeMap(dest, envs, decodeOptions{tag: envTag, inline: envInlineOption, env: true})
}

// envNames names the variables in an error of the env decoder: mapstructure names a
// field by its keys, 'DB.PORT' or 'LABELS[team]', the user has set APP_DB_PORT.
func envNames(err error, prefix string) error {
	switch e := err.(type) { //nolint:errorlint // the errors of mapstructure, as it returns them
	case interface{ Unwrap() []error }:
		errs := e.Unwrap()
		for i := range errs {
			errs[i] = envNames(errs[i], prefix)
		}

		return errors.Join(errs...)
	case *mapstructure.DecodeError:
		return fmt.Errorf("%s%s: %w", prefix, envNameReplacer.Replace(e.Name()), e.Unwrap())
	}

	return err
}

var envNameReplacer = strings.NewReplacer(".", envDelimiter, "[", envDelimiter, "]", "")

// jsonNumberHook turns json.Number from JSON files into a Go number before the
// other hooks run: they expect a plain string or number and would misread it.
func jsonNumberHook(_, _ reflect.Type, data any) (any, error) {
	if number, ok := data.(json.Number); ok {
		return jsonNumber(number), nil
	}

	return data, nil
}

// jsonNumber is an int64, else an uint64, else a float64: int64 values beyond 2^53
// keep their precision.
func jsonNumber(number json.Number) any {
	if v, err := number.Int64(); err == nil {
		return v
	}

	if v, err := strconv.ParseUint(number.String(), 10, 64); err == nil {
		return v
	}

	v, _ := number.Float64() // the JSON decoder has checked the syntax; a huge number is ±Inf

	return v
}

// plainHook gives a field of type any the plain values of a file, inside lists and
// maps too: YAML scalars their resolved value, JSON numbers a float64, as encoding/json.
func plainHook(_, to reflect.Type, data any) (any, error) {
	if to.Kind() != reflect.Interface {
		return data, nil
	}

	return plain(data), nil
}

func plain(data any) any {
	switch value := data.(type) {
	case yamlScalar:
		return value.value
	case json.Number:
		v, _ := value.Float64()

		return v
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = plain(item)
		}

		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = plain(item)
		}

		return out
	}

	return data
}

// numberRangeHook rejects a number that does not fit its field, as env and flags
// do: mapstructure would wrap 70000 around in an uint16 and cut 2.9 to 2 in an int.
// A float field takes any number but one beyond the range of float32.
func numberRangeHook(_, to reflect.Type, data any) (any, error) {
	from := reflect.ValueOf(data)
	if data == nil || !isNumber(from.Kind()) || !isNumber(to.Kind()) {
		return data, nil
	}

	value := from.Convert(to)

	flipped := from.CanInt() && from.Int() < 0 && value.CanUint() || from.CanUint() && value.CanInt() && value.Int() < 0
	fits := !flipped && value.Convert(from.Type()).Interface() == data // kept the sign, high bits and fraction

	if value.CanFloat() {
		fits = !math.IsInf(value.Float(), 0) || from.CanFloat() && math.IsInf(from.Float(), 0)
	}

	if !fits {
		return nil, fmt.Errorf("%v does not fit in %s", data, to)
	}

	return data, nil
}

func isNumber(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Float64 && kind != reflect.Uintptr
}
