package nfigure

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

// Fillers are applied recursively to structures that need
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
	Fill(t reflect.Type, v reflect.Value, tag reflectutils.Tag) (filledAnything bool, err error)

	// PreWalk is called from nfigure.Request only on every known (at that time) configuration
	// struct before any call to Fill()
	PreWalk(tagName string, request *Request, model interface{}) error

	ConfigureComplete() error

	// PreConfigure is called by nfigure.Registry once just before configuration starts
	PreConfigure(tagName string, request *Registry) error

	// for filling maps
	Keys(reflect.Type, reflectutils.Tag) []string
	// for filling arrays & slices
	Len(reflect.Type, reflectutils.Tag) int
	// for source fillers.  nil,nil return means no change
	AddConfigFile(file string, keyPath []string) (Filler, error)
}

type Fillers map[string]Filler

func (f Fillers) Copy() Fillers {
	n := make(Fillers)
	for tag, filler := range f {
		n[tag] = filler
	}
	return n
}

type fillPair struct {
	Filler Filler
	Tag    reflectutils.Tag
	Backup string // because Tag.Tag may be empty
}

func (f Fillers) Pairs(tagSet reflectutils.TagSet) []fillPair {
	pairs := make([]fillPair, 0, len(f))
	done := make(map[string]struct{})
	for _, tag := range tagSet.Tags {
		if filler, ok := f[tag.Tag]; ok {
			pairs = append(pairs, fillPair{
				Filler: filler,
				Tag:    tag,
				Backup: tag.Tag,
			})
			done[tag.Tag] = struct{}{}
		}
	}
	for tag, filler := range f {
		if _, ok := done[tag]; ok {
			continue
		}
		pairs = append(pairs, fillPair{
			Filler: filler,
			Backup: tag,
		})
	}
	return pairs
}

func (f Fillers) Len(t reflect.Type, tagSet reflectutils.TagSet) (int, func() (Fillers, error)) {
	var total int
	pairs := f.Pairs(tagSet)
	lengths := make([]int, len(pairs))
	for i, fp := range pairs {
		length := fp.Filler.Len(t, fp.Tag)
		lengths[i] = length
		total += length
	}
	var index int
	var done int
	return total, func() (Fillers, error) {
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
		return Fillers{key: filler}, nil
	}
}

func (f Fillers) Keys(t reflect.Type, tagSet reflectutils.TagSet) []string {
	var all []string
	seen := make(map[string]struct{})
	for _, fp := range f.Pairs(tagSet) {
		keys := fp.Filler.Keys(t, fp.Tag)
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
			return errors.Wrap(err, "request prefix "+p)
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
			return errors.Wrap(err, t.String())
		}
	}
	return err
}

type fillData struct {
	r       *Request
	name    string
	tags    reflectutils.TagSet
	metaTag metaTag
	fillers Fillers
}

type metaTag struct {
	Name  string `pt:"0"`
	First *bool  `pt:"priority"` // "first" or "all"
	Desc  *bool  `pt:"desc"`     // descend if somewhat filled already?
}

func (x fillData) fillStruct(t reflect.Type, v reflect.Value) (bool, error) {
	var anyFilled bool
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tags := reflectutils.SplitTag(f.Tag).Set()
		var directive metaTag
		err := tags.Get(x.r.metaTag).Fill(&directive)
		fmt.Printf("XXX parse '%s'(%s), tag '%s' -> %v\n", f.Tag, x.r.registry.metaTag, tags.Get(x.r.registry.metaTag), directive)
		if err != nil {
			return false, errors.Wrap(err, f.Name)
		}
		filled, err := fillData{
			r:       x.r,
			name:    f.Name,
			tags:    tags,
			metaTag: directive,
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

func (fillers Fillers) Recurse(name string, t reflect.Type, tagSet reflectutils.TagSet) (Fillers, error) {
	fillers = fillers.Copy()
	for tag, filler := range fillers {
		f, err := filler.Recurse(name, t, tagSet.Get(tag))
		if err != nil {
			return nil, errors.Wrap(err, tag)
		}
		if f == nil {
			delete(fillers, tag)
			continue
		}
		fillers[tag] = f
	}
	return fillers, nil
}

func (x fillData) fillField(t reflect.Type, v reflect.Value) (bool, error) {
	switch x.metaTag.Name {
	case "-":
		return false, nil
	case "":
		//
	default:
		fmt.Println("XXX x.name", x.name, "->", x.metaTag.Name)
		x.name = x.metaTag.Name
	}

	var err error
	x.fillers, err = x.fillers.Recurse(x.name, t, x.tags)
	if err != nil {
		return false, err
	}
	fmt.Println("XXX x.name", x.name)

	var anyFilled bool
	for _, fp := range x.fillers.Pairs(x.tags) {
		filled, err := fp.Filler.Fill(t, v, fp.Tag)
		fmt.Println("XXX FP filled", fp.Tag, filled, err)
		if err != nil {
			return false, err
		}
		if filled {
			anyFilled = true
			if x.metaTag.First != nil && *x.metaTag.First {
				break
			}
		}
	}

	if anyFilled && x.metaTag.Desc != nil && !*x.metaTag.Desc {
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
			fmt.Println("XXX ptr filling err", err)
			return false, err
		}
		if !filled {
			fmt.Println("XXX ptr not filled")
			return false, nil
		}
		fmt.Println("XXX filled ptr")
		v.Set(e)
		return true, nil
	case reflect.Array:
		count, recurseInSequence := x.fillers.Len(t, x.tags)
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
		count, recurseInSequence := x.fillers.Len(t, x.tags)
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
		keys := x.fillers.Keys(t, x.tags)
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
			return false, errors.Wrapf(err, "set key for %T", t)
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
