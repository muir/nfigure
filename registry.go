package nfigure

import (
	"sync"

	"github.com/muir/nflex"
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

// Registry is the overall configuration context that is shared among
// sources of configuration (Filler interface) and consumers of
// configuration (Requests).
type Registry struct {
	requests         []*Request
	lock             sync.Mutex
	configureStarted bool
	registryConfig
}

type registryConfig struct {
	metaTag   string
	validator Validate
	fillers   *fillerCollection
	prefix    []string
}

// RegistryFuncArg is used to set Registry options.
type RegistryFuncArg func(*registryConfig)

// WithFiller provides a source of configuration to a Registry.
//
// The tag parameter specifies how to invoke that source of configuration.
// For example, if you have a function to lookup information from KMS,
// you might register it as the "secret" filler:
//
//	myStruct := struct {
//		DbPassword string `secret:"dbpasswd"`
//	}
//
//	registry := NewRegistry(WithFiller("secret", NewLookupFiller(myFunc)))
//	registry.Request(myStruct)
//
// If the filler is nil, then any pre-existing filler for that tag
// is removed.
func WithFiller(tag string, filler Filler) RegistryFuncArg {
	return func(r *registryConfig) {
		r.fillers.Add(tag, filler)
	}
}

// WithoutFillers drops all of the fillers that have been registered
// so far, including the default ones.
func WithoutFillers() RegistryFuncArg {
	return func(r *registryConfig) {
		r.fillers = newFillerCollection()
	}
}

// WithValidate registers a validation function to be used to check
// configuration structs after the configuration is complete.  Errors
// reported by the validation function will be wrapped with
// commonerrors.ValidationError and returned by Registry.Configgure()
func WithValidate(v Validate) RegistryFuncArg {
	return func(r *registryConfig) {
		r.validator = v
	}
}

// WithMetaTag specifies the name of the meta tag.
//
// The default meta tag is "nfigure".
//
// Meta-level controls in struct tags can control the name
// for recursion (over-ridden by filler-level tags) and
// the behavior for when multiple fillers can potentially
// provide values.
//
// The first meta tag value is positional and is the name used
// for recursion or "-" to indicate that no filling should happen.
//
// If "first" is true then filling stops after the first filler
// succeeds in filling anything.  This is the default.
//
// If "last" is true then filling starts with the last filler and
// stops when a filler succeeds
//
// If "desc" is false then filling doesn't descend into the keys,
// elements, values of something that has been filled at a higher
// level.
//
// If "combine" is true, then multple sources can be combined together
// when filling arrays, slices, and maps
func WithMetaTag(tag string) RegistryFuncArg {
	return func(r *registryConfig) {
		r.metaTag = tag
	}
}

// NewRegistry creates a configuration context that is shared among
// sources of configuration (Filler interface) and consumers of
// configuration (Requests).  Eventually call Configure() on the
// returned registry.
//
// The following default Fillers are registered:
//
//	env		fill from environment variables
//	config		fill from configuration files
//	default		fill from the tag value
//
// Use WithFiller to adjust the set of fillers and to add
// a command-line flags filler.
//
// By default, no command-line filler is used.  If you want one, add
// WithFiller(PosixFlagHanlder()) or WithFiller(GoFlagHandler()) to
// NewRegistry's functional parameters.
func NewRegistry(options ...RegistryFuncArg) *Registry {
	r := &Registry{
		registryConfig: registryConfig{
			metaTag: "nfigure",
			fillers: newFillerCollection().
				Build("env", NewEnvFiller()).
				Build("config", NewFileFiller()).
				Build("default", NewDefaultFiller()),
		},
	}
	for _, f := range options {
		f(&r.registryConfig)
	}
	return r
}

// ConfigFile adds a source of configuration to all Fillers that implement
// CanAddConfigFileFiller will be be offered the config file.
func (r *Registry) ConfigFile(path string, prefix ...string) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	var rejected error
	debugf("fillers %+v", r.fillers)
	var okay bool
	for _, tag := range r.fillers.Order() {
		filler := r.fillers.m[tag]
		canAdd, ok := filler.(CanAddConfigFileFiller)
		if !ok {
			debugf("filler %s does not support config files", tag)
			continue
		}
		n, err := canAdd.AddConfigFile(path, prefix)
		if err != nil {
			if nflex.IsUnknownFileTypeError(err) {
				debugf("filler %s rejected config file %s: %s", tag, path, err)
				rejected = err
				continue
			}
			return errors.Wrap(err, tag)
		}
		if n == nil {
			debugf("filler %s added config file %s and remains the same", tag, path)
			continue
		}
		debugf("filler %s added config file %s and replaces itself", tag, path)
		r.fillers.Add(tag, n)
		okay = true
	}
	if okay {
		return nil
	}
	if rejected != nil {
		return rejected
	}
	return errors.Errorf("Unable to read config from %s", path)
}

/* TODO
// Any type that implements ConfigureReactive that is filled in during
// the configuration process will have React invoked upon it after filling
// and after validation.
type ConfigureReactive interface {
	React(*Registry) error
}
*/

// Configure evaluates all configuration requests.  New configuration
// requests can be added while configure is running.  For example,
// by having a struct field that implements ConfigureReactive.  New configuration
// files can also be added while Configure is running but the new data will
// only be used for configuration that has not already happened.
func (r *Registry) Configure() error {
	r.configureStarted = true
	debugf("registry: %d requests", r.lenRequests())
	for i := 0; i < r.lenRequests(); i++ {
		request := r.getRequest(i)
		err := r.preWalk(request)
		if err != nil {
			return errors.Wrap(err, request.name)
		}
	}
	for _, tag := range r.fillers.Order() {
		filler := r.fillers.m[tag]
		canPreConfigure, ok := filler.(CanPreConfigureFiller)
		if !ok {
			continue
		}
		err := canPreConfigure.PreConfigure(tag, r)
		if err != nil {
			return err
		}
	}
	for i := 0; i < r.lenRequests(); i++ {
		request := r.getRequest(i)
		err := request.fill()
		if err != nil {
			return errors.Wrap(err, request.name)
		}
	}
	for _, tag := range r.fillers.Order() {
		filler := r.fillers.m[tag]
		canConfigureComplete, ok := filler.(CanConfigureCompleteFiller)
		if !ok {
			continue
		}
		err := canConfigureComplete.ConfigureComplete()
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

func (r *Registry) preWalk(request *Request) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.preWalkLocked(request)
}

func (r *Registry) preWalkLocked(request *Request) error {
	fillers := request.getFillersLocked()
	for _, tag := range fillers.Order() {
		filler, ok := fillers.m[tag].(CanPreWalkFiller)
		if !ok {
			debugf("filler %s cannot PreWalk", tag)
			continue
		}
		err := filler.PreWalk(tag, request.object)
		if err != nil {
			return errors.Wrap(err, tag)
		}
		debugf("filler %s completed PreWalk", tag)
	}
	return nil
}

// GetRequests returns the requests that have been registered in the Registry.
func (r *Registry) GetRequests() []*Request {
	requests := make([]*Request, len(r.requests))
	for i, req := range r.requests {
		requests[i] = req
	}
	return requests
}
