package nfigure

import (
	"sync"
	"fmt"

	"github.com/muir/nfigure/nflex"
	"github.com/pkg/errors"
)

// Validate is a subset of the Validate provided by
// https://github.com/go-playground/validator, allowing
// other implementations to be provided if desired
type Validate interface {
	Struct(s interface{}) error
	// StructPartial will only be called with a single Field
	StructPartial(s interface{}, fields ...string) error
}

type Registry struct {
	requests         []*Request
	lock             sync.Mutex
	configFiles      []file
	sources          *nflex.MultiSource
	metaTag          string
	validator        Validate
	fillers          Fillers
	configureStarted bool
}

type RegistryFuncArg func(*Registry)

func WithFiller(tag string, filler Filler) RegistryFuncArg {
	return func(r *Registry) {
		if filler == nil {
			delete(r.fillers, tag)
		} else {
			r.fillers[tag] = filler
		}
	}
}

func WithValidate(v Validate) RegistryFuncArg {
	return func(r *Registry) {
		r.validator = v
	}
}

// Meta-level controls in struct tags can control the name
// for recursion (over-ridden by filler-level tags) and
// the behavior for when multiple fillers can potentially
// provide values.
//
// The default meta tag is "nfigure", the same as used for
// the File fillers.
//
// The first meta tag value is positional and is the name used
// for recursion or "-" to indicate that no filling should happen.
//
// If "first" is true then filling stops after the first filler
// succeeds in filling anything.
//
// If "desc" is false then filling doesn't descend into the keys,
// elements, values of something that has been filled at a higher
// level.
func WithMetaTag(tag string) RegistryFuncArg {
	return func(r *Registry) {
		r.metaTag = tag
	}
}

type file struct {
	name string
	path []string
}

func NewRegistry(options ...RegistryFuncArg) *Registry {
	r := &Registry{
		fillers: Fillers{
			"env":    NewEnvFiller(),
			"source": NewFileFiller(),
		},
	}
	for _, f := range options {
		f(r)
	}
	return r
}

func (r *Registry) ConfigFile(path string, prefix ...string) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	for tag, filler := range r.fillers {
		n, err := filler.AddConfigFile(path, prefix)
		if err != nil {
			return errors.Wrap(err, tag)
		}
		if n != nil {
			r.fillers[tag] = n
		}
	}
	return nil
}

// Any type that implements ConfigureReactive that is filled in during
// the configuration process will have React invoked upon it after filling
// and after validation.
type ConfigureReactive interface {
	React(*Registry) error
}

// Configure evaluates all configuration requests.  New configuration
// requests can be added while configure is running.  For example,
// by having a struct field that implements ConfigureReactive.  New configuration
// files can also be added while Configure is running but the new data will
// only be used for configuration that has not already happened.
func (r *Registry) Configure() error {
	r.configureStarted = true
	fmt.Println("XXX lenReq", r.lenRequests())
	for i := 0; i < r.lenRequests(); i++ {
		request := r.getRequest(i)
		err := r.lockAndPreWalk(request)
		if err != nil {
			return errors.Wrap(err, request.name)
		}
	}
	for tag, filler := range r.fillers {
		err := filler.PreConfigure(tag, r)
		if err != nil {
			return err
		}
	}
	for i := 0; i < r.lenRequests(); i++ {
		request := r.getRequest(i)
		err := request.fill(r.fillers)
		if err != nil {
			return errors.Wrap(err, request.name)
		}
	}
	for _, filler := range r.fillers {
		err := filler.ConfigureComplete()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) lenRequests() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return len(r.requests)
}

func (r *Registry) getRequest(i int) *Request {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.requests[i]
}

func (r *Registry) lockAndPreWalk(req *Request) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.preWalk(req)
}

func (r *Registry) preWalk(request *Request) error {
	fmt.Println("XXX all prewalk")
	for tag, filler := range r.fillers {
		err := filler.PreWalk(tag, request, request.object)
		if err != nil {
			return errors.Wrap(err, tag)
		}
	}
	return nil
}
