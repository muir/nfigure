package nfigure

import (
	"os"
	"strings"
	"testing"

	"github.com/muir/nflex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDataA struct {
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
	QQ []string `nf:"QQ" n2:"RR" n3:"-" meta:",combine"`
}

var mixedCases = []struct {
	base      interface{}
	want      interface{}
	cmd       string
	fillers   string
	remaining string
}{
	{
		cmd:  "empty",
		base: &testDataA{},
		want: &testDataA{
			II: 30, // from source2.yaml (last)
			JJ: 12, // from source.yaml (first)
			MM: struct{ OO string }{
				OO: "source.yaml",
			},
			NNx: struct{ PP string }{
				PP: "s.y",
			},
			QQ: []string{"a", "b", "c", "d", "e", "f"},
		},
		fillers:   "config flag nf meta nfigure noenv",
		remaining: "empty",
	},
	{
		cmd:  "--GG 13 --HH 14",
		base: &testDataA{},
		want: &testDataA{
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
			QQ: []string{"a", "b", "c", "d", "e", "f"},
		},
		fillers: "config flag nf meta nfigure",
	},
	{
		cmd:  "combine qq",
		base: &testDataA{},
		want: &testDataA{
			II: 30, // from source2.yaml (last)
			JJ: 12, // from source.yaml (first)
			MM: struct{ OO string }{
				OO: "source.yaml",
			},
			NNx: struct{ PP string }{
				PP: "s.y",
			},
			// n2 uses RR and that gets the extra letters
			QQ: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"},
		},
		fillers:   "config flag nf n2 meta nfigure noenv",
		remaining: "combine qq",
	},
}

func TestMetaFirstScalar(t *testing.T) {
	require.NoError(t, os.Setenv("GG", "33"), "set GG")
	require.NoError(t, os.Setenv("HH", "34"), "set HH")
	require.NoError(t, os.Setenv("II", "54"), "set II")

	for _, tc := range mixedCases {
		t.Run(tc.cmd, func(t *testing.T) {
			os.Args = append([]string{os.Args[0]}, strings.Split(tc.cmd, " ")...)
			var called int
			fh := PosixFlagHandler(OnStart(func(args []string) {
				if tc.remaining == "" {
					assert.Equal(t, ([]string)(nil), args, "remaining args")
				} else {
					assert.Equal(t, strings.Split(tc.remaining, " "), args, "remaining args")
				}
				called++
			}))
			potentialArgs := map[string]RegistryFuncArg{
				"config":  WithFiller("config", nil),
				"flag":    WithFiller("flag", fh),
				"nf":      WithFiller("nf", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"meta":    WithMetaTag("meta"),
				"nfigure": WithFiller("nfigure", nil),
				"n2":      WithFiller("n2", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"n3":      WithFiller("n3", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"noenv":   WithFiller("env", nil),
			}
			var args []RegistryFuncArg
			for _, n := range strings.Split(tc.fillers, " ") {
				t.Logf("Enable %s", n)
				a, ok := potentialArgs[n]
				require.Truef(t, ok, "set %s", n)
				args = append(args, a)
			}

			registry := NewRegistry(args...)
			err := registry.ConfigFile("source.yaml")
			require.NoError(t, err, "add source.yaml")
			err = registry.ConfigFile("source2.yaml")
			require.NoError(t, err, "add source2.yaml")
			require.NoError(t, registry.Request(tc.base), "request")
			t.Log("About to Configure")
			err = registry.Configure()
			require.NoError(t, err, "configure")
			t.Logf("got %+v", tc.base)
			assert.Equal(t, tc.want, tc.base, "config results")
			assert.Equal(t, 1, called, "called")
		})
	}
}
