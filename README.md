![gonfig](.github/logo-x2.png)

[![GitHub Workflow Status](https://github.com/im-kulikov/gonfig/actions/workflows/go.yml/badge.svg)](https://github.com/im-kulikov/gonfig/actions/workflows/go.yml)
[![Coverage](https://codecov.io/gh/im-kulikov/gonfig/branch/main/graph/badge.svg)](https://codecov.io/gh/im-kulikov/gonfig)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-kulikov/gonfig)](https://goreportcard.com/report/github.com/im-kulikov/gonfig)
![Go version](https://img.shields.io/github/go-mod/go-version/im-kulikov/gonfig?style=flat&label=Go%20%3E%3D)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/im-kulikov/gonfig)](https://pkg.go.dev/mod/github.com/im-kulikov/gonfig)

**gonfig** loads a Go struct from defaults, config files (YAML, JSON, TOML), environment variables and
command-line flags, with one set of rules for every source. Struct tags are the whole setup: no key
registration, no global state.

- one struct, four sources, a fixed priority;
- `--help` lists every flag and environment variable, `--print-config` writes a documented template;
- secrets are hidden from help and exports, strict mode catches typos in config files;
- other sources plug in through a small `Parser` interface.

Every feature has a runnable, tested example in [example_test.go](example_test.go), also on
[pkg.go.dev](https://pkg.go.dev/github.com/im-kulikov/gonfig#pkg-examples).

## Installation

```sh
go get github.com/im-kulikov/gonfig
```

Go 1.26 or newer; the two latest Go releases are supported.

## Quick start

```go
package main

import (
	"log"
	"time"

	"github.com/im-kulikov/gonfig"
)

type Config struct {
	gonfig.DefaultConfigFlag // --config, -c
	gonfig.PrintConfigFlag   // --print-config

	Host     string        `yaml:"host"     env:"HOST"     flag:"host" default:"localhost" usage:"server host"`
	Port     int           `yaml:"port"     env:"PORT"     flag:"port" default:"8080"      usage:"server port"`
	Timeout  time.Duration `yaml:"timeout"  env:"TIMEOUT"  default:"5s"`
	Password string        `yaml:"password" env:"PASSWORD" required:"true" secret:"true"`
}

func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithYAMLLoader()); err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on %s:%d", cfg.Host, cfg.Port)
}
```

```console
$ PASSWORD=secret ./app --config config.yaml --port 9090
$ ./app --help                        # every flag and environment variable
$ ./app --print-config > config.yaml  # a documented template of the whole config
```

## Sources and priority

Every source overrides the previous ones:

1. **Defaults** — `default` tags, and `WithDefaults`, which wins over the tags.
2. **Config file** — the path is pre-scanned from the flags: `--config` / `-c` with `DefaultConfigFlag`,
   or the string field tagged `flag:"...,config:true"`; without its flag that field takes the path from its
   variable (`env:"CONFIG"`), else from its own value (`default:"/etc/app.yaml"`, `WithDefaults`, the code).
   Custom parsers run at this step too.
3. **Environment variables.**
4. **Flags.**

The order is fixed; `Config.SkipDefaults`, `SkipEnv` and `SkipFlags` turn a source off (`SkipFlags` also
turns off the config path). After loading, `required` fields are checked and `Validate()` is called.

## Struct tags

| Tag | Example | Meaning |
|---|---|---|
| `default` | `default:"8080"`, `default:"a,b"`, `default:"k:1,j:2"` | Value of a field that is still empty; lists and maps are comma-separated |
| `env` | `env:"PORT"` | Environment variable; names of nested structs are joined with `_` |
| | `env:",squash"` | Inline the variables of a nested struct, without its prefix |
| | `env:"-"` | Never read from the environment |
| `flag` | `flag:"port,short:p"` | Command-line flag and its one-letter shorthand; `flag:"-"` ignores the field |
| | `flag:"key,base:hex"` | Encoding of a `[]byte` flag: `hex` or `b64` |
| | `flag:"config,short:c,config:true"` | The string field is the path of the config file |
| | `flag:",args"` | The `[]string` field gets the positional arguments |
| `yaml`, `json`, `toml` | `yaml:"port"`, `yaml:",inline"` | Key in a config file; `,inline` inlines a nested struct (or a pointer to one), or gives a map the keys no field has; `-` ignores the field |
| `usage` | `usage:"server port"` | Description in `--help` and in exported configs |
| `required` | `required:"true"` | Loading fails if the field is still its zero value |
| `secret` | `secret:"true"` | Never shown in `--help` or exported configs; on a struct, covers its fields |

## Environment variables

A variable name is the `env` tags from the top struct down, joined with `_`. A nested struct without an
`env` tag hides its fields from the environment and from `--help`; an embedded struct or one tagged
`env:",squash"` adds no prefix.

```go
type Config struct {
	Database struct {
		Host string `env:"HOST"` // DB_HOST
	} `env:"DB"`

	Server struct {
		Port int `env:"PORT"` // PORT
	} `env:",squash"`
}
```

- `Config.EnvPrefix = "APP"` (or `"APP_"`) reads `APP_DB_HOST` and ignores the rest of the environment.
- Lists are comma-separated: `PORTS=80,443`. Map entries are variables of their own:
  `LABELS_team=core` for `Labels map[string]string` tagged `env:"LABELS"`; the rest of the name is the key,
  so `LABELS_team_name` is `team_name`. In a map of structs the next segment is the key and the rest names a
  field: `DBS_main_HOST` for `DBs map[string]DB` tagged `env:"DBS"` (its keys cannot hold `_`).
- Durations (`5s`), IPs, CIDR networks and every `encoding.TextUnmarshaler` (`slog.Level`, `time.Time`)
  are parsed from text.
- Names that belong to something else are ignored: `DB=...` next to the struct `DB` (`DB_HOST`), or
  `NAME_SUFFIX` next to the field `NAME`.

## Config files

`WithYAMLLoader()`, `WithJSONLoader()` or `WithTOMLLoader()` read the file at the config path; without a
path nothing is read. With several of them the file goes to the loader of its extension (`.yaml`, `.yml`,
`.json`, `.toml`), or to the first one added for any other name, so `--config app.conf` works too.

```go
type Config struct {
	gonfig.DefaultConfigFlag // --config config.yaml

	Server struct {
		Addr    string        `yaml:"addr"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"server"`
}

err := gonfig.Load(&cfg, gonfig.WithYAMLLoader())
```

```yaml
server:
  addr: :9000
  timeout: 3s
```

Files follow the rules of the environment: keys come from the format's tag (without one, from the field
name, case-insensitive), embedded structs and `,inline` fields are inlined, and values are converted the
same way, so `"3s"` is a duration in YAML, JSON and TOML alike. An empty file, or one with comments only,
changes nothing.

`FromFS(fsys)` makes a loader read the config path from an `fs.FS`: an `embed.FS` with a config built
into the binary, or a `fstest.MapFS` in tests. The path is then a path in `fsys` (`configs/app.yaml`):

```go
//go:embed configs
var configs embed.FS

err := gonfig.Load(&cfg, gonfig.WithYAMLLoader(gonfig.FromFS(configs)))
```

By default keys of a config file that match no field are ignored. With `gonfig.WithStrict()` (or
`Config{Strict: true}`) such a key, usually a typo, is an error:

```text
gonfig: could not load: could not parse config.yaml: could not decode: 'main.Config' has invalid keys: listen
```

## Flags

A field with a `flag` tag gets a flag. pflag's own flags cover `bool`, `string`, every size of `int`,
`uint` and `float`, `time.Duration`, `net.IP`, `net.IPNet`, `net.IPMask`, `[]byte` (with `base:hex` or
`base:b64`), slices of `bool`, `string`, `int`, `int32`, `int64`, `uint`, `float32`, `float64`, `net.IP`,
`time.Duration`, and `map[string]string`, `map[string]int`, `map[string]int64` (`--labels=team=core,tier=1`).
Any other type the `default` tag parses gets a flag parsed the same way: named types (`type Port int`),
`encoding.TextUnmarshaler` (`slog.Level`, `time.Time`), pointers, other lists and maps
(`--weights=a:true,b:false`). A `func`, `chan` or interface field with a `flag` tag is an error, as is
the same flag on two fields.

`--help` prints every flag with the default from its `default` tag, never a value loaded from a file or
the environment, then every environment variable (unless `SkipEnv`), and exits with code 0. `WithCustomOutput` and
`WithCustomExit` redirect the output and replace `os.Exit`; `Load` then returns `ErrTestExit`.

Positional arguments, the ones left after the flags and all after `--`, go to a `[]string` field tagged
`flag:",args"`; with `required:"true"` at least one is needed. They are not configuration, so
`--print-config` leaves them out. The path of the config file is in your own field, tagged
`flag:"config,short:c,config:true"`, instead of `DefaultConfigFlag`:

```go
type Config struct {
	Path  string   `flag:"config,short:c,config:true"` // ./app -c app.yaml a.txt b.txt
	Files []string `flag:",args" required:"true"`      // [a.txt b.txt]
}
```

Flags of `go test` (`-test.*`) are ignored. In tests set `Args` and `Envs` through `WithConfig`, so the
result does not depend on how the tests are run: a runner that passes `-test.run ^TestX$` as two
arguments leaves the pattern as a positional argument. `Args: nil` means `os.Args`, use `[]string{}`.

## Defaults

The `default` tag fills a field that no source has set: numbers (with Go prefixes, as env and flags read them:
`0644` is octal, `0x10`, `0b101`, `1_000`), strings, booleans, `time.Duration`,
`net.IPNet` (`10.0.0.0/8`), `net.IPMask` (a prefix, `/24`: IPv4 up to 32, IPv6 up to 128), any
`encoding.TextUnmarshaler`, pointers to these, and lists (`1s,2m`) and maps (`read:1s,write:2s`) of them.
The first `:` splits a map entry, so a value may contain it: `api:http://localhost:8080`. List items and
map entries cannot contain `,`.

`WithDefaults(tag, values)` sets defaults at run time from a map keyed by the given tag, with nested maps
for nested structs. They win over `default` tags and lose to files, environment and flags:

```go
gonfig.WithDefaults("yaml", map[string]any{"server": map[string]any{"addr": ":9000"}})
```

## Sections

A pointer to a struct is a section: `nil` until the code or a source sets it, so a nil section means
"not configured".

```go
type Config struct {
	TLS *struct {
		Cert       string `yaml:"cert"        env:"CERT"        required:"true"`
		MinVersion string `yaml:"min_version" env:"MIN_VERSION" default:"TLS13"`
	} `yaml:"tls" env:"TLS"`
}
```

- A section a file or the environment creates starts from its `default` tags, like the root of the
  config (unless `SkipDefaults`): `TLS_CERT=cert.pem` gives `{Cert: cert.pem, MinVersion: TLS13}`.
  `tls: null` keeps it nil, and so does a variable that sets none of its fields (`TLS_OTHER`).
- In a section that is set, `required` fields are checked (`TLS.Cert`) and `Validate()` is called.
- `--help` lists the variables of every section (`TLS_CERT`, `TLS_MIN_VERSION`).
- Flags are not registered inside sections: a section may not exist before the flags are parsed.

## Secrets

Mark a field with `secret:"true"`, or a whole struct to cover its fields. A secret is never shown:
`--help` prints no default for its flag and lists its environment variable as `(secret)`, `Write` and
`--print-config` leave it empty.

## Print the configuration

Embed `PrintConfigFlag` to get `--print-config[=yaml|json|toml|env]`. It prints the loaded configuration
(defaults, file, env and flags) and exits with code 0, like `--help`. Required fields are not checked,
so the output of a fresh setup is a documented template, with a comment above every value:

```console
$ ./app --print-config
# server host (env: HOST, default: localhost)
host: localhost
# server port (env: PORT, default: 8080)
port: 8080
# env: TIMEOUT, default: 5s
timeout: 5s
# env: PASSWORD, secret
password: ""
$ ./app --print-config=env > .env   # for docker --env-file or kubectl create configmap --from-env-file
```

Without a value the format is the one of the file loader (YAML without one), so the output goes straight
back into `--config`. The same is available in code: `gonfig.Write(os.Stdout, &cfg, gonfig.FormatTOML)`,
with `gonfig.WithEnvPrefix(prefix)` when the loader uses `Config.EnvPrefix`.

## Validation

`required:"true"` fails the loading when the field is still its zero value after every source, so `0` and
`false` cannot be required values. The error lists every missing field with its path. Then `Validate()`
is called for every struct that implements `LoaderValidator`: nested structs first, the struct passed to
`Load` last; errors of nested structs name their field (`Server: port 80 is privileged`). A `Validate` a
struct gets from an embedded pointer that is nil is not called: that part is not set.

```go
func (c *Config) Validate() error {
	if c.Port < 1024 {
		return fmt.Errorf("port %d is privileged", c.Port)
	}

	return nil
}
```

## Custom parsers

You can implement your own configuration loaders by implementing the `Parser` interface and registering it with `WithCustomParser`:

```go
type CustomLoader struct{}

func (c *CustomLoader) Load(dest any) error {
	// Custom loading logic here
	return nil
}

func (c *CustomLoader) Type() gonfig.ParserType {
	return "custom-loader"
}

func main() {
	var cfg Config
	if err := gonfig.Load(&cfg, gonfig.WithCustomParser(&CustomLoader{})); err != nil {
		panic(err)
	}
}
```

Custom loaders run after defaults and before environment variables and flags, in the order they were added.
A loader with the type of a built-in one (`gonfig.ParserEnv`, `gonfig.ParserFlags`, `gonfig.ParserDefaults`) replaces it.

To read a file, implement `ParserConfigSetter` as well: the loader passes it the path from `--config`
(with `DefaultConfigFlag`) or from the field tagged `flag:"...,config:true"`, right before `Load`:

```go
type CustomLoader struct {
	path string
}

func (c *CustomLoader) SetConfigPath(path string) { c.path = path }

func (c *CustomLoader) Load(dest any) error {
	if c.path == "" {
		return nil // no --config given
	}

	// read c.path and decode it into dest
	return nil
}

func (c *CustomLoader) Type() gonfig.ParserType {
	return "custom-loader"
}
```

`WithCustomParser` shares one parser between all loads of a `Parser`, so the path of one `Load` would
overwrite the path of another running at the same time. A parser that keeps state is made for every `Load`
with `WithCustomParserInit`:

```go
parser := gonfig.New(gonfig.Config{}, gonfig.WithCustomParserInit(func(gonfig.Config) (gonfig.Parser, error) {
	return &CustomLoader{}, nil
}))
```

## Options

| Option | Effect |
|---|---|
| `WithYAMLLoader()`, `WithJSONLoader()`, `WithTOMLLoader()` | Read the config file in this format; `FromFS(fsys)` reads it from an `fs.FS` |
| `WithStrict()` | Keys of a config file that match no field are an error |
| `WithConfig(func(*Config))` | Change `EnvPrefix`, `Skip*`, `Strict`, `Args`, `Envs` |
| `WithDefaults(tag, map)` | Defaults from a map |
| `WithCustomParser(p)`, `WithCustomParserInit(f)` | Add a parser or replace a built-in one |
| `WithOptions(options)` | Apply a list of options, or a function returning one |
| `WithCustomOutput(w)`, `WithCustomExit(f)` | Output of `--help` / `--print-config` and the exit function |

`gonfig.Load(&cfg, options...)` is `gonfig.New(gonfig.Config{}, options...).Load(&cfg)`. A `Parser` made
by `New` can be reused, also from several goroutines, as long as its custom parsers keep no state of a
`Load` or are made by `WithCustomParserInit`.

## Limitations

- `default` tags: list items and map entries cannot contain `,`, map keys cannot contain `:`.
- `default` tags fill a struct a source creates behind a pointer (a section, an item of `[]*Item`), not an item
  of `[]Item` or `map[string]Item`: give such items their values in the source.
- No flags inside sections (pointers to structs), see [Sections](#sections).
- Embed structs by value: an embedded pointer to a struct is read only when it is set.
- `flag:",base:hex"` and `base:b64` apply to the flag; env and files read a `[]byte` as a list of numbers.
- The config path is pre-scanned knowing only the config flag: in `--name -c app.yaml`, where `-c` is the
  value of `--name`, it still reads `app.yaml`. Write `--name=-c`.
- Env: keys of a map of structs cannot contain `_` (`DBS_main_HOST`), keys of a map of values can.

## Upgrading to v0.7

- Go 1.26 or newer is required; the two latest Go releases are supported.
- `Ptr`, `True` and `False` are removed: use `new(v)`, for example `new(true)`.
- Config files are decoded by the same rules as environment variables. An embedded struct no longer
  needs `yaml:",inline"`; JSON and TOML accept `"3s"` for `time.Duration` and `"10.0.0.0/8"` for
  `net.IPNet`; `encoding.TextUnmarshaler` types (`slog.Level`, `time.Time`) work in every source;
  file keys match field tags case-insensitively; an empty file is not an error.
- Custom `yaml.Unmarshaler`, `json.Unmarshaler` and TOML unmarshalers of field types are no longer
  called; implement `encoding.TextUnmarshaler` instead.
- `--help` shows the `default` tag of a flag instead of its loaded value.
- Errors are one line and name the file: `could not parse config.yaml: ...`.

