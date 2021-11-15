package nfigure

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

type RequestFuncArg func(*Request)

func FromRoot(keys ...string) RequestFuncArg {
	return func(r *Request) {
		r.prefix = keys
	}
}

// Request tracks a config struct that needs to be filled in.
type Request struct {
	registry *Registry
	name     string
	prefix   []string
	object   interface{}
	validator Validate
}

// Request regsiters a struct to be filled in when configuration
// is done.  The model should be a pointer to a struct.
func (r *Registry) Request(model interface{}, options ...RequestFuncArg) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	v := reflect.ValueOf(model)
	if !v.IsValid() || v.IsNil() || v.Type().Kind() != reflect.Ptr || v.Type().Elem().Kind() != reflect.Struct {
		return errors.Errorf("First argument to Request must be a non-nil pointer to a struct, not %T", model)
	}
	req := &Request{
		registry: r,
		object:   model,
	}
	for _, f := range options {
		f(req)
	}
	r.requests = append(r.requests, req)
	if r.configureStarted {
		err := r.preWalk(req)
		if err != nil { return err }
	}
	fmt.Printf("XXX requested %T\n", model)
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
