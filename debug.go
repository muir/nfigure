//go:build debugNfigure
package nfigure

import (
	"log"
)

func debugf(fmt string, args ...interface{}) {
	log.Printf(fmt, args...)
}
func debug(args ...interface{}) {
	log.Println(args...)
}
