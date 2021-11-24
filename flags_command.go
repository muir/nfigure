package nfigure

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/muir/nject/nject"
	"github.com/muir/reflectutils"
	"github.com/pkg/errors"
)

// General calling order....
//
// 1. PosixFlagHandler() or GoFlagHandler()
// 2. PreWalk()
// 3. PreConfigure() -- calls parseFlags()
// 4. Fill()
// 5. ConfigureComplete()

// Flaghandler is the common type for both PosixFlagHanlder() and GoFlagHandler().
// The set of flags are found in struct tags, by default with the "flag" prefix.
//
//	type MyFlags struct {
//		Verbose  int      `flag:"v verbose,counter"`    // each "-v" or "--verbose" increments the integer
//		Comments []string `flag:"comment c,split=none"` // "-c value" and "--comment value" can be given multiple times
//	}
//
// The first argument after flag: is the name or names of the flag.  After
// that there are options.  The supported options are:
//
// "map=explode|prefix": specifies how to handle map types.  With "map=explode",
// key/value pairs are given as arguments after the flag:
//
//	type MyFlags struct {
//		Env map[string]string `flag:"env e,map=explode,split=equals"`
//	}
//
//	cmd --env FOO=bar --env XYZ=yes -e MORE=totally
//
// With "map=prefix", the values are combined into the flag:
//
//	type MyFlags struct {
//		Defs map[string]string `flag:"D,map=prefix"`
//	}
//
//	cmd -DFOO=bar -DXYZ=yes -DMORE=totally
//
// The default is "map=explode"
//
// "split=x": For arrays, slices, and maps, changes how single
// values are split into groups.
//
// The special values of "none", "equal", "equals", "comma", "quote",
// and "space" translate to obvious values.
//
// The default value is "," for arrays and slices and "=" for maps.  For
// "map=prefix", only "=" is supported.
//
// To indicate that a numeric value is a counter, use "counter".
//
// To indicate that a value is required as a flag, use "required".
//
// To tweak the usage message describing the value use "argName=name".
//
//	struct MyFlags struct {
//		Depth      int `flag:"depth,required,argName=levels"`
//		DebugLevel int `flag:"d,counter"`
//	}
//
type FlagHandler struct {
	fhInheritable
	Parent             *FlagHandler // set only for subcommands
	subcommands        map[string]*FlagHandler
	subcommandsOrder   []string
	longFlags          map[string]*flagRef
	shortFlags         map[string]*flagRef
	mapFlags           map[string]*flagRef // only when map=prefix
	rawData            []reflect.StructField
	mapRE              *regexp.Regexp
	remainder          []string
	onActivate         func(*Registry, *FlagHandler) error
	onStart            func(*Registry, *FlagHandler, []string) error
	delayedErr         error
	configModel        interface{}
	usageSummary       string
	positionalHelp     string
	selectedSubcommand string
}

type fhInheritable struct {
	tagName       string
	registry      *Registry
	stopOnNonFlag bool
	doubleDash    bool
	singleDash    bool
	combineShort  bool
	negativeNo    bool
	helpTag       string
}

type flagTag struct {
	Name      []string `pt:"0,split=space"`
	Map       string   `pt:"map"`   // special value: prefix|explode
	Split     string   `pt:"split"` // special value: explode, quote, space, comma, equal, equals, none
	IsCounter bool     `pt:"counter"`
	Required  bool     `pt:"required"` // flag must be used
	ArgName   string   `pt:"argName"`  // name of the argument(s) for usage message
}

type flagRef struct {
	flagTag
	// special	func(*FlagHandler) XXX
	isBool  bool
	isSlice bool
	isMap   bool
	explode int // for arrays only
	setters map[setterKey]func(reflect.Value, string) error
	values  []string
	used    []string
	keys    []string
}

type setterKey struct {
	typ reflect.Type
	tag string
}

var _ Filler = &FlagHandler{}

// PosixFlagHandler creates and configures a flaghandler that
// requires long options to be preceeded with a double-dash
// and will combine short flags together.
//
// Long-form booleans can be set to false with a "no-" prefix.
//
//	tar -xvf f.tgz --numeric-owner --hole-detection=raw --ownermap ownerfile --no-overwrite-dir
//
// Flags are found using struct tags.  See the comment FlagHandler for details
func PosixFlagHandler(opts ...FlaghandlerOptArg) *FlagHandler {
	h := &FlagHandler{
		fhInheritable: fhInheritable{
			doubleDash:   true,
			combineShort: true,
			negativeNo:   true,
			helpTag:      "help",
		},
	}
	h.init()
	h.delayedErr = h.opts(opts)
	return h
}

func GoFlagHandler(opts ...FlaghandlerOptArg) *FlagHandler {
	h := &FlagHandler{
		fhInheritable: fhInheritable{
			doubleDash: true,
			singleDash: true,
		},
	}
	h.init()
	h.delayedErr = h.opts(opts)
	return h
}

func (h *FlagHandler) init() {
	h.subcommands = make(map[string]*FlagHandler)
	h.longFlags = make(map[string]*flagRef)
	h.shortFlags = make(map[string]*flagRef)
	h.mapFlags = make(map[string]*flagRef)
}

func (h *FlagHandler) opts(opts []FlaghandlerOptArg) error {
	for _, f := range opts {
		err := f(h)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *FlagHandler) PreConfigure(tagName string, registry *Registry) error {
	h.tagName = tagName
	h.registry = registry
	if h.delayedErr != nil {
		return h.delayedErr
	}
	if h.configModel != nil {
		err := registry.Request(h.configModel)
		if err != nil {
			return err
		}
	}
	if h.onActivate != nil {
		err := h.onActivate(registry, h)
		if err != nil {
			return err
		}
	}
	return h.parseFlags(1) // 0 is the program name so we skip it
}

func (h *FlagHandler) ConfigureComplete() error {
	if h.selectedSubcommand != "" {
		err := h.subcommands[h.selectedSubcommand].ConfigureComplete()
		if err != nil {
			return errors.Wrap(err, h.selectedSubcommand)
		}
	}
	if h.onStart != nil {
		err := h.onStart(h.registry, h, h.remainder)
		if err != nil {
			return err
		}
	}
	return nil
}

type FlaghandlerOptArg func(*FlagHandler) error

// OnActivate is called before flags are parsed.  It's mostly for subcommands.  The
// callback will be invoked as soon as it is known that the subcommand is being
// used.
func OnActivate(chain ...interface{}) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		return nject.Sequence("default-error-responder",
			nject.Provide("default-error", func() nject.TerminalError {
				return nil
			})).Append("on-activate", chain...).Bind(&h.onActivate, nil)
	}
}

func OnStart(chain ...interface{}) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		return nject.Sequence("default-error-responder",
			nject.Provide("default-error", func() nject.TerminalError {
				return nil
			})).Append("on-start", chain...).Bind(&h.onStart, nil)
	}
}

func PositionalHelp(positionalHelp string) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		h.positionalHelp = positionalHelp
		return nil
	}
}

func (h *FlagHandler) AddSubcommand(command string, usageSummary string, configModel interface{}, opts ...FlaghandlerOptArg) (*FlagHandler, error) {
	if configModel != nil {
		v := reflect.ValueOf(configModel)
		if !v.IsValid() || v.IsNil() || v.Type().Kind() != reflect.Ptr || v.Type().Elem().Kind() != reflect.Struct {
			return nil, ProgrammerError(errors.Errorf("configModel must be a nil or a non-nil pointer to a struct, not %T", configModel))
		}
	}
	sub := &FlagHandler{
		fhInheritable: h.fhInheritable,
		Parent:        h,
		configModel:   configModel,
		usageSummary:  usageSummary,
	}
	h.subcommands[command] = sub
	h.subcommandsOrder = append(h.subcommandsOrder, command)
	sub.init()
	return sub, sub.opts(opts)
}

type optCategory int

const (
	undefinedOpt optCategory = iota
	flagOpt
	optionOpt
	parameterOpt
	subcommandOpt
	lastOpt // keep this one last in the list
)

type opt struct {
	name       string
	help       string
	category   optCategory
	f          reflect.StructField
	isBool     bool
	nonPointer reflect.Type
	primary    bool
	ref        flagRef
	alts       []*opt
}

func (o opt) format(doubleDash bool) string {
	var optional string
	var b strings.Builder
	if !o.ref.Required {
		b.WriteRune('[')
		optional = "]"
	}
	switch o.category {
	case flagOpt:
		b.WriteRune('-')
		b.WriteString(o.name)
	case optionOpt:
		b.WriteRune('-')
		b.WriteString(o.name)
		if o.nonPointer.Kind() == reflect.Map {
			b.WriteRune('<')
			b.WriteString(o.describeArg(o.nonPointer.Key(), o.f.Name+"-key"))
			b.WriteString(">=<")
			b.WriteString(o.describeArg(o.nonPointer.Elem(), o.f.Name+"-value"))
			b.WriteRune('>')
		} else {
			b.WriteRune(' ')
			b.WriteString(o.describeArg(o.nonPointer, o.f.Name))
		}
	case parameterOpt:
		b.WriteRune('-')
		if doubleDash {
			b.WriteRune('-')
		}
		if o.isBool {
			b.WriteString("[no-]")
		}
		b.WriteString(o.name)
		switch o.nonPointer.Kind() {
		case reflect.Bool:
		case reflect.Map:
			b.WriteRune('<')
			b.WriteString(o.describeArg(o.nonPointer.Key(), o.f.Name+"-key"))
			b.WriteString(">=<")
			b.WriteString(o.describeArg(o.nonPointer.Elem(), o.f.Name+"-value"))
			b.WriteRune('>')
		default:
			if doubleDash {
				b.WriteRune('=')
			} else {
				b.WriteRune(' ')
			}
			b.WriteString(o.describeArg(o.nonPointer, o.f.Name))
		}
	}
	b.WriteString(optional)
	b.WriteRune(' ')
	return b.String()
}

func (o opt) describeArg(typ reflect.Type, name string) string {
	switch typ.Kind() {
	case reflect.Slice:
		ed := o.describeArg(typ.Elem(), name)
		return ed + o.ref.Split + ed + "..."
	case reflect.Array:
		ed := o.describeArg(typ.Elem(), name)
		return strings.Join(repeatString(ed, typ.Len()), o.ref.Split)
	case reflect.Bool:
		return "true|false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "x.y"
	case reflect.Complex64, reflect.Complex128:
		return "X+Yi"
	default:
		return name
	}
}

func (o opt) formatAlts(doubleDash bool) string {
	if len(o.alts) == 0 {
		return ""
	}
	return strings.Join(formatOpts(doubleDash, o.alts), "")
}

func (h *FlagHandler) formatOpts(opts []*opt) []string {
	return formatOpts(h.doubleDash, opts)
}

func formatOpts(doubleDash bool, opts []*opt) []string {
	res := make([]string, len(opts))
	for i, o := range opts {
		res[i] = o.format(doubleDash)
	}
	return res
}

func (h *FlagHandler) Usage() string {
	// done := make(map[string]struct{})

	// flags : -x
	// options : -y foo
	// parameters : --ything foo OR --ything=foo
	// required: anything required

	required := make([][]*opt, lastOpt)
	optional := make([][]*opt, lastOpt)

	for _, f := range h.rawData {
		tagSet := reflectutils.SplitTag(f.Tag).Set()
		ref := flagRef{
			flagTag: flagTag{
				Split: ",",
			},
		}
		_ = tagSet.Get(h.tagName).Fill(&ref) // should not error
		help := tagSet.Get(h.helpTag).Tag
		if help == "" {
			help = fmt.Sprintf("set %s (%s)", f.Name, f.Type)
		}
		nonPointer := reflectutils.NonPointer(f.Type)
		isBool := nonPointer.Kind() == reflect.Bool
		var lead *opt
		for i, n := range ref.Name {
			o := &opt{
				name:       n,
				help:       help,
				f:          f,
				isBool:     isBool,
				primary:    i == 0,
				ref:        ref,
				nonPointer: nonPointer,
			}
			if i == 0 {
				lead = o
			} else {
				lead.alts = append(lead.alts, o)
			}

			switch utf8.RuneCountInString(n) {
			case 0:
				continue
			case 1:
				if isBool {
					o.category = flagOpt
				} else {
					o.category = optionOpt
				}
			default:
				o.category = parameterOpt
			}
			if ref.Required {
				required[o.category] = append(required[o.category], o)
				break
			}
			optional[o.category] = append(optional[o.category], o)
		}
	}
	usage := make([]string, 0, len(h.rawData)*2+10+len(h.subcommands)*2)
	usage = append(usage, "Usage: "+os.Args[0]+" ")

	switch len(optional[flagOpt]) {
	case 0:
	default:
		if h.combineShort {
			usage = append(usage, "[-flags] ")
		} else {
			usage = append(usage, "[flags] ")
		}
	}
	usage = append(usage, h.formatOpts(required[flagOpt])...)

	switch len(optional[optionOpt]) {
	case 0:
	default:
		if h.combineShort {
			usage = append(usage, "[-options option-args] ")
		} else {
			usage = append(usage, "[options] ")
		}
	}
	usage = append(usage, h.formatOpts(required[optionOpt])...)

	switch len(optional[parameterOpt]) {
	case 0:
	default:
		usage = append(usage, "[parameters] ")
	}
	usage = append(usage, h.formatOpts(required[parameterOpt])...)

	switch len(h.subcommands) {
	case 0:
	case 1, 2, 3, 4, 5, 6, 7:
		usage = append(usage, strings.Join(h.subcommandsOrder, "|")+" ")
	default:
		usage = append(usage, "subcommand ")
	}

	if h.positionalHelp != "" {
		usage = append(usage, h.positionalHelp)
	}

	usage = append(usage, "\n")

	if len(h.rawData) > 0 {
		usage = append(usage, "\nOptions:\n")
		for i := undefinedOpt; i < lastOpt; i++ {
			for _, optSet := range [][]*opt{required[i], optional[i]} {
				for _, opt := range optSet {
					if !opt.primary {
						continue
					}
					usage = append(usage, fmt.Sprintf(
						"    %-30s %s\n",
						opt.format(h.doubleDash),
						strings.Join(notEmpty(
							opt.formatAlts(h.doubleDash),
							opt.help,
						), " ")))
				}
			}
		}
	}

	if len(h.subcommands) > 0 {
		usage = append(usage, "\nSubcommands:\n")
		for _, subcmd := range h.subcommandsOrder {
			sub := h.subcommands[subcmd]
			usage = append(usage, fmt.Sprintf(
				"    %-20s %s\n",
				subcmd,
				sub.usageSummary))
		}
	}

	return strings.Join(usage, "")
}

//
// program [flags] [--xyz number] subcommand
//
