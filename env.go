package nfigure

import (
	"os"
	"fmt"
	"reflect"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

type EnvFiller struct {
	tag   string
	split string
}

func NewEnvFiller() Filler {
	return EnvFiller{
		tag:   "env",
		split: ",",
	}
}

type envTag struct {
	Variable string `pt:"0"`
	Split    string `pt:"split"`
}

func (e EnvFiller) Fill(t reflect.Type, v reflect.Value, tag reflectutils.Tag) (bool, error) {
	var tagData envTag
	err := tag.Fill(&tagData)
	if err != nil {
		return false, errors.Wrapf(err, "%s tag", e.tag)
	}
	fmt.Println("XXX env fill", tag, "->", tagData)
	if tagData.Variable == "" {
		return false, nil
	}
	value, ok := os.LookupEnv(tagData.Variable)
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
		return false, errors.Wrapf(err, "%s tag", e.tag)
	}
	err = setter(v, value)
	if err != nil {
		return false, errors.Wrapf(err, "%s tag", e.tag)
	}
	return true, nil
}

func (e EnvFiller) Len(reflect.Type, reflectutils.Tag) int                         { return 0 }
func (e EnvFiller) Keys(reflect.Type, reflectutils.Tag) []string                   { return nil }
func (e EnvFiller) Recurse(string, reflect.Type, reflectutils.Tag) (Filler, error) { return e, nil }
func (e EnvFiller) AddConfigFile(string, []string) (Filler, error)                 { return e, nil }
func (e EnvFiller) PreWalk(string,  *Request, interface{}) error                            { return nil }
func (e EnvFiller) PreConfigure(string, *Registry) error                           { return nil }
func (s EnvFiller) ConfigureComplete() error { return nil }
