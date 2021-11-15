package nflex

import (
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type parsedYAML struct {
	root *yaml.Node
	// nodes store maps as alternating key/value in an array
	// cache turns that into a map
	cache map[*yaml.Node]map[string]*yaml.Node

	pathToHere []string
}

func UnmarshalYAML(data []byte) (Source, error) {
	var node yaml.Node
	err := yaml.Unmarshal(data, &node)
	if err != nil {
		return parsedYAML{}, errors.Wrap(err, "yaml")
	}
	return parsedYAML{
		root:  &node,
		cache: make(map[*yaml.Node]map[string]*yaml.Node),
	}, nil
}

func (p parsedYAML) Exists(keys ...string) bool {
	n, err := p.lookup(p.root, keys)
	if err != nil || n == nil {
		return false
	}
	return true
}

func (p parsedYAML) Recurse(keys ...string) Source {
	if len(keys) == 0 {
		return p
	}
	n, err := p.lookup(p.root, keys)
	if err != nil || n == nil {
		return nil
	}
	return parsedYAML{
		root:       n.root,
		cache:      p.cache,
		pathToHere: combine(p.pathToHere, keys),
	}
}

func (p parsedYAML) GetBool(keys ...string) (bool, error) {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return false, err
	}
	b, err := strconv.ParseBool(n.Value)
	if err != nil {
		return false, errors.Wrapf(ErrWrongType, "Lookup %v, parse error: %s", combine(p.pathToHere, keys), err)
	}
	return b, nil
}

func (p parsedYAML) GetInt(keys ...string) (int64, error) {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseInt(n.Value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(ErrWrongType, "Lookup %v, parse error: %s", combine(p.pathToHere, keys), err)
	}
	return i, nil
}

func (p parsedYAML) GetUInt(keys ...string) (uint64, error) {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseUint(n.Value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(ErrWrongType, "Lookup %v, parse error: %s", combine(p.pathToHere, keys), err)
	}
	return i, nil
}

func (p parsedYAML) GetFloat(keys ...string) (float64, error) {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(n.Value, 64)
	if err != nil {
		return 0, errors.Wrapf(ErrWrongType, "Lookup %v, parse error: %s", combine(p.pathToHere, keys), err)
	}
	return f, nil
}

func (p parsedYAML) GetString(keys ...string) (string, error) {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return "", err
	}
	return n.Value, nil
}

var boolRE = regexp.MustCompile(`^(?:true|True|TRUE|false|False|FALSE|t|T|f|F)$`)
var intRE = regexp.MustCompile(`^\d+$`)
var floatRE = regexp.MustCompile(`^(?:(?:\.\d+(?:[eE][-+]?\d+)?)|(?:\d+(?:\.\d+(?:[eE][-+]?\d+)?)?))$`)

func (p parsedYAML) Type(keys ...string) NodeType {
	n, err := p.lookupScalar(keys)
	if err != nil {
		return Undefined
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.MappingNode:
		return Map
	case yaml.SequenceNode:
		return Slice
	case yaml.ScalarNode:
		if boolRE.MatchString(n.Value) {
			return Bool
		}
		if intRE.MatchString(n.Value) {
			return Int
		}
		if floatRE.MatchString(n.Value) {
			return Float
		}
		return String
	default:
		return Undefined // this shouldn't happen
	}
}

func (p parsedYAML) Len(keys ...string) (int, error) {
	n, err := p.lookup(p.root, keys)
	if err != nil {
		return 0, errors.Wrapf(ErrDoesNotExist, "Could not get %v: %s", combine(p.pathToHere, keys), err)
	}
	if n == nil {
		return 0, errors.Wrapf(ErrDoesNotExist, "Could not get %v", combine(p.pathToHere, keys))
	}
	if n.root.Kind != yaml.SequenceNode {
		return 0, errors.Wrapf(ErrWrongType, "Len %s is a %d", combine(p.pathToHere, keys), n.root.Kind)
	}
	return len(n.root.Content), nil
}

func (p parsedYAML) Keys(keys ...string) ([]string, error) {
	n, err := p.lookup(p.root, keys)
	if err != nil {
		return nil, errors.Wrapf(ErrDoesNotExist, "Could not get %v: %s", combine(p.pathToHere, keys), err)
	}
	if n == nil {
		return nil, errors.Wrapf(ErrDoesNotExist, "Could not get %v", combine(p.pathToHere, keys))
	}
	if n.root.Kind != yaml.MappingNode {
		return nil, errors.Wrapf(ErrWrongType, "Len %s is a %d", combine(p.pathToHere, keys), n.root.Kind)
	}
	ret := make([]string, len(n.root.Content)/2)
	for i := 0; i < len(n.root.Content); i += 2 {
		ret[i/2] = n.root.Content[i].Value
	}
	return ret, nil
}

func (p parsedYAML) lookupScalar(keys []string) (*yaml.Node, error) {
	n, err := p.lookup(p.root, keys)
	if err != nil {
		return nil, errors.Wrapf(ErrDoesNotExist, "Could not get %v: %s", combine(p.pathToHere, keys), err)
	}
	if n == nil {
		return nil, errors.Wrapf(ErrDoesNotExist, "Could not get %v", combine(p.pathToHere, keys))
	}
	if n.root.Kind != yaml.ScalarNode {
		return nil, errors.Wrapf(ErrWrongType, "Lookup %v is a %d", combine(p.pathToHere, keys), n.root.Kind)
	}
	return n.root, nil
}

var isNumberRE = regexp.MustCompile(`^[0-9]+$`)

func (p parsedYAML) lookup(n *yaml.Node, keys []string) (*parsedYAML, error) {
	original := keys
	for len(keys) > 0 {
		key := keys[0]
		if n == nil {
			return nil, nil
		}
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) == 0 {
				// empty document, move right along folks
				return nil, nil
			}
			n = n.Content[0]
			continue
		case yaml.SequenceNode:
			if !isNumberRE.MatchString(key) {
				return nil, errors.Errorf("cannot use '%s' as an array index", combine(p.pathToHere, original))
			}
			i, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parse array index")
			}
			if int(i) >= len(n.Content) {
				return nil, nil
			}
			n = n.Content[i]
		case yaml.MappingNode:
			if cache, ok := p.cache[n]; !ok {
				cache = make(map[string]*yaml.Node)
				p.cache[n] = cache
				if len(n.Content)%2 != 0 {
					return nil, errors.Errorf("mapping node %s/%s has non-even content", n.Tag, n.Anchor)
				}
				for i := 0; i < len(n.Content); i += 2 {
					cache[n.Content[i].Value] = n.Content[i+1]
				}
			}
			n = p.cache[n][key]
		case yaml.ScalarNode:
			return nil, errors.Errorf("Cannot index through scalar with '%s'", combine(p.pathToHere, original))
		case yaml.AliasNode:
			n = n.Alias
			continue
		}
		keys = keys[1:]
	}
	if n == nil { return nil, nil }
	return &parsedYAML{
		root:       n,
		cache:      p.cache,
		pathToHere: combine(p.pathToHere, original),
	}, nil
}
