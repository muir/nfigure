package nfigure

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muir/commonerrors"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

func (h *FlagHandler) parseFlags(i int) error {
	h.debug("beginning parse")
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
			return commonerrors.LibraryError(errors.Wrap(err, "unexpected internal error"))
		}
		h.debugf("parseflags mapRE = %s", h.mapRE)
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
				return commonerrors.UsageError(errors.Errorf("Expecting a positional argument after %s, none is available", inErr))
			}
			i++
			kv := strings.SplitN(os.Args[i], ref.Split, 2)
			if len(kv) != 2 {
				return commonerrors.UsageError(errors.Errorf("Expecting key%svalue after %s but didn't find '%s'", ref.Split, inErr, ref.Split))
			}
			h.debugf("parse map split %s = %s", kv[0], kv[1])
			ref.keys = append(ref.keys, kv[0])
			ref.values = append(ref.values, kv[1])
			ref.used = append(ref.used, withDash)
		default:
			count := 1
			if ref.explode != 0 {
				count = ref.explode
			}
			if i+count >= len(os.Args) {
				return commonerrors.UsageError(errors.Errorf("Expecting %d positional arguments after %s, but only %d are available",
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
			return commonerrors.UsageError(errors.Errorf("Flag %s not defined", inErr))
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
					return false, commonerrors.UsageError(errors.Errorf(
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
					return false, commonerrors.LibraryError(errors.New("internal error: expected to find mapFlag"))
				}
			}
			return false, commonerrors.UsageError(errors.Errorf("Flag %s%s not defined", dash, flag))
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
		return false, commonerrors.UsageError(errors.Errorf("Flag %s%s not defined", dash, noDash))
	}

	for ; i < len(os.Args); i++ {
		f := os.Args[i]
		if f == "--" {
			remainder = os.Args[i+1:]
			h.debugf("found -- remaining flags (%d/%d) are positional", i, len(os.Args))
			break
		}
		if h.doubleDash && strings.HasPrefix(f, "--") {
			handled, err := longFlag("--", f[2:])
			if err != nil {
				h.debugf("at %d, failed long flag %s", i, f)
				return err
			}
			if handled {
				h.debugf("at %d, long flag %s handled", i, f)
				continue
			}
		}
		if strings.HasPrefix(f, "-") && f != "-" {
			h.debugf("at %d, single dash %s", i, f)
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
			h.debugf("at %d, selecting subcommand %s", i, f)
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
		h.debugf("at %d, remainder is %v", i, remainder)
		break
	}
	if h.helpText != nil && len(h.longFlags["help"].values) != 0 {
		if testMode {
			testOutput = h.Usage()
			panic("exit0")
		} else {
			fmt.Print(h.Usage())
			os.Exit(0)
		}
	}
	h.remainder = remainder
	return nil
}

// Remaining returns the arguments that were not consumed from os.Args. The other way
// to get the remaining arguments is to add an OnStart callback.
func (h *FlagHandler) Remaining() []string {
	return h.remainder
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
	h.debugf("fill %s %s %s", tag.Tag, tag.Value, t)
	if t.Kind() == reflect.Ptr {
		h.debugf("fill skipping pointer")
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
		h.debugf("fill %s %s map: %v", tag.Tag, tag.Value, rawRef.Map)
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
			return false, commonerrors.LibraryError(errors.Errorf("internal error: Could not find pre-registered flagRef for %s", n))
		}
		if len(ref.values) == 0 {
			found = true
			continue
		}
		h.debugf("fill lookup %s %s %v", tag.Tag, tag.Value, t)
		setter, ok := ref.setters[setterKey{
			typ:   setterType,
			split: ref.Split,
		}]
		if !ok {
			return false, commonerrors.LibraryError(errors.Errorf("internal error: Missing setter for %s:%s", tag.Tag, n))
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
					return false, commonerrors.UsageError(errors.Wrapf(err, "value for counter, %s", ref.used[i]))
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
				return false, commonerrors.ProgrammerError(errors.Wrap(err, ref.used[0]))
			}
			es := setter
			for i, value := range ref.values {
				key := ref.keys[i]
				h.debugf("fill map %s = %s %s %s", key, value, nonPointerType.Key(), nonPointerType.Elem())
				kp := reflect.New(nonPointerType.Key())
				err := ks(kp.Elem(), key)
				if err != nil {
					return false, commonerrors.UsageError(errors.Wrapf(err, "key for %s", ref.used[i]))
				}
				ep := reflect.New(nonPointerType.Elem())
				err = es(ep.Elem(), value)
				if err != nil {
					return false, commonerrors.UsageError(errors.Wrapf(err, "value for %s", ref.used[i]))
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
				return false, commonerrors.ProgrammerError(errors.Wrap(err, ref.used[0]))
			}
			var a reflect.Value
			switch nonPointerType.Kind() {
			case reflect.Array:
				a = v
				if len(ref.values) > v.Len() {
					ref.values = ref.values[0:v.Len()]
				}
			case reflect.Slice:
				a = reflect.MakeSlice(nonPointerType, len(ref.values), len(ref.values))
				v.Set(a)
			default:
				return false, commonerrors.LibraryError(errors.Errorf("internal error: not expecting %s", t))
			}
			for i, value := range ref.values {
				err := setElem(a.Index(i), value)
				if err != nil {
					return false, commonerrors.UsageError(errors.Wrap(err, ref.used[i]))
				}
			}
			return true, nil
		}
		err := setter(v, ref.values[len(ref.values)-1])
		if err != nil {
			return false, commonerrors.UsageError(errors.Wrap(err, ref.used[len(ref.values)-1]))
		}
		return true, nil
	}
	if found {
		return false, nil
	}
	return false, commonerrors.LibraryError(errors.New("missing prewalk"))
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
			return ref, nil, nil, commonerrors.ProgrammerError(errors.New("split=explode tag is for slice, array types only"))
		}
	}
	return ref, setterType, nonPointerType, err
}

// PreWalk is part of the Filler contract and is invoked by Registry.Configure()
//
// PreWalk examines configuration blocks and figures out the flags that
// are defined.  It's possible that more than one field in various config
// blocks references the same flag name.
func (h *FlagHandler) PreWalk(tagName string, model interface{}) error {
	v := reflect.ValueOf(model)
	var walkErr error
	h.debug("beginning PreWalk")
	reflectutils.WalkStructElements(v.Type(), func(f reflect.StructField) bool {
		h.debugf("walk %s %s %s", f.Name, f.Type, f.Tag)
		tag := reflectutils.SplitTag(f.Tag).Set().Get(tagName)
		if tag.Tag == "" {
			return true
		}
		ref, setterType, _, err := parseFlagRef(tag, f.Type)
		if err != nil {
			walkErr = err
			return true
		}
		ref.fieldName = f.Name
		h.rawData = append(h.rawData, f)
		if ref.isMap {
			if ref.Split != "=" && ref.Map == "prefix" {
				walkErr = commonerrors.ProgrammerError(errors.New("map=prefix requires split=equals"))
				return false
			}
			switch ref.Map {
			case "explode", "prefix":
			default:
				walkErr = commonerrors.ProgrammerError(errors.Errorf("map=%s not defined, map=explode|prefix", ref.Map))
			}
		}
		h.debugf("PreWalk make setter %s was %s", setterType, f.Type)
		setter, err := reflectutils.MakeStringSetter(setterType, reflectutils.WithSplitOn(ref.Split))
		if err != nil {
			walkErr = commonerrors.UsageError(errors.Wrap(err, f.Name))
			return true
		}
		for _, n := range ref.Name {
			var m *map[string]*flagRef
			switch utf8.RuneCountInString(n) {
			case 0:
				continue
			case 1:
				m = &h.shortFlags
				h.debugf("prewalk register shortflag")
			default:
				m = &h.longFlags
				h.debugf("prewalk register longflag")
			}
			if ref.isMap && ref.Map == "prefix" {
				m = &h.mapFlags
				h.debugf("prewalk nope, register mapflag")
			}
			h.debugf("prewalk registering %s %s %s", tag.Value, setterType, ref.Split)
			sk := setterKey{
				typ:   setterType,
				split: ref.Split,
			}
			if existing, ok := (*m)[n]; ok {
				// hmm, this flag is defined more than once!
				h.debug("prewalk existing flag registration")
				if existing.flagTagComparable != ref.flagTagComparable || existing.flagRefComparable != ref.flagRefComparable {
					walkErr = commonerrors.ProgrammerError(errors.Errorf("multiple registrations of %s:%s are not compatible with each other: %s/%s/%s vs %s/%s/%s",
						tagName, n,
						existing.fieldName, existing.typ, existing.tagValue,
						ref.fieldName, ref.typ, ref.tagValue,
					))
				}
				existing.isBool = existing.isBool && ref.isBool
				existing.setters[sk] = setter
			} else {
				h.debug("prewalk new flag registration")
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
