package nfigure

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muir/nject/nject"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

type FlagHandler struct {
	fhInheritable
	Parent             *FlagHandler // set only for subcommands
	subcommands        map[string]*FlagHandler
	longFlags          map[string]*flagRef
	shortFlags         map[string]*flagRef
	mapFlags           map[string]*flagRef
	mapRE              *regexp.Regexp
	remainder          []string
	onActivate         func(*Registry, *FlagHandler) error
	onStart            func(*Registry, *FlagHandler, []string) error
	delayedErr         error
	configModel        interface{}
	selectedSubcommand string
}

type fhInheritable struct {
	tagName       string
	registry      *Registry
	stopOnNonFlag bool
	doubleDash    bool
	singleDash    bool
	combineShort  bool
	negativeNo    bool
}

type flagTag struct {
	Name      []string `pt:"0,split=space"`
	Split     string   `pt:"split"` // special value: explode, quote, space, comma
	IsCounter bool     `pt:"counter"`
}

type flagRef struct {
	flagTag
	isBool  bool
	isSlice bool
	isMap   bool
	explode int // for arrays only
	setters map[setterKey]func(reflect.Value, string) error
	values  []string
	used    []string
	keys    []string
}

type setterKey struct {
	typ reflect.Type
	tag string
}

var _ Filler = &FlagHandler{}

// PosixFlagHandler creates and configures a flaghandler that
// requires long options to be preceeded with a double-dash
// and will combine short flags together.
//
// Long-form booleans can be set to false with a "no-" prefix.
//
//	tar -xvf f.tgz --numeric-owner --hole-detection=raw --ownermap ownerfile --no-overwrite-dir
//
func PosixFlagHandler(opts ...FlaghandlerOptArg) *FlagHandler {
	h := &FlagHandler{
		fhInheritable: fhInheritable{
			doubleDash:   true,
			combineShort: true,
			negativeNo:   true,
		},
	}
	h.init()
	h.delayedErr = h.opts(opts)
	return h
}

func GoFlagHandler(opts ...FlaghandlerOptArg) *FlagHandler {
	h := &FlagHandler{
		fhInheritable: fhInheritable{
			doubleDash: true,
			singleDash: true,
		},
	}
	h.init()
	h.delayedErr = h.opts(opts)
	return h
}

func (h *FlagHandler) init() {
	h.subcommands = make(map[string]*FlagHandler)
	h.longFlags = make(map[string]*flagRef)
	h.shortFlags = make(map[string]*flagRef)
	h.mapFlags = make(map[string]*flagRef)
}

func (h *FlagHandler) opts(opts []FlaghandlerOptArg) error {
	for _, f := range opts {
		err := f(h)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *FlagHandler) PreConfigure(tagName string, registry *Registry) error {
	fmt.Println("XXX preconfigure")
	h.tagName = tagName
	h.registry = registry
	if h.delayedErr != nil {
		return h.delayedErr
	}
	if h.configModel != nil {
		err := registry.Request(h.configModel)
		if err != nil {
			return err
		}
	}
	if h.onActivate != nil {
		err := h.onActivate(registry, h)
		if err != nil {
			return err
		}
	}
	return h.parseFlags(0)
}

func (h *FlagHandler) ConfigureComplete() error {
	if h.selectedSubcommand != "" {
		err := h.subcommands[h.selectedSubcommand].ConfigureComplete()
		if err != nil {
			return errors.Wrap(err, h.selectedSubcommand)
		}
	}
	if h.onStart != nil {
		err := h.onStart(h.registry, h, h.remainder)
		if err != nil {
			return err
		}
	}
	return nil
}

type FlaghandlerOptArg func(*FlagHandler) error

// OnActivate is called before flags are parsed.  It's mostly for subcommands.  The
// callback will be invoked as soon as it is known that the subcommand is being
// used.
func OnActivate(chain ...interface{}) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		return nject.Sequence("default-error-responder",
			nject.Provide("default-error", func() nject.TerminalError {
				return nil
			})).Append("on-activate", chain...).Bind(&h.onActivate, nil)
	}
}

func OnStart(chain ...interface{}) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		return nject.Sequence("default-error-responder",
			nject.Provide("default-error", func() nject.TerminalError {
				return nil
			})).Append("on-start", chain...).Bind(&h.onStart, nil)
	}
}

func (h *FlagHandler) AddSubcommand(command string, configModel interface{}, opts ...FlaghandlerOptArg) (*FlagHandler, error) {
	if configModel != nil {
		v := reflect.ValueOf(configModel)
		if !v.IsValid() || v.IsNil() || v.Type().Kind() != reflect.Ptr || v.Type().Elem().Kind() != reflect.Struct {
			return nil, errors.Errorf("configModel must be a nil or a non-nil pointer to a struct, not %T", configModel)
		}
	}
	sub := &FlagHandler{
		fhInheritable: h.fhInheritable,
		Parent:        h,
		configModel:   configModel,
	}
	h.subcommands[command] = sub
	sub.init()
	return sub, sub.opts(opts)
}

func (h *FlagHandler) parseFlags(i int) error {
	fmt.Println("XXX parseflags", i)
	var remainder []string

	if len(h.mapFlags) > 0 {
		me := make([]string, 0, len(h.mapFlags))
		for flag := range h.mapFlags {
			me = append(me, regexp.QuoteMeta(flag))
		}
		var err error
		h.mapRE, err = regexp.Compile(`^(` + strings.Join(me, "|") + `)(.+)$`)
		if err != nil {
			return errors.Wrap(err, "unexpected internal error")
		}
	}

	longFlag := func(dash string, noDash string) (bool, error) {
		if i := strings.IndexByte(noDash, '='); i != -1 {
			flag := noDash[0:i]
			value := noDash[i+1:]
			ref, ok := h.longFlags[flag]
			if ok {
				if ref.explode > 0 {
					return false, errors.Errorf("Flag %s%s expects %d positional arguments following and cannot be used as %s%s=value", dash, flag, ref.explode, dash, flag)
				}
				ref.values = append(ref.values, value)
				ref.used = append(ref.used, dash+flag)
				return true, nil
			}
			if h.mapRE != nil {
				if m := h.mapRE.FindStringSubmatch(flag); len(m) > 0 {
					if ref, ok := h.mapFlags[m[1]]; ok {
						ref.keys = append(ref.keys, m[2])
						ref.values = append(ref.values, value)
						ref.used = append(ref.used, dash+m[1])
						return true, nil
					}
					return false, errors.New("internal error: expected to find mapFlag")
				}
			}
			return false, UsageError(errors.Errorf("Flag %s%s not defined", dash, flag))
		}
		if ref, ok := h.longFlags[noDash]; ok {
			switch {
			case ref.isBool:
				ref.values = append(ref.values, "t")
				ref.used = append(ref.used, dash+noDash)
			case ref.IsCounter:
				ref.values = append(ref.values, "")
				ref.used = append(ref.used, dash+noDash)
			default:
				count := 1
				if ref.explode != 0 {
					count = ref.explode
				}
				if i+count >= len(os.Args) {
					return false, errors.Errorf("Expecting %d positional arguments after %s%s, but only %d are available",
						count, dash, noDash, len(os.Args)-i-1)
				}
				i++
				ref.values = append(ref.values, os.Args[i:i+count]...)
				ref.used = append(ref.used, repeatString(dash+noDash, count)...)
			}
			return true, nil
		}
		if h.negativeNo && strings.HasPrefix(noDash, "no-") {
			if ref, ok := h.longFlags[noDash[3:]]; ok && ref.isBool {
				ref.values = append(ref.values, "f")
				ref.used = append(ref.used, dash+noDash)
				return true, nil
			}
		}
		return false, UsageError(errors.Errorf("Flag %s%s not defined", dash, noDash))
	}

	for ; i < len(os.Args); i++ {
		f := os.Args[i]
		if f == "--" {
			remainder = os.Args[i+1:]
			break
		}
		if h.doubleDash && strings.HasPrefix(f, "--") {
			handled, err := longFlag("--", f[2:])
			if err != nil {
				return err
			}
			if handled {
				continue
			}
		}
		if strings.HasPrefix(f, "-") && f != "-" {
			if h.singleDash {
				handled, err := longFlag("-", f[1:])
				if err != nil {
					return err
				}
				if handled {
					continue
				}
			}
			if h.combineShort {
				potentialFlags := f[1:]
				for len(potentialFlags) > 0 {
					r, size := utf8.DecodeRuneInString(potentialFlags)
					ref, ok := h.shortFlags[string(r)]
					if !ok {
						return UsageError(errors.Errorf("Flag -%c (in %s) not defined", r, f))
					}
					fmt.Printf("XXX ref for -%c: %+v\n", r, ref)
					switch {
					case ref.isBool:
						ref.values = append(ref.values, "t")
						ref.used = append(ref.used, "-"+string(r))
					case ref.IsCounter:
						ref.values = append(ref.values, "")
						ref.used = append(ref.used, "-"+string(r))
					default:
						count := 1
						if ref.explode != 0 {
							count = ref.explode
						}
						if i+count >= len(os.Args) {
							return errors.Errorf("Expecting %d positional arguments after -%c (in %s), but only %d are available",
								count, r, f, len(os.Args)-i-1)
						}
						i++
						fmt.Println("XXX CAPTURING", string(r), os.Args[i:i+count])
						ref.values = append(ref.values, os.Args[i:i+count]...)
						ref.used = append(ref.used, repeatString("-"+string(r), count)...)
					}
					potentialFlags = potentialFlags[size:]
				}
				continue
			}
		}
		if sub, ok := h.subcommands[f]; ok {
			if sub.configModel != nil {
				err := h.registry.Request(sub.configModel)
				if err != nil {
					return err
				}
			}
			if sub.onActivate != nil {
				err := sub.onActivate(h.registry, sub)
				if err != nil {
					return err
				}
			}
			h.selectedSubcommand = f
			return sub.parseFlags(i + 1)
		}
		remainder = os.Args[i:]
		break
	}
	h.remainder = remainder
	return nil
}

func (h *FlagHandler) Fill(t reflect.Type, v reflect.Value, tag reflectutils.Tag) (bool, error) {
	switch t.Kind() {
	case reflect.Ptr:
		// let fill recurse for us
		return false, nil
	}
	ref := flagRef{
		flagTag: flagTag{
			Split: ",",
		},
	}
	err := tag.Fill(&ref.flagTag)
	if err != nil {
		return false, err
	}
	var found bool
	for _, n := range ref.Name {
		var m *map[string]*flagRef
		switch utf8.RuneCountInString(n) {
		case 0:
			continue
		case 1:
			m = &h.shortFlags
		default:
			m = &h.longFlags
		}
		ref, ok := (*m)[n]
		if !ok {
			return false, errors.New("internal error: Could not find pre-registered flagRef")
		}
		if len(ref.values) == 0 && !ref.isMap {
			found = true
			continue
		}
		setter, ok := ref.setters[setterKey{
			typ: t,
			tag: tag.Tag,
		}]
		if !ok {
			return false, errors.New("internal error: Missing setter")
		}
		fmt.Println("XXX filling", tag)
		fmt.Printf("XXX fill with %+v\n", ref)
		if ref.IsCounter {
			var count int
			var err error
			for i, value := range ref.values {
				fmt.Println("XXX counter value", value)
				if value == "" {
					count++
					continue
				}
				replacement, err := strconv.Atoi(value)
				if err != nil {
					return false, errors.Wrapf(err, "value for counter, %s", ref.used[i])
				}
				count = replacement
			}
			if err != nil {
				return false, err
			}
			value := strconv.Itoa(count)
			err = setter(v, value)
			if err != nil {
				return false, err
			}
			return true, nil
		}
		if ref.isSlice && ref.Split == "none" || len(ref.values) > 1 {
			if len(ref.values) == 0 {
				return false, nil
			}
			setElem, err := reflectutils.MakeStringSetter(t.Elem())
			if err != nil {
				return false, errors.Wrap(err, ref.used[0])
			}
			var a reflect.Value
			switch t.Kind() {
			case reflect.Array:
				a = v
				if len(ref.values) > v.Len() {
					ref.values = ref.values[0:v.Len()]
				}
			case reflect.Slice:
				a = reflect.MakeSlice(t, len(ref.values), len(ref.values))
				v.Set(a)
			default:
				return false, errors.Errorf("internal error: not expecting %s", t)
			}
			for i, value := range ref.values {
				fmt.Println("XXX VALUE", i, value)
				err := setElem(a.Index(i), value)
				if err != nil {
					return false, UsageError(errors.Wrap(err, ref.used[i]))
				}
			}
			return true, nil
		}
		if ref.isMap {
			if len(ref.keys) == 0 {
				return false, nil
			}
			m := reflect.MakeMap(t)
			ks, err := reflectutils.MakeStringSetter(reflect.PtrTo(t.Key()))
			if err != nil {
				return false, errors.Wrap(err, ref.used[0])
			}
			es, err := reflectutils.MakeStringSetter(reflect.PtrTo(t.Elem()))
			if err != nil {
				return false, errors.Wrap(err, ref.Name[0])
			}
			for i, value := range ref.values {
				key := ref.keys[i]
				kp := reflect.New(t.Key())
				err := ks(kp, key)
				if err != nil {
					return false, errors.Wrapf(err, "key for %s", ref.used[i])
				}
				ep := reflect.New(t.Elem())
				err = es(ep, value)
				if err != nil {
					return false, errors.Wrapf(err, "value for %s", ref.used[i])
				}
				m.SetMapIndex(reflect.Indirect(kp), reflect.Indirect(ep))
			}
			v.Set(m)
			return true, nil
		}
		err := setter(v, ref.values[len(ref.values)-1])
		if err != nil {
			return false, errors.Wrap(err, ref.used[len(ref.values)-1])
		}
		return true, nil
	}
	if found {
		return false, nil
	}
	return false, errors.New("missing prewalk")
}

func (h *FlagHandler) PreWalk(tagName string, request *Request, model interface{}) error {
	fmt.Println("XXX prewalk", tagName, ".")
	v := reflect.ValueOf(model)
	var walkErr error
	reflectutils.WalkStructElements(v.Type(), func(f reflect.StructField) bool {
		ref := flagRef{
			flagTag: flagTag{
				Split: ",",
			},
		}
		tag := reflectutils.SplitTag(f.Tag).Set().Get(tagName)
		err := tag.Fill(&ref)
		if err != nil {
			walkErr = err
			return true
		}
		t := f.Type
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Bool:
			ref.isBool = true
		case reflect.Slice, reflect.Array:
			ref.isSlice = true
			ref.IsCounter = false
		case reflect.Map:
			ref.isMap = true
			ref.IsCounter = false
		}
		switch ref.Split {
		case "comma":
			ref.Split = ","
		case "quote":
			ref.Split = `"`
		case "space":
			ref.Split = " "
		case "explode":
			if t.Kind() != reflect.Array {
				walkErr = errors.New("split=explode tag is for array types only")
				return false
			}
			ref.explode = t.Len()
			ref.Split = ""
		}
		setter, err := reflectutils.MakeStringSetter(f.Type, reflectutils.WithSplitOn(ref.Split))
		if err != nil {
			walkErr = errors.Wrap(err, f.Name)
			return true
		}
		for _, n := range ref.Name {
			var m *map[string]*flagRef
			switch utf8.RuneCountInString(n) {
			case 0:
				continue
			case 1:
				m = &h.shortFlags
			default:
				m = &h.longFlags
			}
			if ref.isMap {
				m = &h.mapFlags
			}
			sk := setterKey{
				typ: f.Type,
				tag: tag.Tag,
			}
			if existing, ok := (*m)[n]; ok {
				// hmm, this flag is defined more than once!
				existing.isBool = existing.isBool && ref.isBool
				existing.setters[sk] = setter
			} else {
				ref.setters = map[setterKey]func(reflect.Value, string) error{
					sk: setter,
				}
				(*m)[n] = &ref
			}
		}
		return true
	})
	return walkErr
}

func (h *FlagHandler) AddConfigFile(file string, keyPath []string) (Filler, error) { return nil, nil }
func (h *FlagHandler) Keys(reflect.Type, reflectutils.Tag) []string                { return nil } // XXX
func (h *FlagHandler) Len(reflect.Type, reflectutils.Tag) int                      { return 0 }
func (h *FlagHandler) Recurse(structName string, t reflect.Type, tag reflectutils.Tag) (Filler, error) {
	return h, nil
}
