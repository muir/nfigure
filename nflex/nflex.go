package nflex

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

var ErrDoesNotExist = fmt.Errorf("requested item does not exist")
var ErrWrongType = fmt.Errorf("requested item is not the requested type")

type NodeType int

const (
	Undefined NodeType = iota // node does not exist
	Nil
	String
	Float
	Int
	Bool
	Slice
	Map
)

// Source is an abstraction of model encodings.  If you need to work with
// arbitray encodings of data models, you either need to unpack the encoded
// data into a interface{} or you need an abstraction.  This is such an
// abstraction.
type Source interface {
	Exists(keys ...string) bool
	GetBool(keys ...string) (bool, error)
	GetInt(keys ...string) (int64, error)
	GetFloat(keys ...string) (float64, error)
	GetString(keys ...string) (string, error)
	Recurse(keys ...string) Source // can return nil
	Keys(keys ...string) ([]string, error)
	Len(keys ...string) (int, error)
	Type(keys ...string) NodeType
}

var unmarshallers = map[string]func([]byte) (Source, error){
	"yaml": UnmarshalYAML,
	"yml":  UnmarshalYAML,
	"json": UnmarshalJSON,
}

type unmarshalOpts struct {
	FS fs.FS
}

type UnmarshalFileArg func(*unmarshalOpts)

func WithFS(fs fs.FS) UnmarshalFileArg {
	return func(o *unmarshalOpts) {
		o.FS = fs
	}
}

func UnmarshalFile(file string, args ...UnmarshalFileArg) (Source, error) {
	opts := unmarshalOpts{
		FS: unrestrictedFS{},
	}
	for _, f := range args {
		f(&opts)
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext != "" {
		ext = ext[1:]
	}
	uf, ok := unmarshallers[ext]
	if !ok {
		return nil, UnknownFileTypeError(errors.Errorf("Could not determine unmarshaller for %s (%s)", file, ext))
	}
	byts, err := fs.ReadFile(opts.FS, file)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", file)
	}

	return uf(byts)
}

func combine(x []string, y []string) []string {
	n := make([]string, len(x), len(x)+len(y))
	copy(n, x)
	n = append(n, y...)
	return n
}

type unrestrictedFS struct{}

func (u unrestrictedFS) Open(name string) (fs.File, error)     { return os.Open(name) }
func (u unrestrictedFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

type unknownFileTypeError struct {
	cause error
}

// UnknownFileType annotates an error as being caused by not knowing the file type.
func UnknownFileTypeError(err error) error {
	if err == nil {
		return nil
	}
	return unknownFileTypeError{
		cause: errors.WithStack(err),
	}
}

func (u unknownFileTypeError) Error() string { return u.cause.Error() }
func (u unknownFileTypeError) Unwrap() error { return u.cause }
func (u unknownFileTypeError) Cause() error  { return u.cause }
func (u unknownFileTypeError) Is(err error) bool {
	_, ok := err.(unknownFileTypeError)
	return ok
}

func IsUnknownFileTypeError(err error) bool {
	var u unknownFileTypeError
	return errors.Is(err, u)
}
