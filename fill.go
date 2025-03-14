package nfigure

import (
	"reflect"
	"strconv"

	"github.com/muir/commonerrors"
	"github.com/muir/nfigure/internal/pointer"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

// Filler s are applied recursively to structures that need
// to be filled.
type Filler interface {
	// Fill is what's used to populate data in a configuration struct.
	// Fillers can choose: they can fill structs, maps, arrays, slices,
	// and pointers or they can wait for Recurse to be called and then
	// Fill to be called on slice items and struct fields and map values.
	// Map keys must come from Keys() or the struct as a whole.
	Fill(t reflect.Type, v reflect.Value, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (filledAnything bool, err error)
}

// CanRecurseFiller indicates that Recurse() is supported
type CanRecurseFiller interface {
	Filler
	// Recurse is called during the filling process to indicate
	// that we are now filling a sub-struct, array element, map
	// element etc.
	//
	// If the filler knows that with the recursion it will no longer
	// try to fill anything, it can return nil for it's replacment
	// filler.
	Recurse(name string) (Filler, error)
}

// CanLenFiller indicates that Len() is supported
type CanLenFiller interface {
	Filler
	// for filling arrays & slices
	Len(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (int, bool)
}

// CanKeysFiller indicates that Keys() is supported
type CanKeysFiller interface {
	Filler
	// for filling maps
	Keys(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) ([]string, bool)
}

// CanPreWalkFiller indicates the PreWalk is supported
type CanPreWalkFiller interface {
	Filler
	// PreWalk is called from nfigure.Request only on every known (at that time) configuration
	// struct before any call to Fill()
	PreWalk(tagName string, model interface{}) error
}

// CanConfigureCompleteFiller indicates that ConfigureComplete is supported
type CanConfigureCompleteFiller interface {
	Filler
	// ConfigureComplete is called by Registry.Configure() when all configuration is complete.
	// This is currently skipped for Fillers that are subcommand specific.
	ConfigureComplete() error
}

// CanPreConfigureFiller indicates that PreConfigure is supported
type CanPreConfigureFiller interface {
	Filler
	// PreConfigure is called by nfigure.Registry once just before configuration starts
	PreConfigure(tagName string, request *Registry) error
}

// CanAddConfigFileFiller indicates AddConfigFile is supported
type CanAddConfigFileFiller interface {
	Filler
	// If the file type is not supported by this filler, then
	// nflex.UnknownFileTypeError must be returned.
	AddConfigFile(file string, keyPath []string) (Filler, error)
}

type fillData struct {
	r       *Request
	name    string
	tags    reflectutils.TagSet
	meta    metaFields
	fillers *fillerCollection
}

type metaFields struct {
	Name    string `pt:"0"`
	First   *bool  `pt:"first,!last"`     // default is take the first
	Combine *bool  `pt:"combine,!single"` // for slices, maps, etc.  The default is to combine
	Desc    *bool  `pt:"desc"`            // descend if somewhat filled already?
}

// Len is intersting because it returns a func that that returns fillers.  The idea is
// that when you're filling an array or slice, you first need to know how big it is.  That's
// the total size of all the various Fillers combined.  But to fill, it you need to pull
// from one source and then another.  The func that is returned will provide one source
// at a time.  This breaks the semantics of where the individual elements will come from
// if there is a meta tag.  There isn't a good answer for this -- trying to honor the meta
// tag would be really difficult.
func (f *fillerCollection) Len(
	t reflect.Type,
	x fillData,
) (int, func() (*fillerCollection, error)) {
	var total int
	pairs := f.pairs(x.tags, x.meta)
	debugf("fill: Len: %s pairs: %+v", x.name, pairs)
	lengths := make([]int, len(pairs))
	combine := pointer.Value(x.meta.Combine)
	first := pointer.Value(x.meta.First)
	for i, fp := range pairs {
		canLen, ok := fp.Filler.(CanLenFiller)
		if !ok {
			continue
		}
		length, ok := canLen.Len(t, fp.Tag, first, combine)
		if !ok {
			continue
		}
		debugf("fill: Len: filler %s: %d for %s", fp.ForcedTag, length, x.name)
		lengths[i] = length
		total += length
		if !combine {
			break
		}
	}
	var index int
	var done int // resets to zero for each filler
	debugf("fill: Len: total for %s (%s) is %d (%d pairs)", x.name, t, total, len(pairs))
	return total, func() (*fillerCollection, error) {
		for lengths[index] == 0 {
			index++
		}
		key := pairs[index].ForcedTag
		debugf("fill: Len: recurse %s filler %s (%s), %d:%d/%d items", key, x.name, t, index, done, lengths[index])
		filler, err := simpleRecurseFiller(pairs[index].Filler, strconv.Itoa(done))
		if err != nil {
			return nil, err
		}
		done++
		if done >= lengths[index] {
			index++
			done = 0
		}
		return &fillerCollection{
			m:     map[string]Filler{key: filler},
			order: []string{key},
		}, nil
	}
}

func (f *fillerCollection) Keys(t reflect.Type, tagSet reflectutils.TagSet, meta metaFields) []string {
	var all []string
	seen := make(map[string]struct{})
	first := pointer.Value(meta.First)
	combine := pointer.Value(meta.Combine)
	for _, fp := range f.pairs(tagSet, meta) {
		canKey, ok := fp.Filler.(CanKeysFiller)
		if !ok {
			continue
		}
		keys, ok := canKey.Keys(t, fp.Tag, first, combine)
		if !ok {
			continue
		}
		if !combine {
			return keys
		}
		for _, key := range keys {
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				all = append(all, key)
			}
		}
	}
	return all
}

func (r *Request) fill() error {
	v := reflect.ValueOf(r.object).Elem()
	t := v.Type()
	if r.metaTag == "" {
		r.metaTag = r.registry.metaTag
	}
	debug("fill: start fill", t)
	fillers := r.getFillers()
	for _, p := range r.getPrefix() {
		debug("fill: recurse for prefix", p, "from", callers(3))
		var err error
		fillers, err = fillers.Recurse(p, reflect.TypeOf(struct{}{}), reflectutils.TagSet{})
		if err != nil {
			return commonerrors.ConfigurationError(errors.Wrap(err, "request prefix "+p))
		}
	}
	_, err := fillData{
		r:       r,
		name:    "",
		tags:    reflectutils.TagSet{},
		fillers: fillers,
	}.fillStruct(t, v)
	if validator, ok := r.getValidator(); ok {
		err := validator.Struct(r.object)
		if err != nil {
			return commonerrors.ValidationError(errors.Wrap(err, t.String()))
		}
	}
	return err
}

func (x fillData) fillStruct(t reflect.Type, v reflect.Value) (bool, error) {
	var anyFilled bool
	debug("fill: struct", t)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tags := reflectutils.SplitTag(f.Tag).Set()
		debug("fill: field", f.Name, f.Type, f.Tag)
		meta := metaFields{
			First:   pointer.To(true),
			Combine: pointer.To(true),
		}
		err := tags.Get(x.r.metaTag).Fill(&meta)
		if err != nil {
			return false, commonerrors.ProgrammerError(errors.Wrap(err, f.Name))
		}
		if meta.First == nil {
			meta.First = pointer.To(true)
		}
		if meta.Combine == nil {
			meta.Combine = pointer.To(true)
		}
		debugf("fill: parse '%s'(%s), tag '%s' -> {name: %s, first:%v, combine:%v, desc:%v}\n", f.Tag, x.r.registry.metaTag, tags.Get(x.r.registry.metaTag), meta.Name, *meta.First, *meta.Combine, meta.Desc)
		filled, err := fillData{
			r:       x.r,
			name:    f.Name,
			tags:    tags,
			meta:    meta,
			fillers: x.fillers,
		}.recurseFillField(f.Type, v.FieldByIndex(f.Index))
		if filled {
			anyFilled = true
		}
		if err != nil {
			return anyFilled, errors.Wrap(err, f.Name)
		}
	}
	return anyFilled, nil
}

func (f *fillerCollection) SimpleRecurse(name string, t reflect.Type) (*fillerCollection, error) {
	return f.Recurse(name, t, reflectutils.TagSet{})
}

func (f *fillerCollection) Recurse(name string, t reflect.Type, tagSet reflectutils.TagSet) (*fillerCollection, error) {
	f = f.Copy()
	for tagName, filler := range f.m {
		tag := tagSet.Get(tagName)
		recursed, err := recurseFiller(filler, name, tag)
		debug("fill: Recurse", name, t, tagName, tag)
		if err != nil {
			return nil, errors.Wrap(err, tagName)
		}
		f.Add(tagName, recursed)
	}
	return f, nil
}

func (x fillData) recurseFillField(t reflect.Type, v reflect.Value) (bool, error) {
	debug("fill: recurseFillField", x.name, x.meta.Name, t)
	switch x.meta.Name {
	case "-":
		return false, nil
	case "":
		//
	default:
		x.name = x.meta.Name
		debug("fill: fillField setting x.name, value is default", t, x.name)
	}

	debug("fill: fillField recurse", x.name, t, x.tags)
	var err error
	x.fillers, err = x.fillers.Recurse(x.name, t, x.tags)
	if err != nil {
		return false, err
	}
	return x.fillField(t, v)
}

func (x fillData) fillField(t reflect.Type, v reflect.Value) (bool, error) {
	debug("fill: fillField", x.name, x.meta.Name, t)

	var isStructural bool
	switch reflectutils.NonPointer(t).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		isStructural = true
	}

	var anyFilled bool
	combine := pointer.Value(x.meta.Combine)
	first := pointer.Value(x.meta.First)
	debug("fill: pairs next,", x.name, "first:", first, "combine:", combine)
	for _, fp := range x.fillers.pairs(x.tags, x.meta) {
		filled, err := fp.Filler.Fill(t, v, fp.Tag, first, combine)
		debugf("fill: pair filled %s %v %s %s", x.name, filled, fp.Tag, err)
		if err != nil {
			return false, errors.Wrapf(err, "flll %s using %s", x.name, fp.Tag.Tag)
		}
		if filled {
			x.fillers.Remove(fp.Tag.Tag)
			anyFilled = true
			if isStructural && combine {
				continue
			}
			break
		}
	}

	if anyFilled && x.meta.Desc != nil && !*x.meta.Desc {
		return true, nil
	}

	switch t.Kind() {
	case reflect.Struct:
		filled, err := x.fillStruct(t, v)
		return anyFilled || filled, err
	case reflect.Ptr:
		e := reflect.New(t.Elem())
		debugf("fill: t is %s, e is %s", t, e.Elem().Type())
		filled, err := x.fillField(t.Elem(), e.Elem())
		if err != nil {
			return false, err
		}
		if !filled {
			return false, nil
		}
		v.Set(e)
		return true, nil
	case reflect.Array:
		count, recurseInSequence := x.fillers.Len(t, x)
		cap := v.Len()
		elemType := t.Elem()
		for i := 0; i < count && i < cap; i++ {
			var err error
			x.fillers, err = recurseInSequence()
			if err != nil {
				return false, err
			}
			filled, err := x.fillField(elemType, v.Index(i))
			if err != nil {
				return false, err
			}
			if filled {
				anyFilled = true
			}
		}
		return anyFilled, nil
	case reflect.Slice:
		count, recurseInSequence := x.fillers.Len(t, x)
		if count == 0 {
			return false, nil
		}
		var a reflect.Value
		a = reflect.MakeSlice(t, count, count)
		elemType := t.Elem()
		for i := 0; i < count; i++ {
			var err error
			x.fillers, err = recurseInSequence()
			if err != nil {
				return false, err
			}
			debugf("fill slice element %d, name = %s\n", i, x.name)
			filled, err := x.fillField(elemType, a.Index(i))
			if err != nil {
				return false, err
			}
			if filled {
				anyFilled = true
			}
		}
		if v.IsNil() {
			v.Set(a)
		} else {
			v.Set(reflect.AppendSlice(v, a))
		}
		return anyFilled, nil
	case reflect.Map:
		keys := x.fillers.Keys(t, x.tags, x.meta)
		if len(keys) == 0 {
			return anyFilled, nil
		}
		var m reflect.Value
		if v.IsNil() {
			m = reflect.MakeMap(t)
			v.Set(m)
		} else {
			m = v
		}
		f, err := reflectutils.MakeStringSetter(t.Key())
		if err != nil {
			return false, commonerrors.ProgrammerError(errors.Wrapf(err, "set key for %T", t))
		}
		fillers := x.fillers.Copy()
		elemType := t.Elem()
		for _, key := range keys {
			kp := reflect.New(t.Key())
			err := f(kp.Elem(), key)
			if err != nil {
				return false, errors.Wrap(err, "set key")
			}
			vp := reflect.New(elemType)
			debug("fill: recurse for map key", key, "in", x.name)
			x.fillers, err = fillers.SimpleRecurse(key, elemType)
			if err != nil {
				return false, err
			}
			filled, err := x.fillField(elemType, vp.Elem())
			if err != nil {
				return false, errors.Wrap(err, "set value")
			}
			if filled {
				anyFilled = true
			}
			m.SetMapIndex(reflect.Indirect(kp), reflect.Indirect(vp))
		}
		return anyFilled, nil
	default:
		return anyFilled, nil
	}
}

func recurseFiller(filler Filler, name string, tag reflectutils.Tag) (Filler, error) {
	if canRecurse, ok := filler.(CanRecurseFiller); ok {
		if tag.Tag != "" {
			var fileTag fileTag
			err := tag.Fill(&fileTag)
			if err != nil {
				return nil, commonerrors.ProgrammerError(errors.Wrap(err, tag.Tag))
			}
			switch fileTag.Name {
			case "-":
				debug("fill recurseFiller w/", tag.Tag, ": skip")
				return nil, nil
			case "":
				//
				debug("fill recurseFiller w/", tag.Tag, ": keep", name)
			default:
				debug("fill recurseFiller w/", tag.Tag, ": use", fileTag.Name, "instead of", name, "from", callers(6))
				name = fileTag.Name
			}
		}
		return canRecurse.Recurse(name)
	}
	return filler, nil
}

// simpleRecurseFiller does a recursion using name on a single filller
func simpleRecurseFiller(filler Filler, name string) (Filler, error) {
	if canRecurse, ok := filler.(CanRecurseFiller); ok {
		return canRecurse.Recurse(name)
	}
	return filler, nil
}
