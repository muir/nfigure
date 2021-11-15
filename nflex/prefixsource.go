package nflex

import (
	"github.com/pkg/errors"
)

var _ Source = &prefixSource{}

type prefixSource struct {
	prefix []string
	source Source
}

// NewPrefixSource wraps a Source such that it applies a path prefix
// to the entire source.
func NewPrefixSource(source Source, prefix ...string) Source {
	if len(prefix) == 0 {
		return source
	}
	return prefixSource{
		prefix: prefix,
		source: source,
	}
}

func (m prefixSource) recurse(keys []string) ([]string, []string, bool) {
	np := m.prefix
	for len(keys) > 0 && len(np) > 0 {
		if keys[0] != np[0] {
			return nil, nil, true
		}
		keys = keys[1:]
		np = np[1:]
	}
	return np, keys, false
}

func (m prefixSource) Recurse(keys ...string) Source {
	if len(keys) == 0 {
		return m
	}
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return nil
	}
	if len(np) == 0 {
		if len(newKeys) == 0 {
			return m.source
		}
		return m.source.Recurse(newKeys...)
	}
	return prefixSource{
		prefix: np,
		source: m.source,
	}
}

func (m prefixSource) Exists(keys ...string) bool {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return false
	}
	if len(np) == 0 {
		return m.source.Exists(newKeys...)
	}
	return true
}

func (m prefixSource) GetBool(keys ...string) (bool, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return false, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.GetBool(newKeys...)
	}
	return false, errors.Wrapf(ErrWrongType, "key %v is an object (not a boolean)", keys)
}

func (m prefixSource) GetInt(keys ...string) (int64, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.GetInt(newKeys...)
	}
	return 0, errors.Wrapf(ErrWrongType, "key %v is an object (not an integer)", keys)
}

func (m prefixSource) GetFloat(keys ...string) (float64, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.GetFloat(newKeys...)
	}
	return 0, errors.Wrapf(ErrWrongType, "key %v is an object (not an float)", keys)
}

func (m prefixSource) GetString(keys ...string) (string, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return "", errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.GetString(newKeys...)
	}
	return "", errors.Wrapf(ErrWrongType, "key %v is an object (not a string)", keys)
}

func (m prefixSource) Keys(keys ...string) ([]string, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return nil, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.Keys(newKeys...)
	}
	return []string{np[0]}, nil
}

func (m prefixSource) Len(keys ...string) (int, error) {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", keys)
	}
	if len(np) == 0 {
		return m.source.Len(newKeys...)
	}
	return 0, errors.Wrapf(ErrWrongType, "key %v is an object (not an array)", keys)
}

func (m prefixSource) Type(keys ...string) NodeType {
	np, newKeys, mismatch := m.recurse(keys)
	if mismatch {
		return Undefined
	}
	if len(np) == 0 {
		return m.source.Type(newKeys...)
	}
	return Map
}
