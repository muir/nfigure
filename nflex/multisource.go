package nflex

import (
	"github.com/pkg/errors"
)

var _ Source = &MultiSource{}

type MultiSource struct {
	sources []Source
}

func (m *MultiSource) Copy() *MultiSource {
	n := make([]Source, len(m.sources))
	copy(n, m.sources)
	return &MultiSource{
		sources: n,
	}
}

func NewMultiSource(sources ...Source) *MultiSource {
	if len(sources) == 0 {
		return &MultiSource{}
	}
	if m, ok := sources[0].(*MultiSource); ok {
		m = m.Copy()
		m.sources = append(m.sources, sources[1:]...)
		return m
	}
	return &MultiSource{
		sources: sources,
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
		sources: n,
	}
}

func (m *MultiSource) Exists(keys ...string) bool {
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return true
		}
	}
	return false
}

func (m *MultiSource) GetBool(keys ...string) (bool, error) {
	if len(m.sources) == 1 {
		return m.sources[0].GetBool(keys...)
	}
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return source.GetBool(keys...)
		}
	}
	return false, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetInt(keys ...string) (int64, error) {
	if len(m.sources) == 1 {
		return m.sources[0].GetInt(keys...)
	}
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return source.GetInt(keys...)
		}
	}
	return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetFloat(keys ...string) (float64, error) {
	if len(m.sources) == 1 {
		return m.sources[0].GetFloat(keys...)
	}
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return source.GetFloat(keys...)
		}
	}
	return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) GetString(keys ...string) (string, error) {
	if len(m.sources) == 1 {
		return m.sources[0].GetString(keys...)
	}
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return source.GetString(keys...)
		}
	}
	return "", errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
}

func (m *MultiSource) Type(keys ...string) NodeType {
	if len(m.sources) == 1 {
		return m.sources[0].Type(keys...)
	}
	for _, source := range m.sources {
		if source.Exists(keys...) {
			return source.Type(keys...)
		}
	}
	return Undefined
}

func (m *MultiSource) Keys(keys ...string) ([]string, error) {
	if len(m.sources) == 1 {
		return m.sources[0].Keys(keys...)
	}
	results := make([][]string, len(m.sources))
	var total int
	var able int
	for i, source := range m.sources {
		if source.Exists(keys...) {
			var err error
			results[i], err = source.Keys(keys...)
			if err != nil {
				return nil, err
			}
			total += len(results[i])
			able++
		}
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
	var able int
	var total int
	for _, source := range m.sources {
		if source.Exists(keys...) {
			l, err := source.Len(keys...)
			if err != nil {
				return 0, err
			}
			total += l
			able++
		}
	}
	if able == 0 {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	return total, nil
}
