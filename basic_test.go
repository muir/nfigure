package nfigure

import (
	"embed"
	"os"
	"strings"
	"testing"

	"github.com/muir/nflex"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed *.yaml
var content embed.FS

func TestBasicFile(t *testing.T) {
	var testData struct {
		II int
		JJ int `nf:"jj"`
	}
	registry := NewRegistry(
		WithoutFillers(),
		WithFiller("nf", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
		WithMetaTag("nf"),
		WithFiller("nfigure", nil))
	err := registry.ConfigFile("source.yaml")
	require.NoError(t, err, "add source.yaml")
	require.NoError(t, registry.Request(&testData), "add model")
	err = registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 10, testData.II, "II")
	assert.Equal(t, 12, testData.JJ, "JJ")
}

func TestBasicEnv(t *testing.T) {
	require.NoError(t, os.Setenv("c", "3+4i"), "set c")
	require.NoError(t, os.Setenv("D", "5+6i"), "set D")
	var testData struct {
		C complex128 `env:"c"`
		D complex128
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testData), "add model")
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
	os.Args = strings.Split("program -ijk 33 45 xyz abc", " ")
	fh := PosixFlagHandler(OnStart(func(args []string) {
		assert.Equal(t, []string{"xyz", "abc"}, args, "remaining args")
		called++
	}))
	registry := NewRegistry(WithFiller("flag", fh))
	require.NoError(t, registry.Request(&testData), "add model")
	err := registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 33, testData.I, "i")
	assert.Equal(t, 45, testData.J, "j")
	assert.True(t, testData.K, "k")
	assert.Equal(t, 1, called, "onstart call count")
}

func TestBasicDefaul(t *testing.T) {
	var testData struct {
		C complex128 `default:"3+7i"`
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testData), "add model")
	err := registry.Configure()
	require.NoError(t, err, "configure")
	assert.Equal(t, 3+7i, testData.C, "C")
}

func TestIntrospection(t *testing.T) {
	var testDataA struct {
		C complex128 `default:"3+7i"`
	}
	var testDataB struct {
		I int `default:"4"`
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testDataA), "add model")
	require.NoError(t, registry.Request(&testDataB), "add model")
	requests := registry.GetRequests()
	require.Equal(t, 2, len(requests), "len")
	require.Equal(t, &testDataA, requests[0].GetObject(), "A")
	require.Equal(t, &testDataB, requests[1].GetObject(), "B")
}

func TestThreeArgumentsTwoFields(t *testing.T) {
	t.Setenv("IGNORE_FLAGS", "zero:value")

	var options struct {
		First  int `flag:"first f"`
		Second int `flag:"second s"`
	}

	// Set up os.Args with three arguments - the first is ignored, then two flags for the struct
	os.Args = strings.Split("program --zero 0 --first 10 --second 20", " ")

	registry := NewRegistry(
		WithFiller(
			"flag", PosixFlagHandler(
				WithHelpText(""),
				IgnoreSpecificFlagsFromEnv("IGNORE_FLAGS"))))

	_ = registry.Request(&options)
	err := registry.Configure()
	require.NoError(t, err, "configure")

	// Verify the struct fields were populated correctly
	assert.Equal(t, 10, options.First, "First field")
	assert.Equal(t, 20, options.Second, "Second field")
}

func TestIgnoredBooleanFlag(t *testing.T) {
	t.Setenv("IGNORE_BOOL_FLAGS", "verbose,debug:value")

	var options struct {
		Output string `flag:"output o"`
	}

	// Test that boolean ignored flags don't consume the next argument
	os.Args = strings.Split("program --verbose --output result", " ")

	registry := NewRegistry(
		WithFiller(
			"flag", PosixFlagHandler(
				WithHelpText(""),
				IgnoreSpecificFlagsFromEnv("IGNORE_BOOL_FLAGS"))))

	_ = registry.Request(&options)
	err := registry.Configure()
	require.NoError(t, err, "configure")

	// Verify that --output got "result", not "--output"
	assert.Equal(t, "result", options.Output, "Output field should be set correctly")
}

func TestIgnoredFlagTypes(t *testing.T) {
	t.Setenv("IGNORE_SIMPLE_TYPES", "verbose,debug:bool,config:value")

	var options struct {
		Output string `flag:"output o"`
		Name   string `flag:"name n"`
	}

	// Test command with ignored flags before real flags
	// Note: complex flags like maps/slices should use --flag=value syntax for clarity
	os.Args = strings.Split("program --verbose --debug --config /path/file --output result --name test", " ")

	registry := NewRegistry(
		WithFiller(
			"flag", PosixFlagHandler(
				WithHelpText(""),
				IgnoreSpecificFlagsFromEnv("IGNORE_SIMPLE_TYPES"))))

	_ = registry.Request(&options)
	err := registry.Configure()
	require.NoError(t, err, "configure")

	// Verify that the real flags were processed correctly
	assert.Equal(t, "result", options.Output, "Output field should be set correctly")
	assert.Equal(t, "test", options.Name, "Name field should be set correctly")
}
