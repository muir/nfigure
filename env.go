package nfigure

import (
	"fmt"
	"os"
	"reflect"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

type LookupFiller struct {
	lookup func(string) (string, bool, error)
}

func NewEnvFiller() Filler {
	return NewLookupFillerSimple(os.LookupEnv)
}

func NewLookupFillerSimple(lookup func(string) (string, bool)) Filler {
	return LookupFiller{
		lookup: func(s string) (string, bool, error) {
			got, ok := lookup(s)
			return got, ok, nil
		},
	}
}

func NewLookupFiller(lookup func(string) (string, bool, error)) Filler {
	return LookupFiller{
		lookup: lookup,
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
		return false, errors.Wrapf(err, "%s tag", tag.Tag)
	}
	fmt.Println("XXX env fill", tag, "->", tagData)
	if tagData.Variable == "" {
		return false, nil
	}
	value, ok, err := e.lookup(tagData.Variable)
	if err != nil {
		return false, errors.Wrapf(err, tag.Tag)
	}
	if !ok {
		fmt.Println("XXX not set", tagData.Variable)
		return false, nil
	}
	fmt.Println("XXX lookup", tagData.Variable, ":", value)
	var ssa []reflectutils.StringSetterArg
	if tagData.Split != "" {
		ssa = append(ssa, reflectutils.WithSplitOn(tagData.Split))
	}
	setter, err := reflectutils.MakeStringSetter(t, ssa...)
	if err != nil {
		return false, errors.Wrapf(err, "%s tag", tag.Tag)
	}
	err = setter(v, value)
	if err != nil {
		return false, errors.Wrapf(err, "%s tag", tag.Tag)
	}
	return true, nil
}

func (e LookupFiller) Len(reflect.Type, reflectutils.Tag, bool, bool) (int, bool) { return 0, false } // XXX
func (e LookupFiller) Keys(reflect.Type, reflectutils.Tag, bool, bool) ([]string, bool) {
	return nil, false
}                                                                                     // XXX
func (e LookupFiller) Recurse(string, reflect.Type, reflectutils.Tag) (Filler, error) { return e, nil }
func (e LookupFiller) AddConfigFile(string, []string) (Filler, error)                 { return e, nil }
func (e LookupFiller) PreWalk(string, *Request, interface{}) error                    { return nil }
func (e LookupFiller) PreConfigure(string, *Registry) error                           { return nil }
func (s LookupFiller) ConfigureComplete() error                                       { return nil }
