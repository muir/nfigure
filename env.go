package nfigure

import (
	"os"
	"reflect"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

type LookupFiller struct {
	lookup    func(value string, tag string) (string, bool, error)
	wrapError func(error) error
}

type LookupFillerOpt func(*LookupFiller)

func NewEnvFiller(opts ...LookupFillerOpt) Filler {
	return NewLookupFillerSimple(os.LookupEnv,
		append([]LookupFillerOpt{WrapLookupErrors(EnvironmentError)},
			opts...)...)
}

func NewDefaultFiller(opts ...LookupFillerOpt) Filler {
	return NewLookupFiller(func(_, tag string) (string, bool, error) {
		return tag, true, nil
	}, opts...)
}

func NewLookupFillerSimple(lookup func(string) (string, bool), opts ...LookupFillerOpt) Filler {
	return NewLookupFiller(func(s string, _ string) (string, bool, error) {
		got, ok := lookup(s)
		return got, ok, nil
	}, opts...)
}

func NewLookupFiller(lookup func(value string, tag string) (string, bool, error), opts ...LookupFillerOpt) Filler {
	e := LookupFiller{
		lookup:    lookup,
		wrapError: func(e error) error { return e },
	}
	for _, f := range opts {
		f(&e)
	}
	return e
}

func WrapLookupErrors(f func(error) error) LookupFillerOpt {
	return func(e *LookupFiller) {
		e.wrapError = f
	}
}

type envTag struct {
	Variable string `pt:"0"`
	Split    string `pt:"split"`
}

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

func (e LookupFiller) Keys(reflect.Type, reflectutils.Tag, bool, bool) ([]string, bool) {
	return nil, false
}
func (e LookupFiller) Recurse(string, reflect.Type, reflectutils.Tag) (Filler, error) { return e, nil }
func (e LookupFiller) AddConfigFile(string, []string) (Filler, error)                 { return e, nil }
func (e LookupFiller) PreWalk(string, *Request, interface{}) error                    { return nil }
func (e LookupFiller) PreConfigure(string, *Registry) error                           { return nil }
func (s LookupFiller) ConfigureComplete() error                                       { return nil }
