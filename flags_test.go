package nfigure

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.octolab.org/pointer"
)

type flagSet1 struct {
	I   int       `flag:"iflag i"`
	C   int       `flag:"counter c,counter"`
	NC  int       `flag:"ncounter,!counter"`
	SA1 [2]string `flag:"sa1,split=comma"`
	SA2 [2]string `flag:"sa2,split="`
}

type flagSet2 struct {
	M map[string]int  `flag:"M,map=prefix"`
	N *map[int]string `flag:"nm,map=prefix"`
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

var cases = []struct {
	base        interface{}
	cmd         string
	want        interface{}
	remaining   []string
	error       string
	goFlags     bool
	subcommands map[string]interface{}
	sub         string
	wantSub     interface{}
}{
	{
		base: &flagSet1{},
		cmd:  "-i 3",
		want: &flagSet1{
			I: 3,
		},
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
	},
	{
		base: &flagSet1{},
		cmd:  "--sa1=foo --sa1=bar",
		want: &flagSet1{
			SA1: [2]string{"foo", "bar"},
		},
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
	},
	{
		base: &flagSet3{},
		cmd:  "--pflag=39",
		want: &flagSet3{
			P: pointer.ToInt32(39),
		},
	},
	{
		base: &flagSet3{},
		cmd:  "-f 99.4",
		want: &flagSet3{
			PP: pointerToPointerToPonterToFloat64(99.4),
		},
	},
	{
		base: &flagSet3{},
		cmd:  "-p 20 foo -i 10 xy",
		want: &flagSet3{
			P: pointer.ToInt32(20),
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
		base:    &flagSet4{},
		cmd:     "-sa foo bar baz",
		goFlags: true,
		want: &flagSet4{
			SA: [2]string{"foo", "bar"},
		},
		remaining: []string{"baz"},
	},
	{
		base:    &flagSet4{},
		cmd:     "-s x,x -s y -s z z",
		goFlags: true,
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
			Ip: []*int{pointer.ToInt(7), pointer.ToInt(8)},
		},
	},
	{
		base:    &flagSet4{},
		cmd:     "--ip 7 --ip=8",
		goFlags: true,
		want: &flagSet4{
			Ip: []*int{pointer.ToInt(7), pointer.ToInt(8)},
		},
	},
	{
		base:    &flagSet4{},
		cmd:     "--ip 7,8",
		goFlags: true,
		want: &flagSet4{
			Ip: []*int{pointer.ToInt(7), pointer.ToInt(8)},
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
		base: &flagSet5{},
		cmd:  "-O 3+4i=7 -O 9.3-2i=-13",
		want: &flagSet5{
			O: map[complex128]int{
				3 + 4i:   7,
				9.3 - 2i: -13,
			},
		},
	},
}

func pointerToPointerToPonterToFloat64(f float64) ***float64 {
	p := &f
	pp := &p
	ppp := &pp
	return ppp
}

func TestFlags(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			t.Log(tc.cmd)
			var called int
			os.Args = append([]string{"pgrm"}, strings.Split(tc.cmd, " ")...)
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
			fh := PosixFlagHandler(args...)
			subcalled := make(map[string]int)
			for sub, model := range tc.subcommands {
				sub, model := sub, model
				fh.AddSubcommand(sub, "help for "+sub, model, OnStart(func(args []string) {
					assert.Equal(t, tc.remaining, args, "remaining args in "+sub)
					subcalled[sub]++
				}))
			}
			if tc.goFlags {
				fh = GoFlagHandler(args...)
			}
			registry := NewRegistry(WithFiller("flag", fh))
			require.NoError(t, registry.Request(tc.base), "request")
			err := registry.Configure()
			if tc.error != "" {
				if assert.NotNilf(t, err, "expected configure error %s", tc.error) {
					assert.Contains(t, err.Error(), tc.error, "configure error")
				}
				return
			}
			require.NoError(t, err, "configure")
			assert.Equal(t, tc.want, tc.base, "data")
			assert.Equal(t, 1, called, "onstart call count")
			if tc.sub == "" {
				assert.Equal(t, map[string]int{}, subcalled, "sub called")
			} else {
				assert.Equal(t, map[string]int{tc.sub: 1}, subcalled, "sub called")
			}
		})
	}
}
