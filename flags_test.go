package nfigure

import (
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mohae/deepcopy"
	"github.com/muir/commonerrors"
	"github.com/muir/nfigure/internal/pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flagSet1 struct {
	I   int       `flag:"iflag i" frogfault:"382"`
	C   int       `flag:"counter c,counter"`
	NC  int       `flag:"ncounter,!counter"`
	SA1 [2]string `flag:"sa1,split=comma"`
	SA2 [2]string `flag:"sa2,split="`
}

type flagSet2 struct {
	M map[string]int  `flag:"M,map=prefix"`
	N *map[int]string `flag:"nm,map=prefix"`
}

type flagSet2a struct {
	N *map[int]string `flag:"nm"`
}

type flagSet3 struct {
	P  *int32     `flag:"pflag p"`
	PP ***float64 `flag:"fflag f"`
}

type flagSet4 struct {
	S  []string  `flag:"s,split=none"`
	Ip []*int    `flag:"ip,split=comma"`
	SA [2]string `flag:"sa,split=explode"`
}

type flagSet5 struct {
	O map[complex128]int `flag:"O"`
	P *map[string]bool   `flag:"P,map=explode,split=/"`
}

type flagSet6 struct {
	S    string        `flag:"sflag" default:"xyz"`
	B    bool          `flag:"bflag" default:"true" bbad:"9"`
	I    int           `flag:"iflag" default:"10" ibad:"ten"`
	I64  int64         `flag:"i64flag" default:"20" i64bad:"twenty"`
	UI   uint          `flag:"uiflag" default:"30" uibad:"thrirty"`
	UI64 uint64        `flag:"ui64flag" default:"40" ui64bad:"forty"`
	D    time.Duration `flag:"dflag" default:"30m10s" dbad:"thirty min"`
	C    complex64     `flag:"cflag"`
	BP   *bool         `flag:"bp"`
	CP   *complex128   `flag:"cp"`
}

type importBool struct {
	name string
	dflt bool
	help string
	want bool
}

type importString struct {
	name string
	dflt string
	help string
	want string
}

type flagTestCase struct {
	base           interface{}
	cmd            string
	exportCmd      string
	want           interface{}
	wantExport     interface{}
	exportDefault  string
	exportToo      bool
	remaining      []string
	again          bool // doesn't work with capture or error
	error          string
	exportError    string
	goFlags        bool
	subcommands    map[string]interface{}
	sub            string
	wantSub        interface{}
	capture        string // re-run in sub-process capturing output
	additionalArgs []FlaghandlerOptArg
	importBools    []importBool
	importStrings  []importString
	name           string // defaults to cmd
}

var cases = []flagTestCase{
	{
		base: &flagSet1{},
		cmd:  "-i 3",
		want: &flagSet1{
			I: 3,
		},
		exportCmd: "-iflag 3",
	},
	{
		base:          &flagSet1{},
		exportDefault: "frogfault",
		wantExport: &flagSet1{
			I: 382,
		},
		name: "export int",
	},
	{
		base: &flagSet6{},
		name: "export flagset",
		wantExport: &flagSet6{
			S:    "xyz",
			D:    30*time.Minute + 10*time.Second,
			I:    10,
			I64:  20,
			UI:   30,
			UI64: 40,
			B:    true,
		},
	},
	{
		base: &flagSet6{},
		wantExport: &flagSet6{
			S:    "abc",
			D:    10 * time.Minute,
			I:    11,
			I64:  21,
			UI:   31,
			UI64: 41,
			B:    false,
			C:    289 + 8i,
			BP:   pointer.To(false),
			CP:   pointer.To(6 + 7i),
		},
		exportCmd: "-sflag abc -dflag 10m -iflag 11 -i64flag 21 -uiflag 31 -ui64flag 41 -bflag=false -cflag 289+8i --bp=false --cp=6+7i",
	},
	{
		base:          &flagSet6{},
		name:          "export dbad",
		exportError:   "invalid duration",
		exportDefault: "dbad",
	},
	{
		base:          &flagSet6{},
		name:          "export ibad",
		exportError:   "invalid syntax",
		exportDefault: "ibad",
	},
	{
		base:          &flagSet6{},
		name:          "export i64bad",
		exportError:   "invalid syntax",
		exportDefault: "i64bad",
	},
	{
		base:          &flagSet6{},
		name:          "export ui64bad",
		exportError:   "invalid syntax",
		exportDefault: "ui64bad",
	},
	{
		base:          &flagSet6{},
		name:          "export uibad",
		exportError:   "invalid syntax",
		exportDefault: "uibad",
	},
	{
		base: &flagSet1{},
		cmd:  "--iflag 4",
		want: &flagSet1{
			I: 4,
		},
	},
	{
		goFlags: true,
		base:    &flagSet1{},
		cmd:     "-iflag 9",
		want: &flagSet1{
			I: 9,
		},
	},
	{
		base: &flagSet1{},
		cmd:  "--iflag=5",
		want: &flagSet1{
			I: 5,
		},
	},
	{
		base: &flagSet1{},
		cmd:  "-c -c -c -c",
		want: &flagSet1{
			C: 4,
		},
	},
	{
		base: &flagSet1{},
		cmd:  "-c --counter=10 -c -c",
		want: &flagSet1{
			C: 12,
		},
	},
	{
		base:  &flagSet1{},
		cmd:   "--ncounter --ncounter",
		error: "invalid syntax",
	},
	{
		base: &flagSet1{},
		cmd:  "--sa1=foo,bar",
		want: &flagSet1{
			SA1: [2]string{"foo", "bar"},
		},
		exportToo: true,
	},
	{
		base:  &flagSet1{},
		cmd:   "--sa1=foo --sa1=bar",
		again: true,
		want: &flagSet1{
			SA1: [2]string{"foo", "bar"},
		},
		exportToo: true,
	},
	{
		base: &flagSet1{},
		cmd:  "--sa2=foo --sa2=bar",
		want: &flagSet1{
			SA2: [2]string{"foo", "bar"},
		},
	},
	{
		base: &flagSet2{},
		cmd:  "--Mx=7 --Myz=22",
		want: &flagSet2{
			M: map[string]int{
				"x":  7,
				"yz": 22,
			},
		},
	},
	{
		base: &flagSet2{},
		cmd:  "--nm2=xyz --nm-30=ten",
		want: &flagSet2{
			N: &(map[int]string{
				2:   "xyz",
				-30: "ten",
			}),
		},
		exportCmd: "-nm 2=xyz -nm -30=ten",
	},
	{
		base: &flagSet2a{},
		cmd:  "--nm 2=xyz --nm -30=ten",
		want: &flagSet2a{
			N: &(map[int]string{
				2:   "xyz",
				-30: "ten",
			}),
		},
	},
	{
		base: &flagSet3{},
		cmd:  "--pflag=39",
		want: &flagSet3{
			P: pointer.To(int32(39)),
		},
	},
	{
		base: &flagSet3{},
		cmd:  "-f 99.4",
		want: &flagSet3{
			PP: pointer.To(pointer.To(pointer.To(float64(99.4)))),
		},
	},
	{
		base: &flagSet3{},
		cmd:  "-p 20 foo -i 10 xy",
		want: &flagSet3{
			P: pointer.To(int32(20)),
		},
		subcommands: map[string]interface{}{
			"foo": &flagSet1{},
		},
		sub: "foo",
		wantSub: &flagSet1{
			I: 11,
		},
		remaining: []string{"xy"},
	},
	{
		base:      &flagSet4{},
		cmd:       "-sa foo bar baz",
		exportCmd: "-sa foo -sa bar",
		goFlags:   true,
		want: &flagSet4{
			SA: [2]string{"foo", "bar"},
		},
		remaining: []string{"baz"},
	},
	{
		base:      &flagSet4{},
		cmd:       "-sa fooo baar baaz",
		exportCmd: "-sa fooo -sa baar",
		goFlags:   true,
		want: &flagSet4{
			SA: [2]string{"fooo", "baar"},
		},
		error: "unexpected positional",
		additionalArgs: []FlaghandlerOptArg{
			NoPositional(),
		},
	},
	{
		base:      &flagSet4{},
		cmd:       "-sa fooo baar -- baaz",
		exportCmd: "-sa fooo -sa baar",
		goFlags:   true,
		want: &flagSet4{
			SA: [2]string{"fooo", "baar"},
		},
		error: "unexpected positional",
		additionalArgs: []FlaghandlerOptArg{
			NoPositional(),
		},
	},
	{
		base:      &flagSet4{},
		cmd:       "-s x,x -s y -s z z",
		exportToo: true,
		goFlags:   true,
		want: &flagSet4{
			S: []string{"x,x", "y", "z"},
		},
		remaining: []string{"z"},
	},
	{
		base:    &flagSet4{},
		cmd:     "--ip=7,8",
		goFlags: true,
		want: &flagSet4{
			Ip: []*int{pointer.To(7), pointer.To(8)},
		},
	},
	{
		base:    &flagSet4{},
		cmd:     "--ip 7 --ip=8",
		goFlags: true,
		want: &flagSet4{
			Ip: []*int{pointer.To(7), pointer.To(8)},
		},
	},
	{
		base:    &flagSet4{},
		cmd:     "--ip 7,8",
		goFlags: true,
		want: &flagSet4{
			Ip: []*int{pointer.To(7), pointer.To(8)},
		},
	},
	{
		base: &flagSet5{},
		cmd:  "-P yes/true -P no/false",
		want: &flagSet5{
			P: &(map[string]bool{
				"yes": true,
				"no":  false,
			}),
		},
	},
	{
		base:  &flagSet5{},
		cmd:   "-O 3+4i=7 -O 9.3-2i=-13",
		again: true,
		want: &flagSet5{
			O: map[complex128]int{
				3 + 4i:   7,
				9.3 - 2i: -13,
			},
		},
	},
	{
		base: &flagSet5{},
		cmd:  "--help",
		additionalArgs: []FlaghandlerOptArg{
			WithHelpText("this is additional help text"),
			PositionalHelp("this is positional help"),
			FlagHelpTag("helptag"),
		},
		want: &flagSet5{
			O: map[complex128]int{
				3 + 4i:   7,
				9.3 - 2i: -13,
			},
		},
		capture: deindent(`
			Usage: PROGRAME-NAME [-options args] this is positional help
			
			Options:
			     [-O<X+Yi>=<int>]               set O (map[complex128]int)
			     [-P<key>=<true|false>]         set P (*map[string]bool)
			
			this is additional help text
			`),
	},
	{
		base: &flagSet3{},
		cmd:  "-p 20 foo help",
		additionalArgs: []FlaghandlerOptArg{
			WithHelpText("this is additional help text"),
		},
		want: &flagSet3{
			P: pointer.To(int32(20)),
		},
		subcommands: map[string]interface{}{
			"foo": &flagSet1{},
		},
		sub: "foo",
		wantSub: &flagSet1{
			I: 11,
		},
		remaining: []string{"xy"},
		capture: deindent(`
			Usage: PROGRAME-NAME [-options args] [parameters]

			Options:
			     [--iflag=int]                  [-i int]  set I (int)
			     [--counter=int]                [-c int]  set C (int)
			     [--ncounter=int]               set NC (int)
			     [--sa1=SA1,SA1]                set SA1 ([2]string)
			     [--sa2=SA2,SA2]                set SA2 ([2]string)

			Subcommands:
			    help                 provide this usage info
			`),
	},
	{
		base: &flagSet5{},
		cmd:  "--dur=30m --help",
		additionalArgs: []FlaghandlerOptArg{
			WithHelpText("this is additional help text"),
			ImportFlagSet(func() *flag.FlagSet {
				fs := flag.NewFlagSet("foo", flag.ContinueOnError)
				_ = fs.Bool("bb", true, "set the great bb")
				_ = fs.Duration("dur", 30*time.Minute, "set a duration")
				return fs
			}()),
		},
		want: &flagSet5{
			O: map[complex128]int{
				3 + 4i:   7,
				9.3 - 2i: -13,
			},
		},
		capture: deindent(`
			Usage: PROGRAME-NAME [-options args] [parameters]
			
			Options:
			     [-O<X+Yi>=<int>]               set O (map[complex128]int)
			     [-P<key>=<true|false>]         set P (*map[string]bool)
			     [--[no-]bb]                    set the great bb (defaults to true)
			     [--dur=dur]                    set a duration (defaults to 30m0s)
			
			this is additional help text
			`),
	},
	{
		base: &flagSet3{},
		importBools: []importBool{
			{
				name: "foo",
				dflt: true,
				help: "foo er",
				want: false,
			},
			{
				name: "bar",
				dflt: true,
				help: "bar er",
				want: true,
			},
			{
				name: "baz",
				dflt: false,
				help: "baz er",
				want: true,
			},
			{
				name: "bing",
				dflt: false,
				help: "bing er",
				want: false,
			},
		},
		importStrings: []importString{
			{
				name: "alpha",
				dflt: "xyz",
				help: "alpha er",
				want: "xyz",
			},
			{
				name: "beta",
				dflt: "abc",
				help: "beta er",
				want: "def",
			},
		},
		cmd:  "--no-foo --baz --beta=def",
		want: &flagSet3{},
	},
}

var usageRE = regexp.MustCompile(`\AUsage: \S+ `)

func TestFlags(t *testing.T) {
	for _, tc := range cases {
		n := tc.cmd
		if tc.name != "" {
			n = tc.name
		}
		t.Run(n, func(t *testing.T) {
			flagTest(t, tc)
		})
		if tc.again {
			t.Run(n+"-again", func(t *testing.T) {
				tc.again = false
				flagTest(t, tc)
			})
		}
	}
}

func flagTest(t *testing.T, tc flagTestCase) {
	t.Log(tc.cmd)
	var called int
	os.Args = append([]string{os.Args[0]}, strings.Split(tc.cmd, " ")...)
	args := []FlaghandlerOptArg{
		OnStart(func(args []string) {
			if tc.sub == "" {
				assert.Equal(t, tc.remaining, args, "remaining args")
			} else {
				assert.Equal(t, ([]string)(nil), args, "remaining args")
			}
			called++
		}),
	}
	args = append(args, tc.additionalArgs...)
	bools := make([]*bool, len(tc.importBools))
	if tc.importBools != nil {
		fs := flag.NewFlagSet("importedBools", flag.ContinueOnError)
		for i, spec := range tc.importBools {
			bools[i] = fs.Bool(spec.name, spec.dflt, spec.help)
		}
		args = append(args, ImportFlagSet(fs))
	}
	istring := make([]*string, len(tc.importStrings))
	if tc.importStrings != nil {
		fs := flag.NewFlagSet("importedStrings", flag.ContinueOnError)
		for i, spec := range tc.importStrings {
			istring[i] = fs.String(spec.name, spec.dflt, spec.help)
		}
		args = append(args, ImportFlagSet(fs))
	}
	fh := PosixFlagHandler(args...)
	subcalled := make(map[string]int)
	for sub, model := range tc.subcommands {
		sub, model := sub, model
		_, err := fh.AddSubcommand(sub, "help for "+sub, model, OnStart(func(args []string) {
			assert.Equal(t, tc.remaining, args, "remaining args in "+sub)
			subcalled[sub]++
		}))
		assert.NoError(t, err, "add help subcommand")
	}
	if tc.goFlags {
		fh = GoFlagHandler(args...)
	}
	registry := NewRegistry(WithFiller("flag", fh))
	baseCopy := deepcopy.Copy(tc.base)
	require.NoError(t, registry.Request(baseCopy), "request")
	if tc.capture != "" {
		testMode = true
		testOutput = ""
		defer func() { testMode = false }()
		assert.PanicsWithValue(t, "exit0", func() {
			err := registry.Configure()
			assert.NoError(t, err)
			panic("not this value")
		})
		got := usageRE.ReplaceAllLiteralString(testOutput, "Usage: PROGRAME-NAME ")
		assert.Equal(t, tc.capture, got, "command output")
		return
	}
	if tc.want != nil {
		err := registry.Configure()
		if tc.error != "" {
			if assert.NotNilf(t, err, "expected configure error %s", tc.error) {
				assert.Contains(t, err.Error(), tc.error, "configure error")
				assert.True(t, commonerrors.IsUsageError(err), "is usage error")
			}
			return
		}
		require.NoError(t, err, "configure")
		if tc.again {
			err = registry.Configure()
			require.NoError(t, err, "configure again")
			assert.Equal(t, 2, called, "onstart call count")
		} else {
			assert.Equal(t, 1, called, "onstart call count")
		}
		assert.Equal(t, tc.want, baseCopy, "data")
		if tc.sub == "" {
			assert.Equal(t, map[string]int{}, subcalled, "sub called")
		} else {
			assert.Equal(t, map[string]int{tc.sub: 1}, subcalled, "sub called")
		}
		for i, spec := range tc.importBools {
			assert.Equal(t, spec.want, *bools[i], "bool "+spec.name)
		}
		for i, spec := range tc.importStrings {
			assert.Equal(t, spec.want, *istring[i], "string "+spec.name)
		}
	}
	if tc.wantExport != nil || tc.exportCmd != "" || tc.exportToo || tc.exportError != "" {
		exportBase := deepcopy.Copy(tc.base)
		fs := flag.NewFlagSet("foo", flag.ContinueOnError)
		if tc.exportDefault != "" {
			args = append(args, WithDefaultsTag(tc.exportDefault))
		}
		err := ExportToFlagSet(fs, "flag", exportBase, args...)
		if tc.exportError != "" {
			if assert.NotNil(t, err, "export error") {
				assert.Contains(t, err.Error(), tc.exportError, "export error")
			}
			return
		} else {
			require.NoError(t, err, "export flagset")
		}
		args := strings.Split(tc.cmd, " ")
		if tc.exportCmd != "" {
			args = strings.Split(tc.exportCmd, " ")
		}
		require.NoError(t, fs.Parse(args), "parse exported flags")
		want := tc.want
		if tc.wantExport != nil {
			want = tc.wantExport
		}
		assert.Equal(t, want, exportBase, "data")
	}
}

var deindentRE = regexp.MustCompile(`\A(\s+)(?:\S|\n)`)

func deindent(s string) string {
	s = strings.TrimPrefix(s, "\n")
	m := deindentRE.FindStringSubmatch(s)
	if len(m) == 0 {
		return s
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(m[1]))
	return re.ReplaceAllLiteralString(s, "")
}
