package nfigure

import (
	"os"
	"strings"
	"testing"

	"github.com/muir/nfigure/nflex"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetaFirstScalar(t *testing.T) {
	require.NoError(t, os.Setenv("G", "33"), "set G")
	require.NoError(t, os.Setenv("H", "34"), "set H")
	require.NoError(t, os.Setenv("I", "54"), "set I")
	type testData struct {
		G int `env:"G" flag:"G"        meta:",first"`
		H int `env:"H" flag:"H"        meta:",last"`
		I int `env:"I"          nf:"I" meta:",last"`
		J int `                 nf:"j" meta:",first"`
	}
	var got testData
	want := testData{
		G: 33, // from env (first)
		H: 14, // from flags (last)
		I: 30, // from source2 (last)
		J: 12, // from source (first)
	}
	os.Args = strings.Split("pgrm -G 13 -H 14", " ")
	var called int
	fh := PosixFlagHandler(OnStart(func(args []string) {
		assert.Equal(t, ([]string)(nil), args, "remaining args")
		called++
	}))
	registry := NewRegistry(
		WithFiller("flag", fh),
		WithFiller("nf", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
		WithMetaTag("meta"),
		WithFiller("nfigure", nil))
	err := registry.ConfigFile("source.yaml")
	require.NoError(t, err, "add source.yaml")
	err = registry.ConfigFile("source2.yaml")
	registry.Request(&got)
	err = registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, want, got, "config results")
	assert.Equal(t, 1, called, "called")
}
