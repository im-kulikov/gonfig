package gonfig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Error is a custom error type based on a string.
// It represents an error that is constant and does not change at runtime.
type Error string

// Config controls a loader. The order of the sources is fixed, each one overriding
// the previous ones (see New); the Skip* fields turn a built-in source off.
type Config struct {
	SkipDefaults bool // SkipDefaults set to true will not load config from the 'default' tag.
	SkipEnv      bool // SkipEnv set to true will not load config from environment variables.
	SkipFlags    bool // SkipFlags set to true will not load config from flag parameters.

	// EnvPrefix limits environment variables to those starting with it: with "APP"
	// (or "APP_") the field `env:"NAME"` is read from APP_NAME, and APPLE_X is ignored.
	EnvPrefix string

	// Strict makes file loaders fail on keys that match no field, so a typo
	// in a config file is an error instead of a silently ignored value.
	Strict bool

	// Envs hold the environment variable from which envs will be parsed.
	// By default, it is nil and then os.Environ() will be used.
	Envs []string

	// Args hold the command-line arguments from which flags will be parsed.
	// By default, it is nil and then os.Args[1:] will be used.
	Args []string
}

// loader is the state of one Load (see New): the Config after the options, the
// parsers by type, the order of the custom ones, and what the flags set.
type loader struct {
	Config

	output any
	config string

	printConfig string // the value of --print-config, see PrintConfigFlag
	buffer      io.Writer
	orders      []ParserType
	groups      map[ParserType]Parser

	exit func(int) // used for tests, to ignore os.Exit
}

// LoaderOption customizes a loader: adds parsers (WithYAMLLoader, WithCustomParser),
// changes the Config (WithConfig, WithStrict), the defaults (WithDefaults) or the
// output and exit of --help (WithCustomOutput, WithCustomExit). Options are applied
// in order on every Load. Your own options are made of these, for example:
//
//	func WithProduction() gonfig.LoaderOption {
//		return gonfig.WithOptions([]gonfig.LoaderOption{
//			gonfig.WithStrict(),
//			gonfig.WithConfig(func(c *gonfig.Config) { c.EnvPrefix = "APP" }),
//		})
//	}
type LoaderOption func(*loader) error

// ParserType names a parser. A custom parser of a built-in type (ParserDefaults,
// ParserConfigSet, ParserEnv, ParserFlags) replaces the built-in one at its step;
// any other type adds a parser that runs after the config path is known and
// before environment variables and flags (see WithCustomParser).
type ParserType string

const (
	// ParserDefaults Represents the default parser type that handles configuration values
	//   set by default values in the code or configuration. This parser is typically used to
	//   provide fallback values when other sources do not supply a value.
	ParserDefaults ParserType = "defaults"
	// ParserFlags Represents the parser type that handles command-line flags. This parser
	//   processes the command-line arguments passed to the program to configure various options.
	ParserFlags ParserType = "flags"
	// ParserEnv Represents the parser type that handles environment variables. This parser
	//   reads configuration values from environment variables, which can be used to configure
	//   the application in different deployment environments.
	ParserEnv ParserType = "env"

	// ParserConfigSet Represents the parser type that handles command-line flags. This parser
	//   processes the command-line arguments passed to the program to set the config path.
	ParserConfigSet ParserType = "config-setter"
)

// Error implements the error interface for the Error type.
// It returns the error message as a string, which is the underlying value of the Error.
func (e Error) Error() string { return string(e) }

// singleLine keeps the wrapped error for errors.Is/As but prints it on one line:
// decoders and errors.Join separate messages with newlines, which breaks line-based logs.
type singleLine struct{ error }

func (e singleLine) Error() string {
	lines := strings.FieldsFunc(e.error.Error(), func(r rune) bool { return r == '\n' })
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	return strings.Join(lines, "; ")
}

func (e singleLine) Unwrap() error { return e.error }

// WithCustomParser adds a custom parser, for example a loader of another file format.
// Parsers run in the order they were added; adding a parser of the same type again
// replaces it. A parser of a built-in type replaces the built-in one (see ParserType).
// A nil parser is ignored. To get the path from --config, implement ParserConfigSetter.
//
//	parser := gonfig.New(gonfig.Config{}, gonfig.WithCustomParser(myParser))
func WithCustomParser(p Parser) LoaderOption {
	return func(l *loader) error {
		if p != nil {
			l.addParser(p)
		}

		return nil
	}
}

// addParser registers a parser. A parser of a built-in type (defaults, env, flags,
// config-setter) replaces the built-in one at its step. Any other type runs after
// the config path is known and before env, once per Load: registering the same
// type again replaces the parser.
func (l *loader) addParser(p Parser) {
	typ := p.Type()

	switch typ {
	case ParserDefaults, ParserEnv, ParserFlags, ParserConfigSet:
	default:
		if !slices.Contains(l.orders, typ) {
			l.orders = append(l.orders, typ)
		}
	}

	l.groups[typ] = p
}

// WithCustomParserInit allows the injection of a custom parser into the loader by using a provided
// `ParserInit` function. This function is useful for adding custom logic or additional parsers beyond the
// predefined ones.
//
// The function performs the following tasks:
//   - Accepts a `ParserInit` function (`fabric`) that takes a `Config` and returns a `Parser` and an error.
//   - Executes the `fabric` function with the loader's current `Config` to initialize the custom parser.
//   - If the `fabric` function returns an error, it propagates that error immediately.
//   - Otherwise, it adds the parser to the loader's group under the parser's type.
//
// Parameters:
//   - fabric: A `ParserInit` function that returns a custom `Parser` and an error based on the `Config`.
//
// Returns:
//   - A `LoaderOption` that applies the custom parser to the loader's parser group or returns an error if
//     parser initialization fails.
func WithCustomParserInit(fabric ParserInit) LoaderOption {
	return func(l *loader) error {
		switch parser, err := fabric(l.Config); {
		case err != nil:
			return err
		case parser != nil:
			l.addParser(parser)
		}

		return nil
	}
}

// WithOptions allows dynamic application of loader options by accepting either a slice of LoaderOption
// or a function that returns a slice of LoaderOption. It ensures flexibility in configuring the loader.
//
// The function performs the following tasks:
//   - Accepts `options` as an argument of a type `any`, which can be either a `[]LoaderOption` or
//     a `func() []LoaderOption`.
//   - Uses a type switch to determine the type of `options` and converts it into a `[]LoaderOption`.
//   - Applies each LoaderOption to the provided loader (`l`) by iterating over the result.
//   - If an invalid type is passed to `options`, it returns an error with the message indicating the
//     unexpected type.
//   - If applying any option fails, it returns an error that includes the original error.
//
// Parameters:
// - options: Can either be a slice of `LoaderOption` or a function that returns a slice of `LoaderOption`.
//
// Returns:
//   - A `LoaderOption` that applies the resolved list of options to a given loader, or an error if the
//     `options` type is invalid or if any option fails during application.
func WithOptions(options any) LoaderOption {
	return func(l *loader) error {
		var result []LoaderOption
		switch opts := options.(type) {
		case []LoaderOption:
			result = opts
		case func() []LoaderOption:
			result = opts()
		default:
			return fmt.Errorf("invalid options type: %T", opts)
		}

		for _, opt := range result {
			if err := opt(l); err != nil {
				return fmt.Errorf("could not init options: %w", err)
			}
		}

		return nil
	}
}

// WithCustomOutput sets a custom io.Writer as the loader's output destination.
//
// By default, the loader writes its output to os.Stdout. This LoaderOption allows
// overriding the output destination by providing a custom writer (e.g., a buffer,
// a file, or a mock implementation). Useful for testing or redirecting output in
// specific environments.
//
// Parameters:
//   - writer: An implementation of io.Writer (e.g., os.Stdout, bytes.Buffer).
//     If nil, the default (os.Stdout) remains unchanged.
//
// Returns:
// - A LoaderOption that applies the custom writer to the loader's output buffer.
func WithCustomOutput(writer io.Writer) LoaderOption {
	return func(l *loader) error {
		if writer != nil {
			l.buffer = writer
		}

		return nil
	}
}

// WithCustomExit provides an option to override the default exit behavior of the loader.
// This is useful for testing, where calling `os.Exit` would terminate the test process.
//
// By default, `loader.exit` is set to `os.Exit`, but this function allows replacing it
// with a custom exit function (e.g., a no-op or mock function).
//
// Parameters:
//   - exit: A custom function that takes an exit code as an argument. If nil, the default behavior remains unchanged.
//
// Returns:
//   - A LoaderOption function that applies the custom exit behavior to the loader.
func WithCustomExit(exit func(int)) LoaderOption {
	return func(l *loader) error {
		if exit == nil {
			return nil
		}

		l.exit = exit

		return nil
	}
}

// WithConfig allows modifying the Config object using a custom handler function.
// This LoaderOption provides a way to customize configuration settings dynamically
// before the loading process begins.
//
// The provided handler function receives a pointer to the Config object, allowing
// modifications to be applied as needed.
//
// Parameters:
// - handler: A function that takes a *Config and applies custom modifications.
//
// Returns:
// - A LoaderOption that applies the given handler to modify the loader's Config.
func WithConfig(handler func(*Config)) LoaderOption {
	return func(l *loader) error {
		if handler != nil {
			handler(&l.Config)
		}

		return nil
	}
}

// WithStrict makes file loaders fail on keys that match no field (see Config.Strict).
func WithStrict() LoaderOption {
	return WithConfig(func(c *Config) { c.Strict = true })
}

// WithDefaults sets default values for the loader's output structure.
//
// This function takes a tag-key and map of default values and applies them to the target structure
// using `decodeMapToStruct`. It ensures that any unset fields in the structure receive
// the specified default values before other parsing mechanisms (such as environment
// variables or configuration files) are applied.
//
// This function is useful when:
// - You want to provide fallback values for missing configurations.
// - You need to ensure a structure is always initialized with meaningful defaults.
//
// Nested structs take nested maps: {"path": {"to": {"key": value}}} for struct{Path struct{To struct{Key string}}},
// keyed by the given tag. Dotted keys such as "path.to.key" are not supported.
//
// Parameters:
// - keyTag: allows using struct-tag to find field names.
// - defaults: A map where keys are field names (or tagged keys) and values are default values.
//
// Returns:
// - A `LoaderOption` function that applies the default values to the loader's output.
func WithDefaults(keyTag string, defaults map[string]any) LoaderOption {
	return func(l *loader) error {
		return decodeMapToStruct(l.output, defaults, keyTag)
	}
}

// newLoader returns the state of one Load. Every Load gets its own, so one Parser
// can be used from several goroutines, and a Load does not see the config path
// or parsers of the previous one.
func newLoader(c Config, output any) *loader {
	return &loader{
		Config: c,
		output: output,
		exit:   os.Exit,
		buffer: os.Stdout,
		groups: make(map[ParserType]Parser, 4),
	}
}

// setLoaderDefaults fills what the options left unset: Envs and Args of the process,
// and the built-in parsers that Config.Skip* does not disable and a custom parser
// of the same type does not replace. It runs after the options, so the settings
// changed through WithConfig apply.
func (l *loader) setLoaderDefaults() {
	if l.Envs == nil {
		l.Envs = os.Environ()
	}

	if l.Args == nil {
		l.Args = os.Args[min(1, len(os.Args)):] // os.Args may be empty in an embedded runtime
	}

	builtins := []struct {
		skip   bool
		parser Parser
	}{
		{l.SkipDefaults, newDefaultParser()},
		{l.SkipFlags, parseConfigPath(l)},
		{l.SkipEnv, newEnvLoader(l)},
		{l.SkipFlags, newFlagsLoader(l)},
	}

	for _, builtin := range builtins {
		if _, replaced := l.groups[builtin.parser.Type()]; !replaced && !builtin.skip {
			l.groups[builtin.parser.Type()] = builtin.parser
		}
	}
}

// New creates a Parser. Every Load applies the options to a fresh loader, then runs
// the parsers in the order of priority, each overriding the previous ones:
//
//  1. defaults from `default` tags;
//  2. the config path, pre-scanned from the flags;
//  3. custom parsers, such as file loaders, in the order they were added;
//  4. environment variables;
//  5. flags.
//
// Parsers disabled by Config.Skip* are left out. Then --print-config is handled,
// required fields are checked and Validate is called for v and the structs inside it
// that implement LoaderValidator.
// The Parser can be used from several goroutines.
//
//nolint:ireturn
func New(config Config, options ...LoaderOption) Parser {
	return &parserFunc{call: func(v any) error {
		l := newLoader(config, v)

		for _, option := range options {
			if err := option(l); err != nil {
				return fmt.Errorf("gonfig: could not init option: %w", err)
			}
		}

		l.setLoaderDefaults()

		return wrapUsageLoader(l, l.load)(v)
	}}
}

func (l *loader) load(v any) error {
	order := slices.Concat([]ParserType{ParserDefaults, ParserConfigSet}, l.orders, []ParserType{ParserEnv, ParserFlags})

	for _, typ := range order {
		parser, ok := l.groups[typ]
		if !ok { // disabled by Config.Skip*
			continue
		}

		if setter, ok := parser.(ParserConfigSetter); ok {
			setter.SetConfigPath(l.configPathFor(typ))
		}

		if setter, ok := parser.(strictSetter); ok {
			setter.setStrict(l.Strict)
		}

		if err := parser.Load(v); err != nil {
			return fmt.Errorf("gonfig: could not load: %w", err)
		}
	}

	if l.printConfig != "" {
		return l.print(v)
	}

	if err := ValidateRequiredFields(v); err != nil {
		return fmt.Errorf("gonfig: %w", err)
	}

	return validate(v)
}

// print writes the loaded config for --print-config and exits, like --help does.
func (l *loader) print(v any) error {
	if err := Write(l.buffer, v, Format(l.printConfig), WithEnvPrefix(l.EnvPrefix)); err != nil {
		return fmt.Errorf("gonfig: could not print config: %w", err)
	}

	l.exit(0)

	return ErrTestExit
}

// configPathFor returns the config path for the parser of the given type. Of several
// file loaders only one reads the file: the one of its extension, or the first file
// loader when none has it. Other parsers, custom ones included, get the path as is.
func (l *loader) configPathFor(typ ParserType) string {
	if _, isFile := fileFormats[typ]; !isFile {
		return l.config
	}

	ext := strings.ToLower(filepath.Ext(l.config))

	var reader ParserType
	for _, other := range l.orders {
		if format, ok := fileFormats[other]; ok && slices.Contains(format.extensions, ext) {
			reader = other

			break
		} else if ok && reader == "" {
			reader = other // the first file loader, unless one has the extension
		}
	}

	if typ != reader {
		return ""
	}

	return l.config
}

// fileFormat is the format of the file loader that reads the config path, so the
// output of --print-config reads back through --config; without a path the format
// of the first file loader, or YAML without one.
func (l *loader) fileFormat() Format {
	format := FormatYAML

	for _, typ := range slices.Backward(l.orders) {
		if file, ok := fileFormats[typ]; ok {
			if l.config != "" && l.configPathFor(typ) != "" {
				return Format(file.tag)
			}

			format = Format(file.tag)
		}
	}

	return format
}

// Load initializes a new Parser with default settings and applies optional LoaderOptions.
// It then loads the provided target structure using the configured loader service.
//
// This function is shorthand for creating a new Parser with default Config and calling Load on it.
//
// Parameters:
// - v: The target structure where the configuration will be loaded.
// - options: Optional LoaderOptions to customize the behavior of the parser.
//
// Returns:
// - An error if the loading process fails, otherwise nil.
func Load(v any, options ...LoaderOption) error {
	return New(Config{}, options...).Load(v)
}
