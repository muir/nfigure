package nfigure

import (
	"embed"
	"os"
	"strings"
	"testing"

	"github.com/muir/nfigure/nflex"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed *.yaml
var content embed.FS

func TestBasicFile(t *testing.T) {
	var testData struct {
		I int
		J int `nf:"j"`
	}
	registry := NewRegistry(
		WithFiller("nf", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
		WithMetaTag("nf"),
		WithFiller("nfigure", nil))
	err := registry.ConfigFile("source.yaml")
	require.NoError(t, err, "add source.yaml")
	registry.Request(&testData)
	err = registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 10, testData.I, "I")
	assert.Equal(t, 12, testData.J, "J")
}

func TestBasicEnv(t *testing.T) {
	require.NoError(t, os.Setenv("c", "3+4i"), "set c")
	require.NoError(t, os.Setenv("D", "5+6i"), "set D")
	var testData struct {
		C complex128 `env:"c"`
		D complex128
	}
	registry := NewRegistry()
	registry.Request(&testData)
	err := registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 3+4i, testData.C, "C")
	assert.Equal(t, 0+0i, testData.D, "D shouldn't be set")
}

func TestBasicFlags(t *testing.T) {
	var testData struct {
		I int  `flag:"iflag i"`
		J int  `flag:"jflag j"`
		K bool `flag:"k"`
	}
	var called int
	os.Args = strings.Split("-ijk 33 45 xyz abc", " ")
	fh := PosixFlagHandler(OnStart(func(args []string) {
		assert.Equal(t, []string{"xyz", "abc"}, args, "remaining args")
		called++
	}))
	registry := NewRegistry(WithFiller("flag", fh))
	registry.Request(&testData)
	err := registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 33, testData.I, "i")
	assert.Equal(t, 45, testData.J, "j")
	assert.True(t, testData.K, "k")
	assert.Equal(t, 1, called, "onstart call count")
}
