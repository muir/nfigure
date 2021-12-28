//go:build !debugNfigure

package nfigure

var debugging = false

func debugf(fmt string, args ...interface{}) {}
func debug(args ...interface{})              {}
func callers(levels int) []string            { return nil }
