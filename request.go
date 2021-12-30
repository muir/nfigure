package nfigure

import (
	"reflect"

	"github.com/muir/commonerrors"
	"github.com/pkg/errors"
)

type RequestFuncArg func(*Request)

// FromRoot is intened for use when creating a Request
// rather than creating a Registry.  Not that it won't
// work for a registry, but it's more useful at the
// Request level.
//
// FromRoot specifies a path prefix for how the request
// "mounts" into the configuration hierarchy.
func FromRoot(keys ...string) RegistryFuncArg {
	return func(r *registryConfig) {
		r.prefix = keys
	}
}

// Request tracks a config struct that needs to be filled in.
type Request struct {
	registry *Registry
	name     string
	object   interface{}
	registryConfig
}

// Request regsiters a struct to be filled in when configuration
// is done.  The model should be a pointer to a struct.
func (r *Registry) Request(model interface{}, options ...RegistryFuncArg) error {
	v := reflect.ValueOf(model)
	if !v.IsValid() || v.IsNil() || v.Type().Kind() != reflect.Ptr || v.Type().Elem().Kind() != reflect.Struct {
		return commonerrors.ProgrammerError(errors.Errorf(
			"First argument to Request must be a non-nil pointer to a struct, not %T", model))
	}
	req := &Request{
		registry: r,
		object:   model,
		registryConfig: registryConfig{
			fillers: newFillerCollection(),
		},
	}
	for _, f := range options {
		f(&req.registryConfig)
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.requests = append(r.requests, req)
	if r.configureStarted {
		debug("request: prewalking since configuration has already started")
		err := r.preWalkLocked(req)
		if err != nil {
			return err
		}
	} else {
		debug("request: not prewalking")
	}
	return nil
}

func (r *Request) Registry() *Registry {
	return r.registry
}

func (r *Request) getValidator() (Validate, bool) {
	if r.validator != nil {
		return r.validator, true
	}
	r.registry.lock.Lock()
	defer r.registry.lock.Unlock()
	if r.registry.validator != nil {
		return r.registry.validator, true
	}
	return nil, false
}

func (r *Request) getFillers() *fillerCollection {
	r.registry.lock.Lock()
	defer r.registry.lock.Unlock()
	return r.getFillersLocked()
}

func (r *Request) getFillersLocked() *fillerCollection {
	if r.fillers.IsEmpty() {
		return r.registry.fillers
	}
	fillers := r.registry.fillers.Copy()
	for _, tag := range r.fillers.Order() {
		fillers.Add(tag, r.fillers.m[tag])
	}
	return fillers
}

func (r *Request) getPrefix() []string {
	if len(r.registry.prefix) != 0 {
		if len(r.prefix) != 0 {
			p := make([]string, len(r.registry.prefix), len(r.registry.prefix)+len(r.prefix))
			copy(p, r.registry.prefix)
			p = append(p, r.prefix...)
			return p
		}
		return r.registry.prefix
	}
	return r.prefix
}
