package nflex

// CanMutate must be implemented by sources that wrap or contain other
// sources.  The Mutate method allows the wrapped sources to be
// rewrapped or modifed in some way.  The mutation only applies to the
// inner source, not the source that CanMutate.
type CanMutate interface {
	Source
	Mutate(Mutation) Source
}

// Mutation is a function that transforms a source into a new source
type Mutation func(Source) Source

// Apply transforms sources and the sources embedded in other sources
// into a new source by applying the Mutation to both.
func (m Mutation) Apply(source Source) Source {
	if mutator, ok := source.(CanMutate); ok {
		source = mutator.Mutate(m)
	}
	source = m(source)
	return source
}

// Combine turns two mutations into a single mutation which provides
// some efficincy when applying -- it's less effort to apply a combined
// pair of muations that to apply them serially.
func (m Mutation) Combine(n Mutation) Mutation {
	return func(source Source) Source {
		return n(m(source))
	}
}
