// Obligatory // comment

/*
Package nfigure uses reflection to fill configuration structs from
command line arguments, environment variables, and configuration files.

The basic starts with NewRegistry().  Use functional args to control the
set of fillers (environment, files, etc).  Call Request() to add models
(structs) that need to be filled out.  Call ConfigFile() to add configuration
files.

Once that's done, call Configure() to actually fill the structs.

For file fillers (filling from a configuration file), all data elements
that are exported will be filled if there is a matching element in a
configuration file.  Disable filling an element by overriding its fill
name to "-".

The default set of supported tags are:

	nfigure: meta tag, override with WithMetaTag()
	config:  config file filler
	env:     environment variable filler
	default: defaults filler

With the default tags, the behavior of an exported struct field with no tags
matches the behavior of explicitly setting tags like:

	type MyStruct struct {
		Exported string `config:"Exported" env:"-" default:"" nfigure:",!first,desc"`
	}

That is to say that the field will be filled by a matching name in a configuration
file, but it has no default and will not be filled from environment variables.

The meta tag, (default is "nfigure") controls behavior: it provides a default name for recursion,
controls if the first, or last value is taken if multiple fillers can provide values.  It
controls if a sub-struct should be descended into.

By default, no command-line parser is included.

The general form for the tag parameters for handlers and the meta control is 
tag:"name,key=value,flag,!flag".

The name parameter, which is always first, specifies the name to match on.  This overrides
the field name.  If you want to just use the field name, leave the name parameter empty.  Do
not skip it!  A comma is good enough:

	type MyStruct struct {
		Exported string `config:","`
	}

The special value of "-" means to skip that filler entirely for this field.

Boolean parameters can be specified after the name.  The following all mean false:

	!flag
	flag=false
	flag=0
	flag=n

Likewise, "flag,flag=true,flag=1,flag=y" is highly redundent, setting flag to be true
four times.

Each filler defines it's own parameters.  Here's a summary:

	WithMetaTag(), the meta control, "nfigure":
	name (defaults to "")
	last: use the value from the last filler that has one
	desc: descend into structures even if a filler has provided a value
	combine: merge multiple sources for arrays/maps

	NewEnvFiller(), environment variables, "env":
	name (defaults to "-")
	split: how to split strings into array/slice elements

	NewFileFiller(), fill from config files, "config":
	name (defaults to exported field name)

	PosixFlagHandler()/GoFlagHandler, fill from the command line:
	name
	split: how to split strings into array/slice/map key/value elements
		special values: explode, quote, space, comma, equal, equals, none
	map: how to treat maps, values are "explode" or "prefix"
	counter: is this a numeric counter (eg: foo -v -v -v)
	required: is this a required flag
	argName: how to describe parameters for this in the usage message

Known bugs / limitations:

Combining of arrays, slices, and maps from multiple sources only works if all the
sources are configuration files.

*/
package nfigure
