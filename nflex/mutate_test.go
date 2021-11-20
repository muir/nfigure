package nflex

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiFirstNoCombine(t *testing.T) {
	j, err := UnmarshalFile("common.json", WithFS(content))
	require.NoError(t, err, "open common.json")
	y, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s := MultiSourceSetFirst(true).
		Apply(NewMultiSource(j, y))
	s = MultiSourceSetCombine(false).
		Apply(s)
	checkList(t, stds, s, "a", "b")
	checkList(t,
		[]expectation{
			{
				path: []string{"b", "a"},
				cmd:  "len",
				want: 2,
			},
			{
				path: []string{"b", "d"},
				cmd:  "keys",
				want: []string{"j"},
			},
			{
				path: []string{"b", "d", "j"},
				cmd:  "string",
				want: "json",
			},
		},
		s, "a")
}

func TestMultiLastNoCombine(t *testing.T) {
	j, err := UnmarshalFile("common.json", WithFS(content))
	require.NoError(t, err, "open common.json")
	y, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s := MultiSourceSetFirst(false).
		Combine(MultiSourceSetCombine(false)).
		Apply(NewMultiSource(j, y))
	checkList(t, stds, s, "a", "b")
	checkList(t,
		[]expectation{
			{
				path: []string{"b", "a"},
				cmd:  "len",
				want: 2,
			},
			{
				path: []string{"b", "d"},
				cmd:  "keys",
				want: []string{"y"},
			},
			{
				path: []string{"b", "d", "y"},
				cmd:  "string",
				want: "yaml",
			},
		},
		s, "a")
}

func TestMultiDefaults(t *testing.T) {
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
				path: []string{"b", "d", "y"},
				cmd:  "string",
				want: "yaml",
			},
		},
		s, "a")
}

func TestRecursiveApply(t *testing.T) {
	j, err := UnmarshalFile("common.json", WithFS(content))
	require.NoError(t, err, "open common.json")
	y, err := UnmarshalFile("common.yaml", WithFS(content))
	require.NoError(t, err, "open common.yaml")
	s := MultiSourceSetFirst(false).
		Combine(MultiSourceSetCombine(false)).
		Apply(
			NewPrefixSource(
				NewMultiSource(
					NewPrefixSource(j.Recurse("a", "b"), "b"),
					NewPrefixSource(y.Recurse("a", "b"), "b"),
				),
				"a"))
	checkList(t, stds, s, "a", "b")
	checkList(t,
		[]expectation{
			{
				path: []string{"b", "a"},
				cmd:  "len",
				want: 2,
			},
			{
				path: []string{"b", "d"},
				cmd:  "keys",
				want: []string{"y"},
			},
			{
				path: []string{"b", "d", "y"},
				cmd:  "string",
				want: "yaml",
			},
		},
		s, "a")
}
