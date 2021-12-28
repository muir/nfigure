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
	debug("flags: begininng parse")
	if h.alreadyParsed {
		h.clearParse()
	}
	h.alreadyParsed = true
	err := h.addHelpFlagAndCommand(false)
	if err != nil {
		return err
	}
	var remainder []string

	if len(h.mapFlags) > 0 {
		me := make([]string, 0, len(h.mapFlags))
		for flag := range h.mapFlags {
			me = append(me, regexp.QuoteMeta(flag))
		}
		var err error
		h.mapRE, err = regexp.Compile(`^(` + strings.Join(me, "|") + `)(.+)$`)
		if err != nil {
			return NFigureError(errors.Wrap(err, "unexpected internal error"))
		}
		debugf("parseflags mapRE = %s", h.mapRE)
	}

	handleFollowingArgs := func(ref *flagRef, flag string, withDash string, inErr string) error {
		switch {
		case ref.isBool:
			ref.values = append(ref.values, "t")
			ref.used = append(ref.used, withDash)
		case ref.IsCounter:
			ref.values = append(ref.values, "")
			ref.used = append(ref.used, withDash)
		case ref.isMap:
			if i+1 >= len(os.Args) {
				return UsageError(errors.Errorf("Expecting a positional argument after %s, none is available", inErr))
			}
			i++
			kv := strings.SplitN(os.Args[i], ref.Split, 2)
			if len(kv) != 2 {
				return UsageError(errors.Errorf("Expecting key%svalue after %s but didn't find '%s'", ref.Split, inErr, ref.Split))
			}
			debugf("parse map split %s = %s", kv[0], kv[1])
			ref.keys = append(ref.keys, kv[0])
			ref.values = append(ref.values, kv[1])
			ref.used = append(ref.used, withDash)
		default:
			count := 1
			if ref.explode != 0 {
				count = ref.explode
			}
			if i+count >= len(os.Args) {
				return UsageError(errors.Errorf("Expecting %d positional arguments after %s, but only %d are available",
					count, inErr, len(os.Args)-i-1))
			}
			i++
			ref.values = append(ref.values, os.Args[i:i+count]...)
			ref.used = append(ref.used, repeatString(withDash, count)...)
			i += count - 1
		}
		return nil
	}

	handleShort := func(flag string, inErr string) error {
		ref, ok := h.shortFlags[flag]
		if !ok {
			return UsageError(errors.Errorf("Flag %s not defined", inErr))
		}
		return handleFollowingArgs(ref, flag, "-"+flag, inErr)
	}

	longFlag := func(dash string, noDash string) (bool, error) {
		if i := strings.IndexByte(noDash, '='); i != -1 {
			flag := noDash[0:i]
			value := noDash[i+1:]
			ref, ok := h.longFlags[flag]
			if ok {
				if ref.explode > 0 {
					return false, UsageError(errors.Errorf(
						"Flag %s%s expects %d positional arguments following and cannot be used as %s%s=value",
						dash, flag, ref.explode, dash, flag))
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
					return false, NFigureError(errors.New("internal error: expected to find mapFlag"))
				}
			}
			return false, UsageError(errors.Errorf("Flag %s%s not defined", dash, flag))
		}
		if ref, ok := h.longFlags[noDash]; ok {
			return true, handleFollowingArgs(ref, noDash, dash+noDash, dash+noDash)
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
			debug("flags: selecting subcommand", f)
			if sub.configModel != nil {
				err := h.registry.Request(sub.configModel,
					WithFiller(h.tagName, sub))
				if err != nil {
					return err
				}
			}
			h.selectedSubcommand = f
			sub.tagName = h.tagName   // set late (by PreConfigure) so must be propagated
			sub.registry = h.registry // set late (by PreConfigure) so must be propagated
			if sub.onActivate != nil {
				err := sub.onActivate(h.registry, sub)
				if err != nil {
					return err
				}
			}
			return sub.parseFlags(i + 1)
		}
		remainder = os.Args[i:]
		break
	}
	if h.helpText != nil && len(h.longFlags["help"].values) != 0 {
		fmt.Print(h.Usage())
		os.Exit(0)
	}
	h.remainder = remainder
	return nil
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
					return errors.Errorf("Cannot set default value for flag '%s': %s",
						ref.imported.Name, err)
				}
			}
		case 1:
			err := ref.imported.Value.Set(ref.values[0])
			if err != nil {
				return errors.Errorf("Cannot set value for flag '%s': %s",
					ref.imported.Name, err)
			}
		default:
			return errors.Errorf("Cannot set multiple values for flag '%s'", ref.imported.Name)
		}
	}
	if h.selectedSubcommand != "" {
		return h.subcommands[h.selectedSubcommand].importFlags()
	}
	return nil
}

// Fill is part of the Filler interface and will be invoked by Registry.Configure().
//
// Fill may be called multiple times for the same field: if it's a
// pointer, then fill will first be called for it as a pointer, and
// then later it will be called for it as a regular value.  Generally,
// we only want to respond when called as the regular value.
func (h *FlagHandler) Fill(
	t reflect.Type,
	v reflect.Value,
	tag reflectutils.Tag,
	firstFirst bool,
	combineObjects bool,
) (bool, error) {
	debugf("fill %s %s %s", tag.Tag, tag.Value, t)
	if t.Kind() == reflect.Ptr {
		debugf("fill skipping pointer")
		return false, nil
	}
	if tag.Tag == "" {
		return false, nil
	}
	rawRef, setterType, nonPointerType, err := parseFlagRef(tag, t)
	if err != nil {
		return false, err
	}
	var found bool
	isMap := nonPointerType.Kind() == reflect.Map
	for _, n := range rawRef.Name {
		var m *map[string]*flagRef
		debugf("fill flag %s %s map: %v", tag.Tag, tag.Value, rawRef.Map)
		if isMap && rawRef.Map == "prefix" {
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
			return false, NFigureError(errors.Errorf("internal error: Could not find pre-registered flagRef for %s", n))
		}
		if len(ref.values) == 0 {
			found = true
			continue
		}
		debugf("fill lookup %s %s %v", tag.Tag, tag.Value, t)
		setter, ok := ref.setters[setterKey{
			typ:   setterType,
			split: ref.Split,
		}]
		if !ok {
			return false, NFigureError(errors.Errorf("internal error: Missing setter for %s:%s", tag.Tag, n))
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
					return false, UsageError(errors.Wrapf(err, "value for counter, %s", ref.used[i]))
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
			ks, err := reflectutils.MakeStringSetter(nonPointerType.Key())
			if err != nil {
				return false, ProgrammerError(errors.Wrap(err, ref.used[0]))
			}
			es := setter
			for i, value := range ref.values {
				key := ref.keys[i]
				debugf("flagfill map %s = %s %s %s", key, value, nonPointerType.Key(), nonPointerType.Elem())
				kp := reflect.New(t.Key())
				err := ks(kp.Elem(), key)
				if err != nil {
					return false, UsageError(errors.Wrapf(err, "key for %s", ref.used[i]))
				}
				ep := reflect.New(nonPointerType.Elem())
				err = es(ep.Elem(), value)
				if err != nil {
					return false, UsageError(errors.Wrapf(err, "value for %s", ref.used[i]))
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
			setElem, err := reflectutils.MakeStringSetter(nonPointerType.Elem())
			if err != nil {
				return false, ProgrammerError(errors.Wrap(err, ref.used[0]))
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
				return false, NFigureError(errors.Errorf("internal error: not expecting %s", t))
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
			return false, UsageError(errors.Wrap(err, ref.used[len(ref.values)-1]))
		}
		return true, nil
	}
	if found {
		return false, nil
	}
	return false, NFigureError(errors.New("missing prewalk"))
}

func parseFlagRef(tag reflectutils.Tag, t reflect.Type) (flagRef, reflect.Type, reflect.Type, error) {
	ref := flagRef{
		flagTag: flagTag{
			flagTagComparable: flagTagComparable{
				Split: ",",
			},
		},
		typ:      t,
		tagValue: tag.Value,
	}
	nonPointerType := reflectutils.NonPointer(t)
	setterType := nonPointerType
	switch nonPointerType.Kind() {
	case reflect.Bool:
		ref.isBool = true
	case reflect.Slice, reflect.Array:
		ref.isSlice = true
		ref.IsCounter = false
	case reflect.Map:
		ref.isMap = true
		ref.IsCounter = false
		ref.Split = "="
		ref.Map = "explode"
		setterType = nonPointerType.Elem()
	}
	err := tag.Fill(&ref)
	return ref, setterType, nonPointerType, err
}

// PreWalk is part of the Filler contract and is invoked by Registry.Configure()
//
// PreWalk examines configuration blocks and figures out the flags that
// are defined.  It's possible that more than one field in various config
// blocks references the same flag name.
func (h *FlagHandler) PreWalk(tagName string, request *Request, model interface{}) error {
	v := reflect.ValueOf(model)
	var walkErr error
	reflectutils.WalkStructElements(v.Type(), func(f reflect.StructField) bool {
		debugf("prewalk %s %s %s", f.Name, f.Type, f.Tag)
		tag := reflectutils.SplitTag(f.Tag).Set().Get(tagName)
		if tag.Tag == "" {
			return true
		}
		ref, setterType, nonPointerType, err := parseFlagRef(tag, f.Type)
		if err != nil {
			walkErr = err
			return true
		}
		ref.fieldName = f.Name
		h.rawData = append(h.rawData, f)
		switch ref.Split {
		case "none":
			ref.Split = ""
		case "equal", "equals":
			ref.Split = "="
		case "comma":
			ref.Split = ","
		case "quote":
			ref.Split = `"`
		case "space":
			ref.Split = " "
		case "explode":
			switch nonPointerType.Kind() {
			case reflect.Slice:
				ref.Split = ""
			case reflect.Array:
				ref.explode = nonPointerType.Len()
			default:
				walkErr = ProgrammerError(errors.New("split=explode tag is for slice, array types only"))
				return false
			}
		}
		if ref.isMap {
			if ref.Split != "=" && ref.Map == "prefix" {
				walkErr = ProgrammerError(errors.New("map=prefix requires split=equals"))
				return false
			}
			switch ref.Map {
			case "explode", "prefix":
			default:
				walkErr = ProgrammerError(errors.Errorf("map=%s not defined, map=explode|prefix", ref.Map))
			}
		}
		debugf("PreWalk make setter %s was %s", setterType, f.Type)
		setter, err := reflectutils.MakeStringSetter(setterType, reflectutils.WithSplitOn(ref.Split))
		if err != nil {
			walkErr = UsageError(errors.Wrap(err, f.Name))
			return true
		}
		for _, n := range ref.Name {
			var m *map[string]*flagRef
			switch utf8.RuneCountInString(n) {
			case 0:
				continue
			case 1:
				m = &h.shortFlags
				debugf("prewalk register shortflag")
			default:
				m = &h.longFlags
				debugf("prewalk register longflag")
			}
			if ref.isMap && ref.Map == "prefix" {
				m = &h.mapFlags
				debugf("prewalk nope, register mapflag")
			}
			debugf("prewalk registring %s %s %s", tag.Value, setterType, ref.Split)
			sk := setterKey{
				typ:   setterType,
				split: ref.Split,
			}
			if existing, ok := (*m)[n]; ok {
				// hmm, this flag is defined more than once!
				debugf("prewalk existing flag regsitration")
				if existing.flagTagComparable != ref.flagTagComparable || existing.flagRefComparable != ref.flagRefComparable {
					walkErr = ProgrammerError(errors.Errorf("multiple registrations of %s:%s are not compatible with each other: %s/%s/%s vs %s/%s/%s",
						tagName, n,
						existing.fieldName, existing.typ, existing.tagValue,
						ref.fieldName, ref.typ, ref.tagValue,
					))
				}
				existing.isBool = existing.isBool && ref.isBool
				existing.setters[sk] = setter
			} else {
				debugf("prewalk new flag regsitration")
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
