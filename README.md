![gonfig](.github/logo-x2.png)

[![GitHub Workflow Status](https://github.com/im-kulikov/gonfig/actions/workflows/go.yml/badge.svg)](https://github.com/im-kulikov/gonfig/actions/workflows/go.yml)
[![Coverage](https://codecov.io/gh/im-kulikov/gonfig/branch/main/graph/badge.svg)](https://codecov.io/gh/im-kulikov/gonfig)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-kulikov/gonfig)](https://goreportcard.com/report/github.com/im-kulikov/gonfig)
![Go version](https://img.shields.io/github/go-mod/go-version/im-kulikov/gonfig?style=flat&label=Go%20%3E%3D)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/im-kulikov/gonfig)](https://pkg.go.dev/mod/github.com/im-kulikov/gonfig)

## What is _gonfig_?

**gonfig** is a flexible and extensible configuration library designed to simplify working with application settings. 
It supports loading configurations from environment variables, command-line flags, and various configuration file formats. 
Additionally, it offers an easy way to extend support for new formats. One of its key features is the ability to replace 
or customize components, such as using `spf13/pflag` instead of the standard `flag` package from the Go standard library.

This library simplifies configuration management, making it easy to define, override, and merge settings in your applications.

## Why Use Gonfig?

- **Multiple Sources** – Load configurations from config-files, flags, environment variables, or custom sources.
- **Easy to Use** – Simple API for defining and managing configurations.
- **Extensible** – You can implement custom loaders to fit your needs.
- **Lightweight** – No unnecessary dependencies, optimized for performance.

### General Priority Hierarchy:

The priority described below is considered the default priority and can be modified through configuration settings.

1. **Defaults** — Basic configuration values embedded in the application's code via `default` tags.
2. **Config File** — Values loaded from a configuration file (JSON, YAML, TOML). The path is determined by a flag pre-scan.
3. **Environment Variables** — Values from environment variables override file-based configurations.
4. **Flags** — Command-line flags have the highest priority and override all previous values.

## Installation

```sh
go get github.com/im-kulikov/gonfig
```

**Note:** _gonfig_ uses [Go Modules](https://go.dev/wiki/Modules) to manage dependencies.


### Explain Hierarchy:
1. **Defaults** — Set in the code via struct tags.
2. **Config File** — Detailed parameters provided in a structured file.
3. **Environment Variables** — Overrides file settings for deployment flexibility.
4. **Flags** — Direct overrides at runtime, providing the final word on configuration.

## Current status

- [x] Load defaults
- [x] Load environments
- [x] Load flags
- [x] Mark as required
- [x] Load YAML `WithYAMLLoader`
- [x] Load JSON `WithJSONLoader`
- [x] Load TOML `WithTOMLLoader`
- [x] Other formats, you can write it using a custom loader
- [x] One set of rules for every source: embedded structs, `time.Duration`, `net.IPNet`, `encoding.TextUnmarshaler`
- [x] Strict mode for config files `WithStrict`
- [x] Secrets `secret:"true"`: hidden in `--help` and in exported configs
- [x] Export a config `Write` and the `--print-config` flag `PrintConfigFlag`

## Examples

### Simple

```go
package main

import (
    "fmt"

	"github.com/im-kulikov/gonfig"
)

type Config struct {
	Config string `flag:"config,short:c,config:true"`
	Field  string `flag:"field" env:"FIELD" default:"default-value" usage:"description for flags" require:"true"`
}

// go run /path/to/main/folder --config /path/to/config.yml
func main() {
	var cfg Config
	if err := gonfig.Load(&cfg,
		gonfig.WithYAMLLoader(),
		gonfig.WithDefaults(gonfig.FlagTag, map[string]any{
            "field": "some-custom-default-value",
        })); err != nil {
		panic(err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)
}
```

### Default Configuration Flag

You can use `DefaultConfigFlag` to automatically enable `--config / -c` flags without manual tagging:

```go
type Config struct {
	gonfig.DefaultConfigFlag
	Field string `flag:"field" env:"FIELD" default:"default-value"`
}

// go run main.go --config config.yml
func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithYAMLLoader()); err != nil {
		panic(err)
	}
}
```

### Print the configuration

Embed `PrintConfigFlag` to get `--print-config[=yaml|json|toml|env]`. It prints the loaded configuration
(defaults, file, env and flags) and exits with code 0, like `--help`. Required fields are not checked,
so the output of a fresh setup is a documented template, with a comment above every value:

```go
type Config struct {
	gonfig.DefaultConfigFlag
	gonfig.PrintConfigFlag

	Addr  string `yaml:"addr"  env:"ADDR"  default:":8080" usage:"listen address"`
	Token string `yaml:"token" env:"TOKEN" secret:"true"`
}
```

```console
$ go run . --print-config > config.yaml
$ cat config.yaml
# listen address (env: ADDR, default: :8080)
addr: :8080
# env: TOKEN, secret
token: ""
$ go run . --print-config=env > .env   # for docker --env-file or kubectl create configmap --from-env-file
```

Without a value the format is the one of the file loader (YAML without one), so the output goes straight
back into `--config`. The same is available in code: `gonfig.Write(os.Stdout, &cfg, gonfig.FormatTOML)`,
with `gonfig.WithEnvPrefix(prefix)` when the loader uses `Config.EnvPrefix`.

### Secrets

Mark a field with `secret:"true"`, or a whole struct to cover its fields. A secret is never shown:
`--help` prints no default for its flag and lists its environment variable as `(secret)`, `Write` and
`--print-config` leave it empty. `--help` always shows the `default` tag, not the loaded value.

### Strict mode

By default keys of a config file that match no field are ignored. With `gonfig.WithStrict()` (or
`Config{Strict: true}`) a typo is an error:

```text
gonfig: could not load: could not parse config.yaml: could not decode: 'main.Config' has invalid keys: adress
```

Environment variables are never strict: the environment holds every variable of the process.

### Extended (using New.Load)

```go
package main

import (
    "fmt"

	"github.com/im-kulikov/gonfig"
)

type Config struct {
	Field string `flag:"field" env:"FIELD" default:"default-value" usage:"description for flags" require:"true"`
}

func main() {
	var cfg Config
	if err := gonfig.New(gonfig.Config{}).Load(&cfg); err != nil {
		panic(err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)
}
```

### Use YAML loader

```go
package main

import (
	"fmt"
	
	"github.com/im-kulikov/gonfig"
)

type Config struct {
	Config  string `flag:"config,short:c,config:true"`
	Port    int    `yaml:"port"`
}

// go run /path/to/main/folder --config /path/to/config.yml
func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithYAMLLoader()); err != nil {
		panic(err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)
}
```

### Use TOML loader

```go
package main

import (
	"fmt"
	
	"github.com/im-kulikov/gonfig"
)

type Config struct {
	Config  string `flag:"config,short:c,config:true"`
	Port    int    `toml:"port"`
}

// go run /path/to/main/folder --config /path/to/config.toml
func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithTOMLLoader()); err != nil {
		panic(err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)
}
```

### Use JSON loader

```go
package main

import (
	"fmt"
	
	"github.com/im-kulikov/gonfig"
)

type Config struct {
	Config  string `flag:"config,short:c,config:true"`
	Port    int    `json:"port"`
}

// go run /path/to/main/folder --config /path/to/config.json
func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithJSONLoader()); err != nil {
		panic(err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)
}
```

## Custom Loaders

You can implement your own configuration loaders by implementing the `Parser` interface.

### Nested Configurations

When using nested structs, you can either provide an environment name for each level, or use the `squash` option to expose underlying fields directly.

```go
type Config struct {
    // Exposes ACT_API_ADDRESS
    Activator struct {
        Api struct {
            Address string `env:"ADDRESS"`
        } `env:"API"`
    } `env:"ACT"`

    // Exposes DB_HOST (skips intermediate 'Database' name)
    Database struct {
        Host string `env:"HOST"`
    } `env:",squash"`
}
```

If a nested struct has an empty `env:""` tag and is not squashed, its fields will **not** be reachable via environment variables and will be hidden from the help output.

```go
type CustomLoader struct {}

func (c *CustomLoader) Load(dest interface{}) error {
	// Custom loading logic here
	return nil
}

func (c *CustomLoader) Type() gonfig.ParserType {
	return "custom-loader"
}
```

or with config-path defining

```go
type CustomLoader struct {
	path string
}

func (c *CustomLoader) SetPath(path string) { c.path = path }

func (c *CustomLoader) Load(dest interface{}) error {
	// Custom loading logic here
	// for example - json / toml / etc
	return nil
}

func (c *CustomLoader) Type() gonfig.ParserType {
	return "custom-loader"
}
```

## Custom validation

Introduced in [Issue #3](https://github.com/im-kulikov/gonfig/issues/3)

A new Go interface, `LoaderValidator`. Any struct that implements this interface will have its `Validate()` method called automatically by the loading mechanism.

1.  **Define the interface:**
    ```go
    type LoaderValidator interface {
        Validate() error
    }
    ```

2.  **Integrated into the load process:** We modified the existing configuration loading function(s) to include a check for this interface. The execution flow should be:
    *   Load the configuration (e.g., from YAML/JSON).
    *   Run basic validation (e.g., the existing `ValidateRequiredFields`).
    *   **Check if the loaded config struct implements `LoaderValidator`.**
    *   If it does, call the `config.Validate()` method and return any error it produces.

**A user can now easily add sophisticated validation to their config:**
```go
// User's configuration struct
type AppConfig struct {
    Port     int    `yaml:"port" required:"true"`
    Username string `yaml:"username" required:"true"`
    Email    string `yaml:"email"`
}

// Implement the LoaderValidator interface
func (c *AppConfig) Validate() error {
    // Custom validation 1: Check if port is in the valid range
    if c.Port < 1024 || c.Port > 65535 {
        return fmt.Errorf("port must be between 1024 and 65535, got %d", c.Port)
    }

    // Custom validation 2: Check email format if provided
    if c.Email != "" {
        if !isValidEmail(c.Email) {
            return fmt.Errorf("invalid email format: %s", c.Email)
        }
    }
    return nil
}

// Helper function (user-defined)
func isValidEmail(email string) bool {
    // ... simple regex check for example purposes
    re := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
    return re.MatchString(email)
}
```

With this implementation, when the user loads their `AppConfig`, the system will automatically check that `Port` 
and `Username` are provided (basic validation) and *then* run the custom checks to ensure the port number is acceptable 
and the email format is valid.

*   **Backward Compatible:** This is a purely additive change. Existing code without the `Validate()` method will continue to work unchanged.
*   **Clean and Idiomatic:** It follows Go's common pattern of using interfaces for extensibility (e.g., `Stringer`, `Error`).
*   **Powerful and Flexible:** Users are no longer limited to just `required` checks and can implement any validation logic their application requires (cross-field validation, business logic, formatting, etc.).
*   **Centralized Validation:** The validation logic lives alongside the data structure it validates, making the code more organized and maintainable.

## Upgrading to v0.7

- Go 1.26 or newer is required; the two latest Go releases are supported.
- Config files are decoded by the same rules as environment variables. An embedded struct no longer
  needs `yaml:",inline"`; JSON and TOML accept `"3s"` for `time.Duration` and `"10.0.0.0/8"` for
  `net.IPNet`; `encoding.TextUnmarshaler` types (`slog.Level`, `time.Time`) work in every source;
  file keys match field tags case-insensitively; an empty file is not an error.
- Custom `yaml.Unmarshaler`, `json.Unmarshaler` and TOML unmarshalers of field types are no longer
  called; implement `encoding.TextUnmarshaler` instead.
- `--help` shows the `default` tag of a flag instead of its loaded value.
- Errors are one line and name the file: `could not parse config.yaml: ...`.

