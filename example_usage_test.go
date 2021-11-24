package nfigure

import (
	"fmt"
	"os"
	"strings"
)

type arguments struct {
	User      string          `flag:"u user,required"`
	Hosts     []string        `flag:"host h,split=&"`
	Confusion map[int]float64 `flag:"confusion C"`
	// 	OMap map[string]bool `flag:"oset,split=explode"`
}

func Example_usage() {
	fh := PosixFlagHandler()
	os.Args = strings.Split("program --flag-not-defined", " ")
	registry := NewRegistry(WithFiller("flag", fh))
	var arguments arguments
	registry.Request(&arguments)
	err := registry.Configure()
	if IsUsageError(err) {
		fmt.Println(fh.Usage())
	}
	// Output: foo
}
