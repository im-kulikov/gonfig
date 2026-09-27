package gonfig

// Parser interface represents an abstraction for loading configuration.
// Implementations of this interface are responsible for loading configuration data
// from various sources into a specified destination object.
type Parser interface {
	// Load loads the configuration into the specified destination object.
	// The parameter dest should be a pointer to the object where the configuration should be loaded.
	// This method should handle the process of parsing and populating the destination object with configuration data.
	// Returns an error if the loading process fails, allowing the caller to handle any issues that occur.
	Load(dest any) error

	// Type returns the type of the current Parser.
	// This method allows you to determine which type of parser is currently being used.
	// It helps in identifying the source or method of configuration loading (e.g., defaults, flags, environment).
	Type() ParserType
}

// LoaderValidator is implemented by a config struct, or a struct inside it, that checks
// itself after loading. Nested structs are validated first, the struct passed to Load
// last; the errors of nested structs name the path of their field. An embedded struct
// is not validated on its own: its Validate is promoted to the parent.
type LoaderValidator interface {
	Validate() error
}

// ParserPreparer is an interface for preparing a parser before parsing.
// Implementations of this interface are expected to modify or adjust the provided
// `Config` object before it is used in the parsing process. This can include tasks
// such as setting default values, validating settings, or modifying parser behavior
// based on the given configuration.
//
// This interface is useful when you need a preprocessing step before applying a parser.
//
// Method:
// - Prepare(Config): Accepts a `Config` object and modifies it as needed before parsing.
type ParserPreparer interface {
	Prepare(Config)
}

// ParserConfigSetter defines an interface for setting the configuration file path.
// Implementing types are expected to provide a method to set the path where
// the configuration file for the parser is located.
type ParserConfigSetter interface {
	// SetConfigPath sets the path to the configuration file.
	// The path parameter is expected to be a valid file path as a string.
	SetConfigPath(path string)
}

// strictSetter is implemented by the file loaders: the loader passes Config.Strict
// to them right before loading, as it does with the config path.
type strictSetter interface {
	setStrict(strict bool)
}

// parserFunc is a concrete implementation of the Parser interface.
// It wraps a function that performs the actual loading of configuration data.
// The `name` field stores the type of the parser, and the `call` field holds the function
// responsible for loading the configuration into the destination object.
type parserFunc struct {
	name ParserType
	call func(any) error
}

// ParserInit is a function type that allows initializing a Parser with the provided loader Config.
// It takes a Config object as an argument and returns a Parser along with any initialization error.
// This function is used to create custom parsers based on the configuration settings.
type ParserInit func(c Config) (Parser, error)

// Type returns the type of the current parser.
// It implements the Parser interface and helps identify the kind of parser being used.
func (p *parserFunc) Type() ParserType { return p.name }

// Load invokes the function associated with the parser to load the configuration into the destination object.
// It uses the function provided during parser initialization to perform the actual loading process.
// This method adheres to the Parser interface and provides the mechanism to apply configuration settings to the object.
func (p *parserFunc) Load(dest any) error {
	return p.call(dest)
}

// NewCustomParser creates a Parser of the given type from a function that loads
// the configuration into dest, a pointer to the configuration struct:
//
//	parser := gonfig.NewCustomParser("vault", func(dest any) error {
//		cfg := dest.(*Config)
//		cfg.Password = readFromVault()
//
//		return nil
//	})
//
//nolint:ireturn
func NewCustomParser(name ParserType, loader func(any) error) Parser {
	return &parserFunc{name: name, call: loader}
}
