package nfigure

import (
	"reflect"
)

func repeatString(s string, count int) []string {
	r := make([]string, count)
	for i := 0; i < count; i++ {
		r[i] = s
	}
	return r
}

// XXX move to refletutils
func nonPointer(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
