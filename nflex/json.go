package nflex

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

type parsedJSON struct {
	value      *fastjson.Value
	pathToHere []string
}

func UnmarshalJSON(data []byte) (Source, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "json")
	}
	return parsedJSON{
		value: value,
	}, nil
}

func (p parsedJSON) Exists(key ...string) bool {
	v := p.value.Get(key...)
	if v == nil {
		return false
	}
	return true
}

func (p parsedJSON) Recurse(key ...string) Source {
	if len(key) == 0 {
		return p
	}
	v := p.value.Get(key...)
	if v == nil {
		return nil
	}
	return parsedJSON{
		value:      v,
		pathToHere: combine(p.pathToHere, key),
	}
}

func (p parsedJSON) GetBool(key ...string) (bool, error) {
	v := p.value.Get(key...)
	if v == nil {
		return false, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeTrue:
		return true, nil
	case fastjson.TypeFalse:
		return false, nil
	default:
		return false, errors.Wrapf(ErrWrongType, "key %v is a %s (not a boolean)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) GetInt(key ...string) (int64, error) {
	v := p.value.Get(key...)
	if v == nil {
		return 0, errors.Errorf("key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeString:
		i, err := strconv.ParseInt(string(v.String()), 10, 64)
		if err != nil {
			return 0, errors.Wrapf(ErrWrongType, "parse int '%s' at %v: %s", string(v.String()), combine(p.pathToHere, key), err)
		}
		return i, nil
	case fastjson.TypeNumber:
		return v.GetInt64(), nil
	default:
		return 0, errors.Wrapf(ErrWrongType, "key %v is a %s (not a number)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) GetUInt(key ...string) (uint64, error) {
	v := p.value.Get(key...)
	if v == nil {
		return 0, errors.Errorf("key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeString:
		i, err := strconv.ParseUint(string(v.String()), 10, 64)
		if err != nil {
			return 0, errors.Wrapf(ErrWrongType, "parse int '%s' at %v: %s", string(v.String()), combine(p.pathToHere, key), err)
		}
		return i, nil
	case fastjson.TypeNumber:
		return v.GetUint64(), nil
	default:
		return 0, errors.Wrapf(ErrWrongType, "key %v is a %s (not a number)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) GetFloat(key ...string) (float64, error) {
	v := p.value.Get(key...)
	if v == nil {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeNumber:
		return v.GetFloat64(), nil
	default:
		return 0, errors.Wrapf(ErrWrongType, "key %v is a %s (not a number)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) GetString(key ...string) (string, error) {
	v := p.value.Get(key...)
	if v == nil {
		return "", errors.Wrapf(ErrDoesNotExist, "key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeString:
		return string(v.GetStringBytes()), nil
	default:
		return "", errors.Wrapf(ErrWrongType, "key %v is a %s (not a string)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) Keys(key ...string) ([]string, error) {
	v := p.value.Get(key...)
	if v == nil {
		return nil, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeNull:
		return nil, nil
	case fastjson.TypeObject:
		o := v.GetObject()
		if o == nil {
			return nil, errors.New("internal error: expected a non-nil object")
		}
		keys := make([]string, 0, o.Len())
		o.Visit(func(key []byte, _ *fastjson.Value) {
			keys = append(keys, string(key))
		})
		return keys, nil
	default:
		return nil, errors.Wrapf(ErrWrongType, "key %v is a %s (not a string)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) Len(key ...string) (int, error) {
	v := p.value.Get(key...)
	if v == nil {
		return 0, errors.Wrapf(ErrDoesNotExist, "key %v does not exist", combine(p.pathToHere, key))
	}
	switch v.Type() {
	case fastjson.TypeNull:
		return 0, nil
	case fastjson.TypeArray:
		a := v.GetArray()
		return len(a), nil
	default:
		return 0, errors.Wrapf(ErrWrongType, "key %v is a %s (not a string)", combine(p.pathToHere, key), v.Type())
	}
}

func (p parsedJSON) Type(key ...string) NodeType {
	v := p.value.Get(key...)
	if v == nil {
		return Undefined
	}
	switch v.Type() {
	case fastjson.TypeNull:
		return Nil
	case fastjson.TypeObject:
		return Map
	case fastjson.TypeArray:
		return Slice
	case fastjson.TypeString:
		return String
	case fastjson.TypeNumber:
		if intRE.MatchString(v.String()) {
			return Int
		}
		return Float
	case fastjson.TypeTrue, fastjson.TypeFalse:
		return Bool
	default:
		return Undefined
	}
}
