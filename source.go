package nfigure

import (
	"reflect"
	"strconv"

	"github.com/muir/nfigure/nflex"
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

type FileFillerOpts func(*FileFiller)

func WithUnmarshalOpts(opts ...nflex.UnmarshalFileArg) FileFillerOpts {
	return func(s *FileFiller) {
		s.umarshalOptions = opts
	}
}

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

func (s FileFiller) Recurse(name string, t reflect.Type, tag reflectutils.Tag) (Filler, error) {
	if s.source == nil {
		debug("source: recurse -> no filler(nil)", name, "from", callers(4))
		return nil, nil
	}
	if tag.Tag != "" {
		var fileTag fileTag
		err := tag.Fill(&fileTag)
		if err != nil {
			return nil, ProgrammerError(errors.Wrap(err, tag.Tag))
		}
		switch fileTag.Name {
		case "-":
			return nil, nil
		case "":
			//
		default:
			name = fileTag.Name
		}
	}
	source := s.source.Recurse(name)
	if source == nil {
		debug("source: recurse -> does not exist(nil)", name, "from", callers(4))
		return nil, nil
	}
	debug("source: recurse using", name, "from", callers(4))
	return FileFiller{
		source:          nflex.NewMultiSource(source),
		umarshalOptions: s.umarshalOptions,
	}, nil
}

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

func (s FileFiller) Len(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (int, bool) {
	source := nflex.MultiSourceSetFirst(firstFirst).
		Combine(nflex.MultiSourceSetCombine(combineObjects)).
		Apply(s.source)
	length, err := source.Len()
	if err != nil {
		return 0, false
	}
	return length, source.Exists()
}

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
			return false, ConfigurationError(err)
		}
		v.SetInt(i)
		return true, nil
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := source.GetInt()
		if err != nil {
			return false, ConfigurationError(err)
		}
		if i < 0 {
			return false, ConfigurationError(errors.Errorf("attempt to set %T to negative value", t))
		}
		v.SetUint(uint64(i))
		return true, nil
	case reflect.Float32, reflect.Float64:
		f, err := source.GetFloat()
		if err != nil {
			return false, ConfigurationError(err)
		}
		v.SetFloat(f)
		return true, nil
	case reflect.Bool:
		b, err := source.GetBool()
		if err != nil {
			return false, ConfigurationError(err)
		}
		v.SetBool(b)
		return true, nil
	case reflect.String:
		s, err := source.GetString()
		if err != nil {
			return false, ConfigurationError(err)
		}
		v.SetString(s)
		return true, nil
	case reflect.Complex64, reflect.Complex128:
		switch source.Type() {
		case nflex.String:
			s, err := source.GetString()
			if err != nil {
				return false, ConfigurationError(err)
			}
			c, err := strconv.ParseComplex(s, 128)
			if err != nil {
				return false, ConfigurationError(errors.WithStack(err))
			}
			v.SetComplex(c)
			return true, nil
		case nflex.Slice:
			length, err := source.Len()
			if err != nil {
				return false, ConfigurationError(errors.Wrap(err, "length for array representation of complex"))
			}
			if length != 2 {
				return false, ConfigurationError(errors.New("wrong length for complex value"))
			}
			r, err := source.GetFloat("0")
			if err != nil {
				return false, ConfigurationError(err)
			}
			i, err := source.GetFloat("1")
			if err != nil {
				return false, ConfigurationError(err)
			}
			c := complex(r, i)
			v.SetComplex(c)
			return true, nil
		case nflex.Map:
			r, err := source.GetFloat("real")
			if err != nil {
				return false, ConfigurationError(err)
			}
			i, err := source.GetFloat("imaginary")
			if err != nil {
				return false, ConfigurationError(err)
			}
			c := complex(r, i)
			v.SetComplex(c)
			return true, nil
		default:
			return false, ConfigurationError(errors.New("wrong type for complex value"))
		}
	default:
		return false, nil
	}
}
