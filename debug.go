//go:build debugNfigure
// +build debugNfigure

package nfigure

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
)

var debugging = true

func debugf(fmt string, args ...interface{}) {
	log.Printf(fmt, args...)
}
func debug(args ...interface{}) {
	log.Println(args...)
}

func callers(levels int) []string {
	pc := make([]uintptr, levels)
	n := runtime.Callers(2, pc)
	if n == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pc[:n])
	r := make([]string, 0, n)
	for {
		frame, more := frames.Next()
		r = append(r, fmt.Sprintf("%s:%d %s", filepath.Base(frame.File), frame.Line, filepath.Base(frame.Function)))
		if !more || len(r) == n {
			break
		}
	}
	return r
}
