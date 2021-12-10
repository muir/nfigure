package nfigure

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
	"go.octolab.org/pointer"
)

// Filler s are applied recursively to structures that need
// to be filled.
type Filler interface {
	// Recurse is called during the filling process to indicate
	// that we are now filling a sub-struct, array element, map
	// element etc.
	//
	// If the filler knows that with the recursion it will no longer
	// try to fill anything, it can return nil for it's replacment
	// filler.
	Recurse(structName string, t reflect.Type, tag reflectutils.Tag) (Filler, error)

	// Fill is what's used to populate data in a configuration struct.
	// Fillers can choose: they can fill structs, maps, arrays, slices,
	// and pointers or they can wait for Recurse to be called and then
	// Fill to be called on slice items and struct fields and map values.
	// Map keys must come from Keys() or the struct as a whole.
	Fill(t reflect.Type, v reflect.Value, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (filledAnything bool, err error)

	// for filling maps
	Keys(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) ([]string, bool)
	// for filling arrays & slices
	Len(t reflect.Type, tag reflectutils.Tag, firstFirst bool, combineObjects bool) (int, bool)
}

type CanPreWalk interface {
	Filler
	// PreWalk is called from nfigure.Request only on every known (at that time) configuration
	// struct before any call to Fill()
	PreWalk(tagName string, request *Request, model interface{}) error
}

type CanConfigureComplete interface {
	Filler
	// ConfigureComplete is called by Registry.Configure() when all configuration is complete.
	// This is currently skipped for Fillers that are subcommand specific.
	ConfigureComplete() error
}

type CanPreConfigure interface {
	Filler
	// PreConfigure is called by nfigure.Registry once just before configuration starts
	PreConfigure(tagName string, request *Registry) error
}

type CanAddConfigFile interface {
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
	tagSet reflectutils.TagSet,
	meta metaFields,
) (int, func() (*fillerCollection, error)) {
	var total int
	pairs := f.pairs(tagSet, meta)
	lengths := make([]int, len(pairs))
	combine := pointer.ValueOfBool(meta.Combine)
	first := pointer.ValueOfBool(meta.First)
	for i, fp := range pairs {
		length, ok := fp.Filler.Len(t, fp.Tag, first, combine)
		if !ok {
			continue
		}
		lengths[i] = length
		total += length
		if !combine {
			break
		}
	}
	var index int
	var done int
	return total, func() (*fillerCollection, error) {
		for lengths[index] == 0 {
			index++
		}
		tag := pairs[index].Tag
		key := pairs[index].Backup
		filler, err := pairs[index].Filler.Recurse(strconv.Itoa(done), t, tag)
		if err != nil {
			return nil, err
		}
		done++
		if done > lengths[index] {
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
	first := pointer.ValueOfBool(meta.First)
	combine := pointer.ValueOfBool(meta.Combine)
	for _, fp := range f.pairs(tagSet, meta) {
		keys, ok := fp.Filler.Keys(t, fp.Tag, first, combine)
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
	fillers := r.getFillers()
	for _, p := range r.getPrefix() {
		var err error
		fillers, err = fillers.Recurse(p, reflect.TypeOf(struct{}{}), reflectutils.TagSet{})
		if err != nil {
			return ConfigurationError(errors.Wrap(err, "request prefix "+p))
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
			return ValidationError(errors.Wrap(err, t.String()))
		}
	}
	return err
}

func (x fillData) fillStruct(t reflect.Type, v reflect.Value) (bool, error) {
	var anyFilled bool
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tags := reflectutils.SplitTag(f.Tag).Set()
		meta := metaFields{
			First:   pointer.ToBool(true),
			Combine: pointer.ToBool(true),
		}
		err := tags.Get(x.r.metaTag).Fill(&meta)
		if err != nil {
			return false, ProgrammerError(errors.Wrap(err, f.Name))
		}
		if meta.First == nil {
			meta.First = pointer.ToBool(true)
		}
		if meta.Combine == nil {
			meta.Combine = pointer.ToBool(true)
		}
		fmt.Printf("XXX parse '%s'(%s), tag '%s' -> %v\n", f.Tag, x.r.registry.metaTag, tags.Get(x.r.registry.metaTag), meta)
		filled, err := fillData{
			r:       x.r,
			name:    f.Name,
			tags:    tags,
			meta:    meta,
			fillers: x.fillers,
		}.fillField(f.Type, v.FieldByIndex(f.Index))
		if filled {
			anyFilled = true
		}
		if err != nil {
			return anyFilled, errors.Wrap(err, f.Name)
		}
	}
	return anyFilled, nil
}

func (fillers *fillerCollection) Recurse(name string, t reflect.Type, tagSet reflectutils.TagSet) (*fillerCollection, error) {
	fillers = fillers.Copy()
	for tag, filler := range fillers.m {
		f, err := filler.Recurse(name, t, tagSet.Get(tag))
		if err != nil {
			return nil, errors.Wrap(err, tag)
		}
		fillers.Add(tag, f)
	}
	return fillers, nil
}

func (x fillData) fillField(t reflect.Type, v reflect.Value) (bool, error) {
	switch x.meta.Name {
	case "-":
		return false, nil
	case "":
		//
	default:
		x.name = x.meta.Name
	}

	var err error
	x.fillers, err = x.fillers.Recurse(x.name, t, x.tags)
	if err != nil {
		return false, err
	}

	var isStructural bool
	switch reflectutils.NonPointer(t).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		isStructural = true
	}

	var anyFilled bool
	combine := pointer.ValueOfBool(x.meta.Combine)
	first := pointer.ValueOfBool(x.meta.First)
	for _, fp := range x.fillers.pairs(x.tags, x.meta) {
		filled, err := fp.Filler.Fill(t, v, fp.Tag, first, combine)
		fmt.Println("XXX FP filled", x.name, filled, fp.Tag, err)
		if err != nil {
			return false, errors.Wrapf(err, "flll %s using %s", x.name, fp.Tag.Tag)
		}
		if filled {
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
		fmt.Println("XXX t is", t, "e is", e.Elem().Type())
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
		count, recurseInSequence := x.fillers.Len(t, x.tags, x.meta)
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
		count, recurseInSequence := x.fillers.Len(t, x.tags, x.meta)
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
		f, err := reflectutils.MakeStringSetter(reflect.PtrTo(t.Key()))
		if err != nil {
			return false, ProgrammerError(errors.Wrapf(err, "set key for %T", t))
		}
		fillers := x.fillers.Copy()
		elemType := t.Elem()
		for _, key := range keys {
			kp := reflect.New(t.Key())
			err := f(kp, key)
			if err != nil {
				return false, errors.Wrap(err, "set key")
			}
			vp := reflect.New(elemType)
			x.fillers, err = fillers.Recurse(key, elemType, x.tags)
			if err != nil {
				return false, err
			}
			filled, err := x.fillField(elemType, vp)
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
