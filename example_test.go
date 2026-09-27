package gonfig_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/im-kulikov/gonfig"
)

// source sets what the loader reads instead of os.Args and os.Environ,
// so the examples do not depend on how they are run.
func source(args []string, envs ...string) gonfig.LoaderOption {
	return gonfig.WithConfig(func(c *gonfig.Config) { c.Args, c.Envs = args, envs })
}

// configFile writes a config file for an example and returns its path.
func configFile(name, content string) string {
	dir, err := os.MkdirTemp("", "gonfig-example")
	if err != nil {
		panic(err)
	}

	path := filepath.Join(dir, name)
	if err = os.WriteFile(path, []byte(content), 0o600); err != nil {
		panic(err)
	}

	return path
}

// Every source overrides the previous one: defaults, environment variables, flags.
func Example() {
	type Config struct {
		Host string `flag:"host" env:"HOST" default:"localhost" usage:"server host"`
		Port int    `flag:"port" env:"PORT" default:"8080"      usage:"server port"`
		Name string `flag:"name" env:"NAME" required:"true"`
	}

	var cfg Config
	err := gonfig.Load(&cfg, source([]string{"--name", "gonfig"}, "PORT=9090", "NAME=from-env"))

	fmt.Printf("%+v, err: %v\n", cfg, err)
	// Output: {Host:localhost Port:9090 Name:gonfig}, err: <nil>
}

func ExampleDefaultConfigFlag() {
	type Config struct {
		gonfig.DefaultConfigFlag // enables --config and -c

		Server struct {
			Addr    string        `yaml:"addr"    env:"ADDR"    default:":8080"`
			Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" default:"5s"`
		} `yaml:"server" env:"SERVER"`
	}

	path := configFile("config.yaml", "server:\n  addr: :9000\n  timeout: 3s\n")

	var cfg Config
	err := gonfig.Load(&cfg, gonfig.WithYAMLLoader(), source([]string{"--config", path}, "SERVER_TIMEOUT=10s"))

	fmt.Printf("%+v, err: %v\n", cfg.Server, err)
	// Output: {Addr::9000 Timeout:10s}, err: <nil>
}

func ExampleWithJSONLoader() {
	type Config struct {
		gonfig.DefaultConfigFlag

		Port     int           `json:"port"`
		Interval time.Duration `json:"interval"`
	}

	path := configFile("config.json", `{"port": 9090, "interval": "30s"}`)

	var cfg Config
	err := gonfig.Load(&cfg, gonfig.WithJSONLoader(), source([]string{"--config", path}))

	fmt.Printf("%+v, err: %v\n", cfg, err)
	// Output: {DefaultConfigFlag:{} Port:9090 Interval:30s}, err: <nil>
}

func ExampleWithTOMLLoader() {
	type Config struct {
		Path string `flag:"config,short:c,config:true"` // the config path in a field of your own

		Port int `toml:"port"`
	}

	path := configFile("config.toml", "port = 9090\n")

	var cfg Config
	err := gonfig.Load(&cfg, gonfig.WithTOMLLoader(), source([]string{"-c", path}))

	fmt.Println(cfg.Port, err)
	// Output: 9090 <nil>
}

func ExampleWithStrict() {
	type Config struct {
		gonfig.DefaultConfigFlag

		Address string `yaml:"address"`
	}

	path := configFile("config.yaml", "listen: :8080\n") // the field is address

	var cfg Config
	err := gonfig.Load(&cfg, gonfig.WithYAMLLoader(), gonfig.WithStrict(), source([]string{"--config", path}))

	_, reason, _ := strings.Cut(err.Error(), "could not decode: ") // after the path of the file

	fmt.Println(errors.Is(err, gonfig.ErrCantParse))
	fmt.Println(reason)
	// Output:
	// true
	// 'gonfig_test.Config' has invalid keys: listen
}

func ExamplePrintConfigFlag() {
	type Config struct {
		gonfig.PrintConfigFlag // enables --print-config[=yaml|json|toml|env]

		Addr  string `yaml:"addr"  env:"ADDR"  default:":8080" usage:"listen address"`
		Token string `yaml:"token" env:"TOKEN" secret:"true"`
	}

	var cfg Config
	err := gonfig.Load(&cfg,
		gonfig.WithCustomExit(func(int) {}), // os.Exit(0) outside of the example
		source([]string{"--print-config"}, "TOKEN=hunter2"))

	fmt.Println(errors.Is(err, gonfig.ErrTestExit))
	// Output:
	// # listen address (env: ADDR, default: :8080)
	// addr: :8080
	// # env: TOKEN, secret
	// token: ""
	// true
}

func ExampleWrite() {
	type Config struct {
		Level    string        `env:"LEVEL"    default:"info" usage:"log level"`
		Timeouts []string      `env:"TIMEOUTS"`
		Interval time.Duration `env:"INTERVAL"`
		Password string        `env:"PASSWORD" secret:"true"`
	}

	cfg := Config{Level: "debug", Timeouts: []string{"1s", "2s"}, Interval: time.Minute, Password: "hunter2"}
	if err := gonfig.Write(os.Stdout, &cfg, gonfig.FormatEnv, gonfig.WithEnvPrefix("APP")); err != nil {
		panic(err)
	}

	// Output:
	// # log level (default: info)
	// APP_LEVEL=debug
	// APP_TIMEOUTS=1s,2s
	// APP_INTERVAL=1m0s
	// # secret
	// APP_PASSWORD=
}

// Nested structs add their `env` name as a prefix; squash and embedding do not.
func ExampleUsageOfEnvs() {
	type Server struct {
		Port int `env:"PORT" default:"8080"`
	}

	type Config struct {
		Activator struct {
			API struct {
				Address string `env:"ADDRESS"`
			} `env:"API"`
		} `env:"ACT"`

		Database struct {
			Host string `env:"HOST" usage:"database host"`
		} `env:"DB"`

		Server `env:",squash"`
	}

	fmt.Println(gonfig.UsageOfEnvs(&Config{}, gonfig.EnvUsageWithPrefix("APP")))
	// Output:
	// Environment variables:
	//   - 'APP_ACT_API_ADDRESS' <string>
	//   - 'APP_DB_HOST' <string> — database host
	//   - 'APP_PORT' <int> (default: 8080)
}

// vaultLoader is a custom parser that also receives the path from --config.
type vaultLoader struct{ path string }

func (v *vaultLoader) Type() gonfig.ParserType   { return "vault" }
func (v *vaultLoader) SetConfigPath(path string) { v.path = path }

func (v *vaultLoader) Load(dest any) error {
	if v.path != "" {
		dest.(*vaultConfig).Token = "token from " + v.path
	}

	return nil
}

type vaultConfig struct {
	gonfig.DefaultConfigFlag

	Token string `env:"TOKEN"`
}

func ExampleWithCustomParser() {
	var cfg vaultConfig
	err := gonfig.Load(&cfg, gonfig.WithCustomParser(&vaultLoader{}), source([]string{"-c", "secret/app"}))

	fmt.Println(cfg.Token, err)
	// Output: token from secret/app <nil>
}

type portConfig struct {
	Port int `env:"PORT" default:"80"`
}

func (c *portConfig) Validate() error {
	if c.Port < 1024 {
		return fmt.Errorf("port %d is privileged", c.Port)
	}

	return nil
}

// A config that implements LoaderValidator is validated after loading.
func ExampleLoaderValidator() {
	var cfg portConfig
	fmt.Println(gonfig.Load(&cfg, source(nil)))
	fmt.Println(gonfig.Load(&cfg, source(nil, "PORT=8080")))
	// Output:
	// port 80 is privileged
	// <nil>
}
