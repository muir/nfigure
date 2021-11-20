package nflex

import (
	"github.com/pkg/errors"
)

var _ CanMutate = &MultiSource{}

type MultiSource struct {
	sources []Source
	first   bool
	combine bool
}

func (m *MultiSource) Copy() *MultiSource {
	n := make([]Source, len(m.sources))
	copy(n, m.sources)
	return &MultiSource{
		first:   m.first,
		combine: m.combine,
		sources: n,
	}
}

func NewMultiSource(sources ...Source) *MultiSource {
	if len(sources) == 0 {
		return &MultiSource{
			first:   true,
			combine: true,
		}
	}
	if m, ok := sources[0].(*MultiSource); ok {
		m = m.Copy()
		m.sources = append(m.sources, sources[1:]...)
		return m
	}
	return &MultiSource{
		first:   true,
		combine: true,
		sources: sources,
	}
}

func (m *MultiSource) Mutate(mutation Mutation) Source {
	n := &MultiSource{
		first:   m.first,
		combine: m.combine,
		sources: make([]Source, len(m.sources)),
	}
	for i, source := range m.sources {
		n.sources[i] = mutation.Apply(source)
	}
	return n
}

// MultiSourceSetFirst sets the priority for which source is
// evaluated first for MultiSource sources.  For scalars (int,
// string, etc), the first evaluated source that has a value is the value returned.
// The default is first=true which means that the first (not
// last) source wins.
func MultiSourceSetFirst(first bool) Mutation {
	return func(source Source) Source {
		if m, ok := source.(*MultiSource); ok {
			if m.first == first {
				return source
			}
			c := m.Copy()
			c.first = first
			return c
		}
		return source
	}
}

// MultiSourceSetCombine sets the behvior for which how many sources
// are used for slices and maps in a MultiSource.  The default
// is true: slices and maps are combined.  Slices are appended and
// maps are combined.
//
// If combine is false there can be some surprising behavior because
// paths may exist beyond what keys says.  For example, suppose
// we have two objects:
//
//	one:
//		map:
//			key1: value1
//	two:
//		map:
//			key2: value2
//
// With combine=false, keys(map) = [key1] but lookup(map.key2) = value2
func MultiSourceSetCombine(combine bool) Mutation {
	return func(source Source) Source {
		if m, ok := source.(*MultiSource); ok {
			if m.combine == combine {
				return source
			}
			c := m.Copy()
			c.combine = combine
			return c
		}
		return source
	}
}

// CombineSources expects any MultiSource to be the first source
// provided.  Nil sources are allowed and filtered out.  The result
// may be nil if all inputs are nil.
func CombineSources(sources ...Source) Source {
	notNil := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			notNil = append(notNil, source)
		}
	}
	switch len(notNil) {
	case 0:
		return nil
	case 1:
		return notNil[0]
	default:
		return NewMultiSource(notNil...)
	}
}

func (m *MultiSource) AddSource(source Source) {
	m.sources = append(m.sources, source)
}

func (m *MultiSource) Recurse(keys ...string) Source {
	n := make([]Source, 0, len(m.sources))
	for _, source := range m.sources {
		r := source.Recurse(keys...)
		if r != nil {
			n = append(n, r)
		}
	}
	if len(n) == 0 {
		return nil
	}
	return &MultiSource{
		first:   m.first,
		combine: m.combine,
		sources: n,
	}
}

// find doesn't guarantee that something exists
func (m *MultiSource) find(keys []string) (Source, bool) {
	switch len(m.sources) {
	case 0:
		return nil, false
	case 1:
		return m.sources[0], true
	}
	if m.first {
		for _, source := range m.sources {
			if source.Exists(keys...) {
				return source, true
			}
		}
	} else {
		for i := len(m.sources) - 1; i >= 0; i-- {
			source := m.sources[i]
			if source.Exists(keys...) {
				return source, true
			}
		}
	}
	return nil, false
}

func (m *MultiSource) Exists(keys ...string) bool {
	_, ok := m.find(keys)
	return ok
}

func (m *MultiSource) GetBool(keys ...string) (bool, error) {
	if source, ok := m.find(keys); ok {
		return source.GetBool(keys...)
	}
	return false, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetInt(keys ...string) (int64, error) {
	if source, ok := m.find(keys); ok {
		return source.GetInt(keys...)
	}
	return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetFloat(keys ...string) (float64, error) {
	if source, ok := m.find(keys); ok {
		return source.GetFloat(keys...)
	}
	return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetString(keys ...string) (string, error) {
	if source, ok := m.find(keys); ok {
		return source.GetString(keys...)
	}
	return "", errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) Type(keys ...string) NodeType {
	if source, ok := m.find(keys); ok {
		return source.Type(keys...)
	}
	return Undefined
}

func (m *MultiSource) Keys(keys ...string) ([]string, error) {
	if len(m.sources) == 1 {
		return m.sources[0].Keys(keys...)
	}
	if !m.combine {
		if source, ok := m.find(keys); ok {
			return source.Keys(keys...)
		}
		return nil, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	results := make([][]string, len(m.sources))
	var total int
	var able int
	for i, source := range m.sources {
		if !source.Exists(keys...) {
			continue
		}
		found, err := source.Keys(keys...)
		if err != nil {
			return nil, err
		}
		results[i] = found
		total += len(found)
		able++
	}
	if able == 0 {
		return nil, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	combined := make([]string, 0, total)
	seen := make(map[string]struct{})
	for _, res := range results {
		for _, key := range res {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			combined = append(combined, key)
		}
	}
	return combined, nil
}

func (m *MultiSource) Len(keys ...string) (int, error) {
	if len(m.sources) == 1 {
		return m.sources[0].Len(keys...)
	}
	if !m.combine {
		if source, ok := m.find(keys); ok {
			return source.Len(keys...)
		}
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	var able int
	var total int
	for _, source := range m.sources {
		if !source.Exists(keys...) {
			continue
		}
		l, err := source.Len(keys...)
		if err != nil {
			return 0, err
		}
		total += l
		able++
	}
	if able == 0 {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	return total, nil
}
