# nfigure - per-library configuration

[![GoDoc](https://godoc.org/github.com/muir/nfigure?status.png)](https://pkg.go.dev/github.com/muir/nfigure)
![unit tests](https://github.com/muir/nfigure/actions/workflows/go.yml/badge.svg)
[![report card](https://goreportcard.com/badge/github.com/muir/nfigure)](https://goreportcard.com/report/github.com/muir/nfigure)

Install:

	go get github.com/muir/nfigure

---

Nfigure is a configuration library with the property that the set of things
to be configured can be assembled during startup.  This allows independently-developed
libraries to express their configuration needs.

Configuration request details are expressed with struct tags.

Nfigure natively supports configuration via files, environment variables, and command line flags.

## In a library, in-house or published for others to use

```go
type myLibraryConfig struct {
	Field1	string	  `env="FIELD1" flag:"field1" default:"f1" describe:"Field1 controls the first field"`
	Field2	int	  `nfigure:mylibrary.field2` 
	Field3	[]string  `flag:field3`
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

## At the program level

This is an example using nserve.  Where this gets interesting is if multiple
binaries are built from the same source, the set of libraires can exist in a
list and only the ones that are needed for particular executables will have their
configuration evaluated.

```
type programLevelConfig struct {
	Field1	string `env="field1" default:"F1"`
	Field4	float64	`flag:"field4" default:"F4" describe:
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

## Common tags

- `default:"value"` specifies a default value, applies to other data suppliers too.
- `help:"a help message describing the flag, does not need to reference the name"`

## Environment variables

Usage:

- `env:"VARNAME"` specifies that a value can or should be loaded from an environment variable

## Flag setup

#### There are many diferent flavors of flags in use.  

	-verbose 3
	-verbose=3
	--verbose 3
	--verbose=3
	-v 3
	-v -v -v
	-vvv 
	v


#### How do you combine them?

	--debug=true
	-d 
	--verbose=3
	-v

	-vd 3 true
	vd 3 true

### Usage

- `flag:"name"` specifies the name of the command line flag to fill a value.
- `flag:"name n"` specifies a single letter alternative
- `flag:"name,split=comma` for array values, specifies that strings will be split on comma, flag can only be given once
- `flag:"name,explode=true` for array values, specifies that the flag can be given multiple times and is not split
- `flag:"name,content=application/json` specifies that the value is JSON that should be decoded
- `flag:"name,counter=true` for integer values, counts the number of times the flag is used, flag cannot take argument

### Default behavior

The default behavior is that single-letter are used after a single dash and may be combined.  Multi-letter
flags require a two-dash prefix.

	--debug 
	-d
	--verbose
	-v
	-dv (debug and verbose)

For boolean values, negation is "--no-":

	-no-verbose

