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
	require.NoError(t, os.Setenv("GG", "33"), "set G")
	require.NoError(t, os.Setenv("HH", "34"), "set H")
	require.NoError(t, os.Setenv("II", "54"), "set I")
	type testData struct {
		GG int `env:"GG" flag:"GG"        meta:",first"`
		HH int `env:"HH" flag:"HH"        meta:",last"`
		II int `env:"II"          nf:"II" meta:",last"`
		JJ int `                  nf:"jj" meta:",first"`
		MM struct {
			OO string
		}
		NNx struct {
			PP string
		} `nf:"NN"`
	}
	var got testData
	want := testData{
		GG: 33, // from env (first)
		HH: 14, // from flags (last)
		II: 30, // from source2.yaml (last)
		JJ: 12, // from source.yaml (first)
		MM: struct{ OO string }{
			OO: "source.yaml",
		},
		NNx: struct{ PP string }{
			PP: "s.y",
		},
	}
	os.Args = strings.Split("pgrm --GG 13 --HH 14", " ")
	var called int
	fh := PosixFlagHandler(OnStart(func(args []string) {
		assert.Equal(t, ([]string)(nil), args, "remaining args")
		called++
	}))
	registry := NewRegistry(
		WithFiller("source", nil),
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
	t.Logf("got %+v", got)
	assert.Equal(t, want, got, "config results")
	assert.Equal(t, 1, called, "called")
}
