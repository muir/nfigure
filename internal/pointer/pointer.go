package pointer

func To[T any](t T) *T {
	return &t
}

func Value[T any](t *T) T {
	if t != nil {
		return *t
	}
	var b T
	return b
}
