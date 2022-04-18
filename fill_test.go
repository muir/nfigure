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

type testDataB struct {
	SS map[string]int `n4:"ss" n5:"ss" meta:",combine"`
}

var mixedCases = []struct {
	base      interface{}
	want      interface{}
	cmd       string
	fillers   string
	remaining string
	redact    func(interface{}) interface{}
	files     []string
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
	{
		cmd:  "combine maps",
		base: &testDataB{},
		want: &testDataB{
			SS: map[string]int{
				"a": 328,
				"b": 93,
				"c": 28,
			},
		},
		redact: func(td interface{}) interface{} {
			d := td.(*testDataB)
			return d.SS
		},
		fillers: "n4 n5",
		files:   []string{},
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
			var expectCalled int
			fh := PosixFlagHandler(OnStart(func(args []string) {
				if tc.remaining == "" {
					assert.Equal(t, ([]string)(nil), args, "remaining args")
				} else {
					assert.Equal(t, strings.Split(tc.remaining, " "), args, "remaining args")
				}
				called++
			}))
			filler := func(path string, keys ...string) Filler {
				f, err := NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content))).AddConfigFile(path, keys)
				require.NoError(t, err, path)
				return f
			}
			potentialArgs := map[string]RegistryFuncArg{
				"config":  WithFiller("config", nil),
				"flag":    WithFiller("flag", fh),
				"nf":      WithFiller("nf", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"meta":    WithMetaTag("meta"),
				"nfigure": WithFiller("nfigure", nil),
				"n2":      WithFiller("n2", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"n3":      WithFiller("n3", NewFileFiller(WithUnmarshalOpts(nflex.WithFS(content)))),
				"n4":      WithFiller("n4", filler("source4.yaml")),
				"n5":      WithFiller("n5", filler("source5.yaml")),
				"noenv":   WithFiller("env", nil),
			}
			var args []RegistryFuncArg
			for _, n := range strings.Split(tc.fillers, " ") {
				t.Logf("Enable %s", n)
				a, ok := potentialArgs[n]
				require.Truef(t, ok, "set %s", n)
				args = append(args, a)
				if n == "flag" {
					expectCalled = 1
				}
			}

			registry := NewRegistry(args...)
			files := []string{"source.yaml", "source2.yaml"}
			if tc.files != nil {
				files = tc.files
			}
			for _, file := range files {
				err := registry.ConfigFile(file)
				require.NoErrorf(t, err, "add %s", file)
			}
			require.NoError(t, registry.Request(tc.base), "request")
			t.Log("About to Configure")
			err := registry.Configure()
			require.NoError(t, err, "configure")
			var want interface{}
			var got interface{}
			want, got = tc.want, tc.base
			if tc.redact != nil {
				want, got = tc.redact(tc.want), tc.redact(tc.base)
			}
			t.Logf("got %+v", tc.base)
			assert.Equal(t, want, got, "config results")
			assert.Equal(t, expectCalled, called, "called")
		})
	}
}
