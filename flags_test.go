package nfigure

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flagSet1 struct {
	I   int       `flag:"iflag i"`
	C   int       `flag:"counter c,counter"`
	NC  int       `flag:"ncounter,!counter"`
	SA1 [2]string `flag:"sa1,split=comma"`
	SA2 [2]string `flag:"sa2,split="`
}

var cases = []struct {
	base	interface{}
	cmd       string
	want      interface{}
	sub       interface{}
	remaining []string
	error string
}{
	{
		base: &flagSet1{},
		cmd: "-i 3",
		want: &flagSet1{
			I: 3,
		},
	},
	{
		base: &flagSet1{},
		cmd: "--iflag 4",
		want: &flagSet1{
			I: 4,
		},
	},
	{
		base: &flagSet1{},
		cmd: "--iflag=5",
		want: &flagSet1{
			I: 5,
		},
	},
	{
		base: &flagSet1{},
		cmd: "-c -c -c -c",
		want: &flagSet1{
			C: 4,
		},
	},
	{
		base: &flagSet1{},
		cmd: "-c --counter=10 -c -c",
		want: &flagSet1{
			C: 12,
		},
	},
	{
		base: &flagSet1{},
		cmd: "--ncounter --ncounter",
		error: "invalid syntax",
	},
	{
		base: &flagSet1{},
		cmd: "--sa1=foo,bar",
		want: &flagSet1{
			SA1: [2]string{"foo", "bar"},
		},
	},
	{
		base: &flagSet1{},
		cmd: "--sa1=foo --sa1=bar",
		want: &flagSet1{
			SA1: [2]string{"foo", "bar"},
		},
	},
}

func TestFlags(t *testing.T) {
	for _, tc := range cases {
		t.Log(tc.cmd)
		var called int
		os.Args = strings.Split(tc.cmd, " ")
		fh := PosixFlagHandler(OnStart(func(args []string) {
			assert.Equal(t, tc.remaining, args, "remaining args")
			called++
		}))
		registry := NewRegistry(WithFiller("flag", fh))
		require.NoError(t, registry.Request(tc.base), "request")
		err := registry.Configure()
		if tc.error != "" {
			assert.NotNilf(t, err, "expected configure error %s", tc.error)
			assert.Contains(t, err.Error(), tc.error, "configure error")
			continue
		} 
		require.NoError(t, err, "configure")
		assert.Equal(t, tc.want, tc.base, "data")
		assert.Equal(t, 1, called, "onstart call count")
	}
}
