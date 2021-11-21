package nfigure

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

func (h *FlagHandler) parseFlags(i int) error {
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

	handleShort := func(flag string, inErr string) error {
		ref, ok := h.shortFlags[flag]
		if !ok {
			return UsageError(errors.Errorf("Flag %s not defined", inErr))
		}
		switch {
		case ref.isBool:
			ref.values = append(ref.values, "t")
			ref.used = append(ref.used, "-"+flag)
		case ref.IsCounter:
			ref.values = append(ref.values, "")
			ref.used = append(ref.used, "-"+flag)
		default:
			count := 1
			if ref.explode != 0 {
				count = ref.explode
			}
			if i+count >= len(os.Args) {
				return errors.Errorf("Expecting %d positional arguments after %s, but only %d are available",
					count, inErr, len(os.Args)-i-1)
			}
			i++
			ref.values = append(ref.values, os.Args[i:i+count]...)
			ref.used = append(ref.used, repeatString("-"+flag, count)...)
			i += count - 1
		}
		return nil
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
				i += count - 1
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
			if h.singleDash && utf8.RuneCountInString(f[1:]) > 1 {
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
					err := handleShort(string(r), fmt.Sprintf(
						"-%c (in %s)", r, f))
					if err != nil {
						return err
					}
					potentialFlags = potentialFlags[size:]
				}
				continue
			}
			err := handleShort(f[1:], f)
			if err != nil {
				return err
			}
			continue
		}
		if sub, ok := h.subcommands[f]; ok {
			if sub.configModel != nil {
				err := h.registry.Request(sub.configModel,
					WithFiller(h.tagName, sub))
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

func (h *FlagHandler) Fill(
	t reflect.Type,
	v reflect.Value,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (bool, error) {
	if tag.Tag == "" {
		return false, nil
	}
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
	isMap := reflectutils.NonPointer(t).Kind() == reflect.Map
	for _, n := range ref.Name {
		var m *map[string]*flagRef
		if isMap {
			m = &h.mapFlags
		} else {
			switch utf8.RuneCountInString(n) {
			case 0:
				continue
			case 1:
				m = &h.shortFlags
			default:
				m = &h.longFlags
			}
		}
		ref, ok := (*m)[n]
		if !ok {
			return false, errors.Errorf("internal error: Could not find pre-registered flagRef for %s", n)
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
			return false, errors.Errorf("internal error: Missing setter for -%s", n)
		}
		if ref.IsCounter {
			var count int
			var err error
			for i, value := range ref.values {
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
		if ref.isMap {
			if len(ref.keys) == 0 {
				return false, nil
			}
			m := reflect.MakeMap(t)
			ks, err := reflectutils.MakeStringSetter(t.Key())
			if err != nil {
				return false, errors.Wrap(err, ref.used[0])
			}
			es := setter
			for i, value := range ref.values {
				key := ref.keys[i]
				kp := reflect.New(t.Key())
				err := ks(kp.Elem(), key)
				if err != nil {
					return false, errors.Wrapf(err, "key for %s", ref.used[i])
				}
				ep := reflect.New(t.Elem())
				err = es(ep.Elem(), value)
				if err != nil {
					return false, errors.Wrapf(err, "value for %s", ref.used[i])
				}
				m.SetMapIndex(reflect.Indirect(kp), reflect.Indirect(ep))
			}
			v.Set(m)
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
				err := setElem(a.Index(i), value)
				if err != nil {
					return false, UsageError(errors.Wrap(err, ref.used[i]))
				}
			}
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
	v := reflect.ValueOf(model)
	var walkErr error
	reflectutils.WalkStructElements(v.Type(), func(f reflect.StructField) bool {
		ref := flagRef{
			flagTag: flagTag{
				Split: ",",
			},
		}
		tag := reflectutils.SplitTag(f.Tag).Set().Get(tagName)
		if tag.Tag == "" {
			return true
		}
		h.rawData = append(h.rawData, f)
		err := tag.Fill(&ref)
		if err != nil {
			walkErr = err
			return true
		}
		t := reflectutils.NonPointer(f.Type)
		setterType := t
		switch t.Kind() {
		case reflect.Bool:
			ref.isBool = true
		case reflect.Slice, reflect.Array:
			ref.isSlice = true
			ref.IsCounter = false
		case reflect.Map:
			ref.isMap = true
			ref.IsCounter = false
			setterType = t.Elem()
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
		setter, err := reflectutils.MakeStringSetter(setterType, reflectutils.WithSplitOn(ref.Split))
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
				typ: t,
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
func (h *FlagHandler) Keys(reflect.Type, reflectutils.Tag, bool, bool) ([]string, bool) {
	return nil, false
}
func (h *FlagHandler) Len(reflect.Type, reflectutils.Tag, bool, bool) (int, bool) { return 0, false }

func (h *FlagHandler) Recurse(structName string, t reflect.Type, tag reflectutils.Tag) (Filler, error) {
	return h, nil
}
