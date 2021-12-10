package nfigure

import (
	"go.octolab.org/pointer"
	"github.com/muir/reflectutils"
)

type fillerCollection struct {
	m     map[string]Filler
	order []string
	dirty bool
}

type fillPair struct {
	Filler Filler
	Tag    reflectutils.Tag
	Backup string // because Tag.Tag may be empty
}

// Copy makes shallow copy.
func (f fillerCollection) Copy() *fillerCollection {
	n := fillerCollection{
		m:     make(map[string]Filler),
		order: make([]string, 0, len(f.m)),
	}
	for tag, filler := range f.m {
		n.m[tag] = filler
	}
	f.Clean()
	copy(n.order, f.order)
	return &n
}

func (f *fillerCollection) IsEmpty() bool {
	if f == nil { return true } 
	return len(f.m) == 0
}

func (f *fillerCollection) Order() []string {
	f.Clean()
	return f.order
}

func (f *fillerCollection) Add(tag string, filler Filler) {
	if filler == nil {
		if _, ok := f.m[tag]; ok {
			f.dirty = true
			delete(f.m, tag)
		}
	} else {
		if _, ok := f.m[tag]; !ok {
			f.order = append(f.order, tag)
		}
		if f.m == nil { f.m = make(map[string]Filler) }
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
}

func (f *fillerCollection) pairs(tagSet reflectutils.TagSet, meta metaFields) []fillPair {
	pairs := make([]fillPair, 0, len(f.m))
	done := make(map[string]struct{})
	p := func(tag reflectutils.Tag) {
		if filler, ok := f.m[tag.Tag]; ok {
			pairs = append(pairs, fillPair{
				Filler: filler,
				Tag:    tag,
				Backup: tag.Tag,
			})
			done[tag.Tag] = struct{}{}
		}
	}
	if pointer.ValueOfBool(meta.First) {
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
			continue
		}
		pairs = append(pairs, fillPair{
			Filler: filler,
			Backup: tag,
		})
	}
	return pairs
}
