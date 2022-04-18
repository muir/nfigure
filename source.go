package nfigure

import (
	"reflect"
	"strconv"

	"github.com/muir/commonerrors"
	"github.com/muir/nflex"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

// FileFiller implements the Filler contract
type FileFiller struct {
	source          nflex.Source
	umarshalOptions []nflex.UnmarshalFileArg
}

var _ CanRecurseFiller = FileFiller{}
var _ CanLenFiller = FileFiller{}
var _ CanKeysFiller = FileFiller{}
var _ CanAddConfigFileFiller = FileFiller{}

// FileFillerOpts is a functional arugment for NewFileFiller()
type FileFillerOpts func(*FileFiller)

// WithUnmarshalOpts passes through to
// https://pkg.go.dev/github.com/muir/nflex#UnmarshalFile
func WithUnmarshalOpts(opts ...nflex.UnmarshalFileArg) FileFillerOpts {
	return func(s *FileFiller) {
		s.umarshalOptions = opts
	}
}

// NewFileFiller creates a CanAddConfigFileFiller filler that implements
// AddConfigFile.  Unlike most other fillers, file fillers will fill values
// without explicit tags by matching config fields to struct field names.
//
// To prevent a match, tag it with "-":
//
//	type MyStruct struct {
//		PrivateField string `config:"-"`       // don't fill this one
//		MyField      string `config:"myField"` // fill this one
//	}
//
func NewFileFiller(opts ...FileFillerOpts) FileFiller {
	s := FileFiller{}
	for _, f := range opts {
		f(&s)
	}
	return s
}

// AddConfigFile is invoked by Registry.ConfigFile to note an additional
// file to fill.
func (s FileFiller) AddConfigFile(path string, keyPath []string) (Filler, error) {
	source, err := nflex.UnmarshalFile(path, s.umarshalOptions...)
	if err != nil {
		return nil, err
	}
	debug("source: adding config file", path)
	return FileFiller{
		source:          nflex.CombineSources(s.source, source),
		umarshalOptions: s.umarshalOptions,
	}, nil
}

type fileTag struct {
	Name string `pt:"0"`
}

// Recurse is part of the CanRecurseFiller contract and is called by registry.Configure()
func (s FileFiller) Recurse(name string) (Filler, error) {
	if s.source == nil {
		debug("source: recurse", name, "-> no filler(nil) from", callers(8))
		return nil, nil
	}
	source := s.source.Recurse(name)
	if source == nil {
		debug("source: recurse", name, "-> does not exist(nil) from", callers(8))
		return nil, nil
	}
	debug("source: recurse", name, "from", callers(4))
	return FileFiller{
		source:          nflex.NewMultiSource(source),
		umarshalOptions: s.umarshalOptions,
	}, nil
}

// Keys is part of the CanKeysFiller contract and is called by registry.Configure()
func (s FileFiller) Keys(t reflect.Type, tag reflectutils.Tag, first, combine bool) ([]string, bool) {
	source := nflex.MultiSourceSetFirst(first).
		Combine(nflex.MultiSourceSetCombine(combine)).
		Apply(s.source)
	keys, err := source.Keys()
	if err != nil {
		return nil, false
	}
	return keys, source.Exists()
}

// Len is part of the CanLenFiller contract and is called by registry.Configure()
func (s FileFiller) Len(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (int, bool) {
	source := nflex.MultiSourceSetFirst(firstFirst).
		Combine(nflex.MultiSourceSetCombine(combineObjects)).
		Apply(s.source)
	length, err := source.Len()
	if err != nil {
		return 0, false
	}
	exists := source.Exists()
	debugf("source: Len %s %v/%v: %d, %v\n", tag, firstFirst, combineObjects, length, exists)
	return length, exists
}

// Fill is part of the Filler contract and is called by registry.Configure()
func (s FileFiller) Fill(
	t reflect.Type,
	v reflect.Value,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (bool, error) {
	debug("source: fill into", t, tag, "first", firstFirst, "combine", combineObjects)
	source := nflex.MultiSourceSetFirst(firstFirst).
		Combine(nflex.MultiSourceSetCombine(combineObjects)).
		Apply(s.source)
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := source.GetInt()
		if err != nil {
			debug("source: could not fill int", err)
			return false, commonerrors.ConfigurationError(err)
		}
		v.SetInt(i)
		return true, nil
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := source.GetInt()
		if err != nil {
			return false, commonerrors.ConfigurationError(err)
		}
		if i < 0 {
			return false, commonerrors.ConfigurationError(errors.Errorf("attempt to set %T to negative value", t))
		}
		v.SetUint(uint64(i))
		return true, nil
	case reflect.Float32, reflect.Float64:
		f, err := source.GetFloat()
		if err != nil {
			return false, commonerrors.ConfigurationError(err)
		}
		v.SetFloat(f)
		return true, nil
	case reflect.Bool:
		b, err := source.GetBool()
		if err != nil {
			return false, commonerrors.ConfigurationError(err)
		}
		v.SetBool(b)
		return true, nil
	case reflect.String:
		s, err := source.GetString()
		if err != nil {
			return false, commonerrors.ConfigurationError(err)
		}
		v.SetString(s)
		return true, nil
	case reflect.Complex64, reflect.Complex128:
		switch source.Type() {
		case nflex.String:
			s, err := source.GetString()
			if err != nil {
				return false, commonerrors.ConfigurationError(err)
			}
			c, err := strconv.ParseComplex(s, 128)
			if err != nil {
				return false, commonerrors.ConfigurationError(errors.WithStack(err))
			}
			v.SetComplex(c)
			return true, nil
		case nflex.Slice:
			length, err := source.Len()
			if err != nil {
				return false, commonerrors.ConfigurationError(errors.Wrap(err, "length for array representation of complex"))
			}
			if length != 2 {
				return false, commonerrors.ConfigurationError(errors.New("wrong length for complex value"))
			}
			r, err := source.GetFloat("0")
			if err != nil {
				return false, commonerrors.ConfigurationError(err)
			}
			i, err := source.GetFloat("1")
			if err != nil {
				return false, commonerrors.ConfigurationError(err)
			}
			c := complex(r, i)
			v.SetComplex(c)
			return true, nil
		case nflex.Map:
			r, err := source.GetFloat("real")
			if err != nil {
				return false, commonerrors.ConfigurationError(err)
			}
			i, err := source.GetFloat("imaginary")
			if err != nil {
				return false, commonerrors.ConfigurationError(err)
			}
			c := complex(r, i)
			v.SetComplex(c)
			return true, nil
		default:
			return false, commonerrors.ConfigurationError(errors.New("wrong type for complex value"))
		}
	default:
		return false, nil
	}
}
