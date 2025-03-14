package nfigure

import (
	"github.com/muir/nfigure/internal/pointer"
	"github.com/muir/reflectutils"
)

type fillerCollection struct {
	m     map[string]Filler
	order []string
	dirty bool
}

// Copy makes shallow copy.
func (f fillerCollection) Copy() *fillerCollection {
	n := fillerCollection{
		m:     make(map[string]Filler),
		order: make([]string, len(f.m)),
	}
	for tag, filler := range f.m {
		n.m[tag] = filler
	}
	f.Clean()
	copy(n.order, f.order)
	debug("fillers: copy, order now", n.order, f)
	return &n
}

func newFillerCollection() *fillerCollection {
	debug("fillers: new", callers(3))
	return &fillerCollection{
		m: make(map[string]Filler),
	}
}

func (f *fillerCollection) IsEmpty() bool {
	if f == nil {
		return true
	}
	return len(f.m) == 0
}

func (f *fillerCollection) Order() []string {
	f.Clean()
	debug("fillers: order:", f.order)
	return f.order
}

func (f *fillerCollection) Remove(tag string) {
	if _, ok := f.m[tag]; ok {
		debug("fillers: REMOVE", tag)
		f.dirty = true
		delete(f.m, tag)
	}
}

func (f *fillerCollection) Add(tag string, filler Filler) {
	if filler == nil {
		f.Remove(tag)
	} else {
		if _, ok := f.m[tag]; !ok {
			f.order = append(f.order, tag)
			debug("fillers: ADD", tag, "order now", f.order)
		}
		f.m[tag] = filler
	}
}

// Build modifies the fillerCollection it receives and returns it
func (f *fillerCollection) Build(tag string, filler Filler) *fillerCollection {
	f.Add(tag, filler)
	return f
}

func (f *fillerCollection) Clean() {
	if !f.dirty {
		debug("fillers: Clean, not dirty")
		return
	}
	f.dirty = false
	for i, tag := range f.order {
		if _, ok := f.m[tag]; ok {
			continue
		}
		n := make([]string, i, len(f.m))
		if i > 0 {
			copy(n, f.order[:i])
		}
		for _, tag := range f.order[i+1:] {
			if _, ok := f.m[tag]; ok {
				n = append(n, tag)
			}
		}
		f.order = n
		break
	}
	debug("fillers: after clean, order is", f.order)
}

type fillPair struct {
	ForcedTag string // because Tag.Tag may be empty
	Tag       reflectutils.Tag
	Filler    Filler
}

func (f *fillerCollection) pairs(tagSet reflectutils.TagSet, meta metaFields) []fillPair {
	debug("fillers: creating pairs", tagSet, "order (for backup) is", f.Order())
	pairs := make([]fillPair, 0, len(f.m))
	done := make(map[string]struct{})
	p := func(tag reflectutils.Tag) {
		if filler, ok := f.m[tag.Tag]; ok {
			debug("fillers: creating pairs, found filler for tag", tag.Tag)
			pairs = append(pairs, fillPair{
				Filler:    filler,
				Tag:       tag,
				ForcedTag: tag.Tag,
			})
			done[tag.Tag] = struct{}{}
		} else {
			debug("fillers, no filler (no pair) for", tag.Tag)
		}
	}
	if pointer.Value(meta.First) {
		for _, tag := range tagSet.Tags {
			p(tag)
		}
	} else {
		for i := len(tagSet.Tags) - 1; i >= 0; i-- {
			p(tagSet.Tags[i])
		}
	}
	for _, tag := range f.Order() {
		if _, ok := done[tag]; ok {
			continue
		}
		filler, ok := f.m[tag]
		if !ok {
			debug("fillers: creating pairs, could not find", tag)
			continue
		}
		debug("fillers: creating pairs, found backup", tag)
		pairs = append(pairs, fillPair{
			Filler:    filler,
			ForcedTag: tag,
		})
	}
	return pairs
}
