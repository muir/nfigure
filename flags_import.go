package nfigure

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muir/commonerrors"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

type hasIsBool interface {
	IsBoolFlag() bool
}

// ImportFlagSet pulls in flags defined with the standard "flag"
// package.  This is useful when there are libaries being used
// that define flags.
//
// flag.CommandLine is the default FlagSet.
//
// ImportFlagSet is not the recommended way to use nfigure, but sometimes
// there is no choice.
func ImportFlagSet(fs *flag.FlagSet) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		if fs.Parsed() {
			return commonerrors.ProgrammerError(errors.New("Cannot import FlagSets that have been parsed"))
		}
		var err error
		fs.VisitAll(func(f *flag.Flag) {
			var isBool bool
			if hib, ok := f.Value.(hasIsBool); ok {
				isBool = hib.IsBoolFlag()
			}
			ref := &flagRef{
				flagTag: flagTag{
					Name: []string{f.Name},
				},
				flagRefComparable: flagRefComparable{
					isBool: isBool,
				},
				imported: f,
			}
			switch utf8.RuneCountInString(f.Name) {
			case 0:
				err = commonerrors.ProgrammerError(errors.New("Invalid flag in FlagSet with no Name"))
			case 1:
				h.shortFlags[f.Name] = ref
			default:
				h.longFlags[f.Name] = ref
			}
			h.imported = append(h.imported, ref)
		})
		return err
	}
}

// importFlags deals with setting values for standard "flags" that have been
// imported.
func (h *FlagHandler) importFlags() error {
	for _, ref := range h.imported {
		switch len(ref.values) {
		case 0:
			if ref.imported.DefValue != "" {
				err := ref.imported.Value.Set(ref.imported.DefValue)
				if err != nil {
					return commonerrors.ProgrammerError(errors.Errorf("Cannot set default value for flag '%s': %s",
						ref.imported.Name, err))
				}
			}
		default:
			for _, value := range ref.values {
				err := ref.imported.Value.Set(value)
				if err != nil {
					return commonerrors.UsageError(errors.Errorf("Cannot set value for flag '%s': %s",
						ref.imported.Name, err))
				}
			}
		}
	}
	if h.selectedSubcommand != "" {
		return h.subcommands[h.selectedSubcommand].importFlags()
	}
	return nil
}

// ExportToFlagSet provides a way to use the regular "flag" package to
// when defining flags in a model.  This provides a compatibility option for
// library writers so that if nfigure is not the primary configuration system
// for a program, flag setting by libraries is still easy.
//
// flag.CommandLine is the default FlagSet.
//
// Only some of the FlaghandlerOptArgs make sense in this context.  The others
// will be ignored.
//
// ExportToFlagSet only exports flags.
//
// Subcommands are not supported by the "flag" package and will be ignored
// by ExportToFlagSet.  Counters are not supported by the "flag" package and
// will be treated as numerical types.
//
// If a flag has multiple aliases, only the first name will be used.
func ExportToFlagSet(fs *flag.FlagSet, tagName string, model interface{}, opts ...FlaghandlerOptArg) error {
	h := GoFlagHandler(opts...)
	err := h.PreWalk(tagName, model)
	if err != nil {
		return err
	}

	defaultTag := "default"
	if h.defaultTag != "" {
		defaultTag = h.defaultTag
	}

	value := reflect.ValueOf(model)
	nonPtr := value
	for nonPtr.Type().Kind() == reflect.Ptr {
		if nonPtr.IsNil() {
			return commonerrors.ProgrammerError(errors.New("Must provide a model or pointer to model, not nil"))
		}
		nonPtr = nonPtr.Elem()
	}

	for _, f := range h.rawData {
		f := f
		v := nonPtr.FieldByIndex(f.Index)
		tagSet := reflectutils.SplitTag(f.Tag).Set()
		tag := tagSet.Get(tagName)
		defaultValue := tagSet.Get(defaultTag)
		ref, setterType, nonPointerType, err := parseFlagRef(tag, f.Type)
		if err != nil {
			return err
		}
		setter, err := reflectutils.MakeStringSetter(setterType, reflectutils.WithSplitOn(ref.Split))
		if err != nil {
			return commonerrors.UsageError(errors.Wrap(err, f.Name))
		}
		help := tagSet.Get(h.helpTag).Value
		if help == "" {
			help = fmt.Sprintf("set %s (%s)", f.Name, f.Type)
		}
		vcopy := v
		getV := func() reflect.Value {
			return vcopy
		}
		vt := v.Type()
		for vt.Kind() == reflect.Ptr {
			current := getV
			getV = func() reflect.Value {
				v := current()
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				return v.Elem()
			}
			vt = vt.Elem()
		}
		if v.Type().Kind() == reflect.Ptr {
			c := getV
			getV = func() reflect.Value {
				v := c()
				getV = func() reflect.Value {
					return v
				}
				return v
			}
		}
		switch {
		case len(ref.Name) == 0:
			return commonerrors.LibraryError(errors.New("missing name"))
		case ref.isBool:
			v := getV()
			var defaultBool bool
			if defaultValue.Value != "" {
				var err error
				defaultBool, err = strconv.ParseBool(defaultValue.Value)
				if err != nil {
					return commonerrors.ProgrammerError(errors.Wrapf(err, "Parse value of %s tag for default bool", defaultTag))
				}
			}
			fs.BoolVar(v.Addr().Interface().(*bool), ref.Name[0], defaultBool, help)
		case ref.isMap:
			ks, err := reflectutils.MakeStringSetter(nonPointerType.Key())
			if err != nil {
				return commonerrors.ProgrammerError(errors.Wrap(err, ref.used[0]))
			}
			var once bool
			fs.Func(ref.Name[0], help, func(s string) error {
				if s == "" {
					return commonerrors.UsageError(errors.Errorf("Invalid (empty) value for -%s", ref.Name[0]))
				}
				v := getV()
				if !once {
					m := reflect.MakeMap(nonPointerType)
					v.Set(m)
					once = true
				}
				vals := strings.SplitN(s, ref.Split, 2)
				key := vals[0]
				var value string
				if len(vals) == 2 {
					value = vals[1]
				}
				debugf("flagfill map %s = %s %s %s", key, value, nonPointerType.Key(), nonPointerType.Elem())
				kp := reflect.New(nonPointerType.Key())
				err := ks(kp.Elem(), key)
				if err != nil {
					return commonerrors.UsageError(errors.Wrapf(err, "key for %s", ref.Name[0]))
				}
				ep := reflect.New(nonPointerType.Elem())
				err = setter(ep.Elem(), value)
				if err != nil {
					return commonerrors.UsageError(errors.Wrapf(err, "value for %s", ref.Name[0]))
				}
				v.SetMapIndex(reflect.Indirect(kp), reflect.Indirect(ep))
				return nil
			})
		case ref.isSlice:
			setElem, err := reflectutils.MakeStringSetter(nonPointerType.Elem())
			if err != nil {
				return commonerrors.ProgrammerError(errors.Wrap(err, ref.used[0]))
			}
			switch nonPointerType.Kind() {
			case reflect.Array:
				index := 0
				fs.Func(ref.Name[0], help, func(s string) error {
					v := getV()
					var values []string
					if ref.Split != "" {
						values = strings.Split(s, ref.Split)
					} else {
						values = []string{s}
					}
					if len(values)+index > v.Len() {
						return commonerrors.UsageError(errors.Errorf("overflow array %s", ref.Name[0]))
					}
					for i, value := range values {
						err := setElem(v.Index(i+index), value)
						if err != nil {
							return commonerrors.UsageError(errors.Wrap(err, ref.Name[0]))
						}
					}
					index += len(values)
					return nil
				})
			case reflect.Slice:
				var once bool
				fs.Func(ref.Name[0], help, func(s string) error {
					v := getV()
					var values []string
					if ref.Split != "" {
						values = strings.Split(s, ref.Split)
					} else {
						values = []string{s}
					}
					a := reflect.MakeSlice(nonPointerType, len(values), len(values))
					for i, value := range values {
						err := setElem(a.Index(i), value)
						if err != nil {
							return commonerrors.UsageError(errors.Wrap(err, ref.Name[0]))
						}
					}
					if once {
						v.Set(reflect.AppendSlice(v, a))
					} else {
						v.Set(a)
						once = true
					}
					return nil
				})
			default:
				return commonerrors.LibraryError(errors.Errorf("internal error: not expecting %s", v.Type()))
			}

		default:
			fs.Func(ref.Name[0], help, func(s string) error {
				err := setter(v, s)
				return commonerrors.UsageError(errors.Wrap(err, s))
			})
		}
	}
	if debugging {
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Printf("export defined -%s\n", f.Name)
		})
	}

	return nil
}