package gonfig

import (
	"fmt"
	"os"
)

// Config holds the configuration options for loading settings using various parsers such as defaults,
// environment variables, and command-line flags.
//
// Fields:
// - SkipDefaults: If true, the loader will skip loading configurations from the 'default' tags in struct fields.
// - SkipEnv: If true, the loader will skip loading configurations from environment variables.
// - SkipFlags: If true, the loader will skip loading configurations from command-line flags.
//
//   - EnvPrefix: A string that specifies a prefix for filtering environment variables. Only variables
//     starting with this prefix will be considered.
//
//   - LoaderOrder: Defines the order in which the parsers (defaults, env, flags) will be executed.
//     This allows prioritization of certain parsers over others.
//
//   - Envs: A slice of environment variables to be used for parsing. If left nil, the loader will
//     default to using `os.Environ()`.
//
//   - Args: A slice of command-line arguments to be used for parsing. If left nil, the loader will
//     default to using `os.Args`. This can be explicitly parsed by the user if required.
//
// `loader` struct:
// - Embeds `Config` to inherit its configuration options.
// - `groups`: A map that holds the different `Parser` implementations, indexed by `ParserType`.
//
// Types:
// - `LoaderOption`: A function type used to apply custom options to the `loader`.
// - `ParserType`: Represents the type of parser (e.g., "defaults", "env", "flags").
//
// Constants:
// - `ParserDefaults`: Represents the default parser that loads configurations based on struct tags.
// - `ParserFlags`: Represents the parser that loads configurations from command-line flags.
// - `ParserEnv`: Represents the parser that loads configurations from environment variables.
//
// Example Usage:
// Custom parsers can be injected using `LoaderOption` functions, such as `WithCustomParser`.
type Config struct {
	SkipDefaults bool // SkipDefaults set to true will not load config from 'default' tag.
	SkipEnv      bool // SkipEnv set to true will not load config from environment variables.
	SkipFlags    bool // SkipFlags set to true will not load config from flag parameters.

	EnvPrefix string // EnvPrefix for environment variables.

	// LoaderOrder allows to set priority for Parser's.
	LoaderOrder []ParserType

	// Envs hold the environment variable from which envs will be parsed.
	// By default, is nil and then os.Environ() will be used.
	Envs []string

	// Args hold the command-line arguments from which flags will be parsed.
	// By default, is nil and then os.Args will be used.
	// Unless loader.Flags() will be explicitly parsed by the user.
	Args []string
}

// loader is responsible for managing the configuration loading process by coordinating different parsers.
// It embeds the `Config` struct and contains a map of `Parser` implementations.
//
// Fields:
//
//   - Config: The embedded configuration settings that control how the loader behaves. This includes
//     flags for skipping defaults, environment variables, or command-line flags, as well as any custom
//     environment variables and arguments provided.
//
//   - groups: A map where the keys are `ParserType` values (such as "defaults", "env", and "flags"),
//     and the values are `Parser` instances. This map allows the loader to invoke the correct parser
//     based on the order specified in `LoaderOrder` from the `Config`.
//
// The `loader` is initialized with a set of defaults and custom options can be added through
// `LoaderOption` functions. Each parser in the `groups` map is responsible for loading part of the
// configuration from its respective source (e.g., defaults, environment variables, or flags).
//
// Example:
// A `loader` might have parsers for environment variables and flags configured, and it would
// execute them in the order defined by `LoaderOrder`, applying each configuration in sequence.
type loader struct {
	Config
	groups map[ParserType]Parser
}

// LoaderOption defines a function type used to customize the behavior of the loader.
// Each `LoaderOption` takes a pointer to a `loader` and returns an error if the customization fails.
//
// The purpose of `LoaderOption` is to allow for flexible configuration of the loader instance.
// Options can be applied to modify the loader’s behavior, such as adding custom parsers,
// modifying existing parsers, or changing configuration settings.
//
// Usage:
// A `LoaderOption` function is passed to the `New` function or similar, allowing for dynamic
// configuration. Multiple options can be combined, and each one will be applied to the loader
// sequentially.
//
// Example:
//
//	func WithCustomSetting(setting string) LoaderOption {
//	    return func(l *loader) error {
//	        // modify loader based on custom setting
//	        l.Config.SomeSetting = setting
//	        return nil
//	    }
//	}
type LoaderOption func(*loader) error

// ParserType represents the different types of parsers that can be used in the loader system.
// It is defined as a string to allow for flexible and extensible parser type definitions.
//
// Each value of `ParserType` corresponds to a specific source or method of configuration parsing.
// The parser types determine the priority and the sequence in which the parsers are applied.
//
// Constants:
//
//   - ParserDefaults: Represents the default parser type that handles configuration values
//     set by default values in the code or configuration. This parser is typically used to
//     provide fallback values when other sources do not supply a value.
//
//   - ParserFlags: Represents the parser type that handles command-line flags. This parser
//     processes the command-line arguments passed to the program to configure various options.
//
//   - ParserEnv: Represents the parser type that handles environment variables. This parser
//     reads configuration values from environment variables, which can be used to configure
//     the application in different deployment environments.
//
// Example usage:
// To specify the order in which parsers should be applied, you can set the `LoaderOrder`
// field in the `Config` structure with these constants, e.g.,
//
//	config := Config{
//	    LoaderOrder: []ParserType{ParserDefaults, ParserEnv, ParserFlags},
//	}
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
)

// WithCustomParser creates a LoaderOption that adds a custom parser to the loader's group of parsers.
// This function allows you to inject a parser into the loader, which will be used to handle a specific
// type of configuration source. The custom parser will be added to the loader's parser group, enabling
// it to be invoked during the configuration loading process.
//
// Parameters:
//   - p: The custom parser to be added to the loader. The parser must implement the `Parser` interface.
//     If the provided parser is `nil`, no action is taken and the function returns `nil`.
//
// Returns:
//   - A `LoaderOption` function that, when applied to a `loader`, will add the custom parser to the loader's
//     group of parsers.
//
// Example usage:
// If you have a custom parser that implements the `Parser` interface and you want to include it in the
// loader's configuration process, you can use this function to add it:
//
//	myParser := NewMyCustomParser() // Assume this returns a valid Parser
//	option := WithCustomParser(myParser)
//	loader := NewLoader(config, option)
//
// This will ensure that `myParser` is used by the loader to process configuration data.
func WithCustomParser(p Parser) LoaderOption {
	return func(l *loader) error {
		if p == nil {
			return nil
		}

		l.groups[p.Type()] = p

		return nil
	}
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
//   - A `LoaderOption` that applies the custom parser to the loader's parser group, or returns an error if
//     parser initialization fails.
func WithCustomParserInit(fabric ParserInit) LoaderOption {
	return func(l *loader) error {
		switch parser, err := fabric(l.Config); {
		case err != nil:
			return err
		default:
			l.groups[parser.Type()] = parser

			return nil
		}
	}
}

// WithOptions allows dynamic application of loader options by accepting either a slice of LoaderOption
// or a function that returns a slice of LoaderOption. It ensures flexibility in configuring the loader.
//
// The function performs the following tasks:
//   - Accepts `options` as an argument of type `any`, which can be either a `[]LoaderOption` or
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
//     options type is invalid or if any option fails during application.
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

// setLoaderDefaults initializes a loader with default values based on the provided configuration.
// It sets up the environment variables, command-line arguments, and the order in which parsers
// will be applied, ensuring defaults are in place if not explicitly provided in the Config.
//
// The function performs the following tasks:
//   - If no environment variables are provided in the Config, it defaults to using `os.Environ()`.
//   - If no arguments are provided in the Config, it defaults to `os.Args[1:]`.
//   - If no loader order is defined, it sets a default order: Defaults -> Env -> Flags.
//   - Initializes a map of parsers (`parsers`), based on the Config options such as SkipDefaults,
//     SkipEnv, and SkipFlags, to include or exclude certain parsers.
//
// Parameters:
//   - c: The Config object that contains user-provided settings for environment variables, arguments,
//     and parser control options.
//
// Returns:
// - A pointer to a `loader` struct, which contains the updated Config and the map of available parsers.
func setLoaderDefaults(c Config) *loader {
	if c.Envs == nil {
		c.Envs = os.Environ()
	}

	if c.Args == nil {
		c.Args = os.Args[1:]
	}

	if c.LoaderOrder == nil {
		c.LoaderOrder = []ParserType{ParserDefaults, ParserEnv, ParserFlags}
	}

	parsers := make(map[ParserType]Parser)
	if !c.SkipDefaults {
		parsers[ParserDefaults] = newDefaultParser()
	}

	if !c.SkipEnv {
		parsers[ParserEnv] = newEnvLoader(c.Envs, c.EnvPrefix)
	}

	if !c.SkipFlags {
		parsers[ParserFlags] = newFlagsLoader(c.Args)
	}

	return &loader{Config: c, groups: parsers}
}

// New creates a new Parser based on the provided configuration and optional LoaderOptions.
// The function initializes a loader service (`svc`) with default settings from the provided
// configuration. Then it applies each LoaderOption to customize the service if any are provided.
//
// The function returns a `parserFunc` that, when called, will:
// - Apply all the LoaderOptions to the `svc`.
// - Iterate through the `LoaderOrder` and invoke the corresponding group parsers.
// If any parser fails or if a group parser is missing, the function returns an error.
//
// Parameters:
// - config: The Config object used to initialize the default settings for the loader.
// - options: A variadic number of LoaderOption functions to customize the loader.
//
// Returns:
// - A Parser that can be used to load and parse values into the provided target structure.
func New(config Config, options ...LoaderOption) Parser {
	svc := setLoaderDefaults(config)

	// return group parser
	return &parserFunc{call: func(v interface{}) error {
		for _, option := range options {
			if err := option(svc); err != nil {
				return fmt.Errorf("gonfig: could not init option: %w", err)
			}
		}

		for _, typ := range svc.LoaderOrder {
			if svc.groups[typ] == nil {
				return fmt.Errorf("gonfig: empty parser %s", typ)
			}

			if err := svc.groups[typ].Load(v); err != nil {
				return err
			}
		}

		return ValidateRequiredFields(v)
	}}
}
