# nfigure - per-library configuration

[![GoDoc](https://godoc.org/github.com/muir/nfigure?status.png)](https://pkg.go.dev/github.com/muir/nfigure)
![unit tests](https://github.com/muir/nfigure/actions/workflows/go.yml/badge.svg)
[![report card](https://goreportcard.com/badge/github.com/muir/nfigure)](https://goreportcard.com/report/github.com/muir/nfigure)
[![codecov](https://codecov.io/gh/muir/nfigure/branch/main/graph/badge.svg)](https://codecov.io/gh/muir/nfigure)

Install:

	go get github.com/muir/nfigure

---

Nfigure is a reflective configuration library.  It supports:

- Describing what to configure using struct tags
- Configuration from: multiple configuration file formats, and multiple files
- Configuration from: environment variables
- Configuration from: the command line
- Posix-style and Go-style command line parsing
- Support for subcommands when parsing command lines
- Multi-stage binding to allow independently-developed libraries to express their configruration needs ahead of program startup
- Custom type support using [reflectutils.RegisterStringSetter()](https://pkg.go.dev/github.com/muir/reflectutils#RegisterStringSetter).
- Filling all normal Go types, array, slices, maps, and time.Duration
- Ability to export flag-based configuration requests to the "flag" module (useful for libraries)

While nfigure has lots of flexibility and many features, using it should be simple.

## Example: In a library, in-house or published for others to use

It can pre-register configuration at the library level, before program startup.  This allows
library-specific configuration to be handled at the library-level rather than pushed to 
a central main.

```go
type myLibraryConfig struct {
	Field1	string	  `env="FIELD1" flag:"field1" default:"f1" help:"Field1 controls the first field"`
	Field2	int	  `config:"mylibrary.field2"` 
	Field3	[]string  `flag:"field3"`
}

type MyLibrary btruct {
	config	myLibraryConfig
}

func createMyLibrary(nreg *nfigure.Registry) *MyLibrary {
	lib := MyLibrary{}
	nreg.Register(&lib.config,
		nfigure.Prefix("myLibrary"),
		nfigure.ConfigFileFrom(`env="MYLIBRARY_CONFIG_FILE" flag:"mylibrary-config"`),
	)
	return &lib
}

## Example: At the program level

This is an example using [nserve](https://github.com/muir/nject/tree/main/nserve).
Where this gets interesting is if multiple
binaries are built from the same source, the set of libraires can exist in a
list and only the ones that are needed for particular executables will have their
configuration evaluated.

```go
type programLevelConfig struct {
	Field1	string `env="field1" default:"F1"`
	Field4	float64	`flag:"field4" default:"3.9"`
}

func createMyApp(myLibrary *mylibrary.MyLibrary, nreg *nfigure.Registery) error {
	// start servers, do not return until time to shut down
	var config programLevelConfig
	nreg.Register(&config, "main")
	_ = nreg.Configure()
}

func main() {
	app, _ := nserve.CreateApp("myApp", 
		nfigure.NewRegistryFactory(),
		createMyLibrary, 
		createMyApp)
	_ = app.Do(nserve.Start)
	_ = app.Do(nserve.Stop)
}
```

## Supported tags

Assuming a command line parser was bound, the follwing tags are supported:

- `nfigure`: the meta tag, used to control filler interactions
- `default`: a filler that provides a literal value
- `env`: fill values from environment variables
- `config`: fill values from configuration files
- `flag`: fill values from the command line (Go style or Posix style)
- `help`: per-item help text for command line Usage

## Environment variables

Usage:

- `env:"VARNAME"` specifies that a value can or should be loaded from an environment variable

## Command line parsing

Both Go-style and Posix-style command line parsing is supported.  In addition to the
base features, counters are supported.  Filling maps, arrays, and slices is supported.

- `flag:"name"` specifies the name of the command line flag to fill a value.
- `flag:"name n"` specifies a single letter alternative
- `flag:"name,split=comma` for array values, specifies that strings will be split on comma, flag can only be given once
- `flag:"name,explode=true` for array values, specifies that the flag can be given multiple times and is not split
- `flag:"name,counter` for numberic values, counts the number of times the flag is used, flag cannot take argument
- `flag:"name,map=explode,split=equal` for maps, support -name a=b -name b=c
- `flag:"name,map=prefix` for maps, support --namex=a --nameb=c

### Posix-style

When using Posix-style flags (`PosixFlagHandler()`), flags whose names are only a single rune
can be combined on the command line:

	--debug 
	-d
	--verbose
	-v
	-dv (debug and verbose)

For boolean values, negation is "--no-":

	--no-verbose

## Best Practices

### Best Practices for existing libraries

Libraries that are already published and using the standard "flag" package
can be refactored to use nfigure.  If they register themselves with flag during
init, then that behavior should be retained:

```go
package mylibrary 

import (
	"flag"
	"github.com/muir/nfigure"
)

type MyConfig struct {
	MyField string `flag:"myfield"`
}

sub init() {
	err := nfigure.ExportToFlagSet(flag.CommandLine, "flag", &MyConfig)
	if err != nil {
		panic(err.Error())
	}
}
```

In a program that is using nfigure, MyConfig can be explicitly imported:

```go
registery.Request(&mylibrary.MyConfig)
```

However, if there are other libraries that only support "flag" and they're being
imported:

```go
GoFlagHandler(nfigure.ImportFlagSet(flag.CommandLine))
```

Then MyConfig should not also be explicity imported since that would end up
with the flags being defined twice.

### Best practices for new libraries

New libraries should use nfigure to handle their configruation needs.  The suggested
way to do this is to have a New function that takes a registry as arguments.

Separate New() and Start() so that configuation can happen after New() but before Start().

Users of your library can use NewWithRegistry() if they're using nfigure.  For other
users, they can fill MyConfig by hand or use FillConfigFromCommandline() to populate it
with command line parsing.

```go
func NewWithRegistry(registry *nfigure.Registry) mySelf {
	var config MyConfig
	registry.Request(&config)
	...
}

func FillConfigFromCommandline() *MyConfig {
	var config MyConfig
	registery.Request(&config)
	return &config
}

func NewWithConfig(config MyConfig) mySelf {
	...
}
```

### Best practices for program writers

Use nfigure everywhere!  Be careful not to combine ImportFlagSet with
registry.Request() of the same models that are ExportToFlagSet()ed 
in library inits.

Separate library creation from library starting.  Allow configuration
to be deferred until until library start.

