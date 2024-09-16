package gonfig

// Parser interface represents an abstraction for loading configuration.
// Implementations of this interface are responsible for loading configuration data
// from various sources into a specified destination object.
type Parser interface {
	// Load loads the configuration into the specified destination object.
	// The parameter dest should be a pointer to the object where the configuration should be loaded.
	// This method should handle the process of parsing and populating the destination object with configuration data.
	// Returns an error if the loading process fails, allowing the caller to handle any issues that occur.
	Load(dest interface{}) error

	// Type returns the type of the current Parser.
	// This method allows you to determine which type of parser is currently being used.
	// It helps in identifying the source or method of configuration loading (e.g., defaults, flags, environment).
	Type() ParserType
}

// ParserUsage interface provides a way to retrieve information about configuration methods
// and options that a parser supports. It is intended to help users understand how to specify
// and use different configuration sources.
type ParserUsage interface {
	// Usage returns information about configuration methods supported by the parser.
	// This includes details on:
	// - command-line flags
	// - environment variables
	// - configuration files
	// - other configuration methods
	// The returned string describes the available configuration options and how they can be used.
	Usage() string
}

// parserFunc is a concrete implementation of the Parser interface.
// It wraps a function that performs the actual loading of configuration data.
// The `name` field stores the type of the parser, and the `call` field holds the function
// responsible for loading the configuration into the destination object.
type parserFunc struct {
	name ParserType
	call func(interface{}) error
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
func (p *parserFunc) Load(dest interface{}) error {
	return p.call(dest)
}

// NewCustomParser creates a new custom parser with the specified name and loader function.
//
// Parameters:
//   - name: A ParserType value representing the name or type of the custom parser.
//   - Loader: A function that takes an interface{} and returns an error. This function
//     defines how the custom parser should load or parse the configuration data.
//
// Returns:
//   - A Parser interface which is implemented by the custom parser. This allows the
//     library to use the provided loader function to process configuration data according
//     to the specified ParserType.
//
// Example usage:
//
//	customParser := NewCustomParser("myCustomParser", func(cfg interface{}) error {
//	    // Custom parsing logic here
//	    return nil
//	})
func NewCustomParser(name ParserType, Loader func(interface{}) error) Parser {
	return &parserFunc{name: name, call: Loader}
}
