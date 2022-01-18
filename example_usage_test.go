package nfigure

import (
	"fmt"
	"os"
	"strings"

	"github.com/muir/commonerrors"
)

type arguments struct {
	User      string          `flag:"u user,required,argName=email" help:"email address"`
	Hosts     []string        `flag:"host h,split=&"`
	Confusion map[int]float64 `flag:"confusion C,map=prefix"`
	OMap      map[string]bool `flag:"oset,split=/"`
}

func Example_usage() {
	fh := PosixFlagHandler(PositionalHelp("file(s)"))
	os.Args = strings.Split("program --flag-not-defined", " ")
	registry := NewRegistry(WithFiller("flag", fh))
	var arguments arguments
	_ = registry.Request(&arguments)
	err := registry.Configure()
	if commonerrors.IsUsageError(err) {
		fmt.Println(fh.Usage())
	}
	// Output: Usage: program [-options args] -u email [parameters] file(s)
	//
	// Options:
	//      -u email                       email address
	//      [--host=Hosts&Hosts...]        [-h Hosts&Hosts...]  set Hosts ([]string)
	//      [--confusion<int>=<N.N>]       [-C<int>=<N.N>]  set Confusion (map[int]float64)
	//      [--oset key/true|false]        set OMap (map[string]bool)
}
