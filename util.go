package nfigure

func repeatString(s string, count int) []string {
	r := make([]string, count)
	for i := 0; i < count; i ++ {
		r[i] = s
	}
	return r
}
