package nfigure

import (
	"flag"
	"unicode/utf8"

	"github.com/pkg/errors"
)

type hasIsBool interface {
	IsBoolFlag() bool
}

// ImportFlagSet pulls in flags defined with the standard "flag"
// package.  This is useful when there are libaries being used
// that define flags.
//
// flag.CommandLine is the default FlagSet.
//
// ImportFlagSet is not the recommended way to use nfigure, but sometimes
// there is no choice.
func ImportFlagSet(fs *flag.FlagSet) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		if fs.Parsed() {
			return errors.New("Cannot import FlagSets that have been parsed")
		}
		var err error
		fs.VisitAll(func(f *flag.Flag) {
			var isBool bool
			if hib, ok := f.Value.(hasIsBool); ok {
				isBool = hib.IsBoolFlag()
			}
			ref := &flagRef{
				flagTag: flagTag{
					Name: []string{f.Name},
				},
				flagRefComparable: flagRefComparable{
					isBool: isBool,
				},
				imported: f,
			}
			switch utf8.RuneCountInString(f.Name) {
			case 0:
				err = errors.New("Invalid flag in FlagSet with no Name")
			case 1:
				h.shortFlags[f.Name] = ref
			default:
				h.longFlags[f.Name] = ref
			}
			h.imported = append(h.imported, ref)
		})
		return err
	}
}

// importFlags deals with setting values for standard "flags" that have been
// imported.
func (h *FlagHandler) importFlags() error {
	for _, ref := range h.imported {
		switch len(ref.values) {
		case 0:
			if ref.imported.DefValue != "" {
				err := ref.imported.Value.Set(ref.imported.DefValue)
				if err != nil {
					return errors.Errorf("Cannot set default value for flag '%s': %s",
						ref.imported.Name, err)
				}
			}
		case 1:
			err := ref.imported.Value.Set(ref.values[0])
			if err != nil {
				return errors.Errorf("Cannot set value for flag '%s': %s",
					ref.imported.Name, err)
			}
		default:
			return errors.Errorf("Cannot set multiple values for flag '%s'", ref.imported.Name)
		}
	}
	if h.selectedSubcommand != "" {
		return h.subcommands[h.selectedSubcommand].importFlags()
	}
	return nil
}
