package nfigure

import (
	"os"
	"reflect"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

// LookupFiller is a variable provider that is based upon looking up strings
// based on their name.  An example is filling data from environment variables.
type LookupFiller struct {
	lookup    func(value string, tag string) (string, bool, error)
	wrapError func(error) error
}

var _ CanLenFiller = LookupFiller{}

// LookupFillerOpt are options for creating LookupFillers
type LookupFillerOpt func(*LookupFiller)

// NewEnvFiller creates a LookupFiller that looks up variables from the environment.
// NewRegistry includes maps "env" to such a filler.
//
// The first word after the tag name is the name of the environment variable.  Also
// supported is a "split" tag that can be used to specify how to split up a value when
// filling an array or slice from a single string.  Special values for split are:
// "none", "comma", "equals".
//
//	type MyStruct struct {
//		WeightFactor float64  `env:"WEIGHT_FACTOR"`
//		Groups       []string `env:"GROUPS,split=|"`
//	}
func NewEnvFiller(opts ...LookupFillerOpt) Filler {
	return NewLookupFillerSimple(os.LookupEnv,
		append([]LookupFillerOpt{WrapLookupErrors(EnvironmentError)},
			opts...)...)
}

// NewDefaultFiller creates a LookupFiller that simpley fills in the value provided
// into the variable.  Comma (",") is not allowed in the values because that is used
// to introduce options common to LookupFiller.
//
// NewRegistry includes maps "default" to such a filler.  To use "dflt" instead,
// add the following optional arguments to your NewRegistry invocation:
//
//	WithFiller("default", nil),
//	WithFiller("dflt", NewDefaultFiller)
//
// To fill a slice, set a split value:
//
//	type MyStruct struct {
//		Users []string `default:"root|nobody,split=|"`
//	}
func NewDefaultFiller(opts ...LookupFillerOpt) Filler {
	return NewLookupFiller(func(_, tag string) (string, bool, error) {
		return tag, true, nil
	}, opts...)
}

// NewLookupFillerSimple creates a LookupFiller from a function that
// does a simple lookup like os.LookupEnv().
func NewLookupFillerSimple(lookup func(string) (value string, ok bool), opts ...LookupFillerOpt) Filler {
	return NewLookupFiller(func(s string, _ string) (string, bool, error) {
		got, ok := lookup(s)
		return got, ok, nil
	}, opts...)
}

// NewLookupFiller creates a LookupFiller from a function that can return error
// in addition to a value and an indicator if a value is returned.  An error return
// will likely cause program termination so it should be used when there is something
// blocking the ability to look up a value.
func NewLookupFiller(
	lookup func(key string, tag string) (value string, ok bool, err error),
	opts ...LookupFillerOpt,
) Filler {
	e := LookupFiller{
		lookup:    lookup,
		wrapError: func(e error) error { return e },
	}
	for _, f := range opts {
		f(&e)
	}
	return e
}

// WrapLookupErrors applies a transformation to errors returned by LookupFillers.
func WrapLookupErrors(f func(error) error) LookupFillerOpt {
	return func(e *LookupFiller) {
		e.wrapError = f
	}
}

type envTag struct {
	Variable string `pt:"0"`
	Split    string `pt:"split"`
}

// Fill is part of the Filler contract.  It is used by Registry.Configure.
func (e LookupFiller) Fill(
	t reflect.Type,
	v reflect.Value,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (bool, error) {
	if tag.Tag == "" {
		return false, nil
	}
	var tagData envTag
	err := tag.Fill(&tagData)
	if err != nil {
		return false, ProgrammerError(errors.Wrapf(err, "%s tag", tag.Tag))
	}
	if tagData.Variable == "" {
		return false, nil
	}
	value, ok, err := e.lookup(tagData.Variable, tag.Value)
	if err != nil {
		return false, ProgrammerError(errors.Wrapf(err, tag.Tag))
	}
	if !ok {
		return false, nil
	}
	var ssa []reflectutils.StringSetterArg
	if tagData.Split != "" {
		ssa = append(ssa, reflectutils.WithSplitOn(tagData.Split))
	}
	setter, err := reflectutils.MakeStringSetter(t, ssa...)
	if err != nil {
		return false, ProgrammerError(errors.Wrapf(err, "%s tag", tag.Tag))
	}
	err = setter(v, value)
	if err != nil {
		return false, e.wrapError(errors.Wrapf(err, "%s tag", tag.Tag))
	}
	return true, nil
}

// Len is part of the Filler contract
func (e LookupFiller) Len(
	t reflect.Type,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (int, bool) {
	return lenThroughFill(e, t, tag, firstFirst, combineObjects)
}

func lenThroughFill(
	f Filler,
	t reflect.Type,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (int, bool) {
	switch reflectutils.NonPointer(t).Kind() {
	case reflect.Array, reflect.Slice:
	//
	default:
		return 0, false
	}
	v := reflect.New(t).Elem()
	filled, err := f.Fill(t, v, tag, firstFirst, combineObjects)
	if err != nil {
		return 0, false
	}
	if !filled {
		return 0, false
	}
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.Len(), true
}
