package nfigure

func repeatString(s string, count int) []string {
	r := make([]string, count)
	for i := 0; i < count; i++ {
		r[i] = s
	}
	return r
}

func notEmpty(strings ...string) []string {
	n := make([]string, 0, len(strings))
	for _, s := range strings {
		if s != "" {
			n = append(n, s)
		}
	}
	return n
}
