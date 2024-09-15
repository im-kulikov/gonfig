![gonfig](.github/logo-x2.png)

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/im-kulikov/gonfig/ci.yaml?branch=master&style=flat-square)](https://github.com/im-kulikov/gonfig/actions?query=workflow%3ACI)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-kulikov/gonfig?style=flat-square)](https://goreportcard.com/report/github.com/im-kulikov/gonfig)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.21-61CFDD.svg?style=flat-square)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/im-kulikov/gonfig)](https://pkg.go.dev/mod/github.com/im-kulikov/gonfig)

## Install

```shell
go get github.com/im-kulikov/gonfig
```

**Note:** _gonfig_ uses [Go Modules](https://go.dev/wiki/Modules) to manage dependencies.

## What is _gonfig_?

**gonfig** is a flexible and extensible configuration library designed to simplify working with application settings. 
It supports loading configurations from environment variables, command-line flags, and various configuration file formats. 
Additionally, it offers an easy way to extend support for new formats. One of its key features is the ability to replace 
or customize components, such as using `spf13/pflag` instead of the standard `flag` package from the Go standard library.

### General Priority Hierarchy:

The priority described below is considered the default priority and can be modified through configuration settings.

1. **Defaults** — These are basic configuration values embedded in the application's code. They ensure the application can run even if no external configurations are provided.

2. **Environment Variables** — Environment variables are usually used to configure deployment-related parameters (e.g., logins, ports, database addresses). These variables often have a higher priority as they can be dynamically set depending on the environment.

3. **Flags** — Command-line flags usually have the highest priority since they allow direct overriding of any settings at application startup. This is useful when a quick configuration change is needed without modifying the code or config files.

4. **Config File** — A configuration file stored on disk, typically containing predefined parameters for a specific environment. This can be in formats like JSON, YAML, TOML, etc.

5. **Remote Config** — This is a configuration retrieved from external sources, such as configuration servers or cloud services (e.g., Consul, Etcd, or AWS SSM). These systems usually allow centralized management of settings across different applications.


### Explain Hierarchy:
1. **Defaults** — Set in the code. For example, the default server port is `8080`.
2. **Environment Variables** — Environment variables can be used to set database connections or other services linked to the environment.
3. **Flags** — Command-line arguments always have the highest priority, as they can be specified at application startup to override any other parameter.
4. **Config File** — The configuration file specifies more detailed parameters, such as database connections or the application's operating mode.
5. **Remote Config** — Configuration retrieved from a remote server can override settings from the config file.


## Current status

- [x] Load defaults
- [x] Load environments
- [x] Load flags
- [x] Mark as required
- [ ] Load YAML (you can use custom loader)
- [ ] Load JSON (you can use custom loader)
- [ ] Load TOML (you can use custom loader)
- [x] Other formats, you can write it using custom loader

## Examples

```go
package main

import (
	"github.com/davecgh/go-spew/spew"
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

	spew.Dump(cfg)
}
```