package nflex

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed *.json *.yaml
var content embed.FS

func TestJSON(t *testing.T) {
	s, err := UnmarshalFile("common.json", WithFS(content))
	require.NoError(t, err, "open common.json")
	checkList(t, stds, s, "a", "b")
	checkList(t, nodup, s, "a", "b")
}

func TestYAML(t *testing.T) {
	s, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	checkList(t, stds, s, "a", "b")
	checkList(t, nodup, s, "a", "b")
}

func TestRecurse1(t *testing.T) {
	s, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s = s.Recurse("a")
	checkList(t, stds, s, "b")
	checkList(t, nodup, s, "b")
}

func TestRecurse2(t *testing.T) {
	s, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s = s.Recurse("a", "b")
	checkList(t, stds, s)
	checkList(t, nodup, s)
}

func TestPrefix(t *testing.T) {
	s, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s = s.Recurse("a", "b")
	s = NewPrefixSource(s, "a", "b")
	checkList(t, stds, s, "a", "b")
	checkList(t, nodup, s, "a", "b")
}

func TestMultiDuplicateSimple(t *testing.T) {
	j, err := UnmarshalFile("common.json", WithFS(content))
	require.NoError(t, err, "open common.json")
	y, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s := NewMultiSource(j, y)
	checkList(t, stds, s, "a", "b")
	checkList(t,
		[]expectation{
			{
				path: []string{"b", "a"},
				cmd:  "len",
				want: 4,
			},
			{
				path: []string{"b", "d"},
				cmd:  "keys",
				want: []string{"j", "y"},
			},
			{
				path: []string{"b", "d", "j"},
				cmd:  "string",
				want: "json",
			},
			{
				path: []string{"b", "d", "y"},
				cmd:  "string",
				want: "yaml",
			},
		},
		s, "a")
}

func key(prefix []string, more ...string) []string {
	n := make([]string, len(prefix), len(prefix)+len(more))
	copy(n, prefix)
	n = append(n, more...)
	return n
}

type expectation struct {
	path   []string
	want   interface{}
	cmd    string
	errors bool
	follow []expectation
}

var stds = []expectation{
	{
		path: []string{"i"},
		cmd:  "int",
		want: int64(28),
	},
	{
		path: []string{"f"},
		cmd:  "float",
		want: 34.39,
	},
	{
		path: []string{"s"},
		cmd:  "string",
		want: "foo",
	},
	{
		path: []string{"bt"},
		cmd:  "bool",
		want: true,
	},
	{
		path: []string{"bf"},
		cmd:  "bool",
		want: false,
	},
	{
		path: []string{"bf"},
		cmd:  "exists",
		want: true,
	},
	{
		path: []string{"a", "0"},
		cmd:  "recurse",
		follow: []expectation{
			{
				cmd:  "int",
				want: int64(3),
			},
		},
	},
	{
		path: []string{"a"},
		cmd:  "recurse",
		follow: []expectation{
			{
				path: []string{"1"},
				cmd:  "int",
				want: int64(11),
			},
		},
	},
	{
		path: []string{"m"},
		cmd:  "exists",
		want: true,
	},
	{
		path: []string{"m"},
		cmd:  "keys",
		want: []string{"k1", "k2"},
		follow: []expectation{
			{
				path: []string{"m", "k1"},
				cmd:  "string",
				want: "v1",
			},
		},
	},
	{
		path: []string{"d"},
		cmd:  "exists",
		want: true,
	},
}

var nodup = []expectation{
	{
		path: []string{"a"},
		cmd:  "len",
		want: 2,
	},
}

func checkList(t *testing.T, wants []expectation, s Source, prefix ...string) {
	for _, want := range wants {
		check(t, want, s, prefix...)
	}
}

func check(t *testing.T, want expectation, s Source, prefix ...string) {
	var v interface{}
	var err error
	path := key(prefix, want.path...)
	switch want.cmd {
	case "exists":
		v = s.Exists(path...)
	case "bool":
		v, err = s.GetBool(path...)
	case "int":
		v, err = s.GetInt(path...)
	case "float":
		v, err = s.GetFloat(path...)
	case "string":
		v, err = s.GetString(path...)
	case "recurse":
		s = s.Recurse(path...)
		if want.errors {
			assert.Nilf(t, s, "%s %v", want.cmd, path)
			return
		}
		assert.NotNilf(t, s, "%s %v", want.cmd, path)
		prefix = nil
	case "keys":
		v, err = s.Keys(path...)
	case "len":
		v, err = s.Len(path...)
	case "type":
		v = s.Type(path...)
	default:
		assert.Fail(t, want.cmd, "no such cmd")
	}
	if want.errors {
		assert.NotNil(t, err, "error %s %v", want.cmd, path)
		return
	}
	if !assert.NoErrorf(t, err, "%s %v", want.cmd, path) {
		return
	}
	if want.cmd != "recurse" {
		assert.Equalf(t, want.want, v, "%s %v", want.cmd, path)
	}
	checkList(t, want.follow, s, prefix...)
}
