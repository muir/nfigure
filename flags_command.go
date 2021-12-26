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
	helpText           *string // if not-nil, implies --help flag and help subcommmand
	helpAlreadyAdded   bool
	alreadyParsed      bool
}

var _ CanPreWalkFiller = &FlagHandler{}
var _ CanConfigureCompleteFiller = &FlagHandler{}
var _ CanPreConfigureFiller = &FlagHandler{}

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
	Name []string `pt:"0,split=space"`
	flagTagComparable
}

type flagTagComparable struct {
	Map       string `pt:"map"`   // special value: prefix|explode
	Split     string `pt:"split"` // special value: explode, quote, space, comma, equal, equals, none
	IsCounter bool   `pt:"counter"`
	Required  bool   `pt:"required"` // flag must be used
	ArgName   string `pt:"argName"`  // name of the argument(s) for usage message
}

type flagRef struct {
	flagTag
	flagRefComparable
	values    []string
	used      []string
	keys      []string
	setters   map[setterKey]func(reflect.Value, string) error
	fieldName string
	tagValue  string
	typ       reflect.Type
}

type flagRefComparable struct {
	isBool  bool
	isSlice bool
	isMap   bool
	explode int // for arrays only
}

// setterKey is used to cache setters.  Setters only depend upon the
// type of the thing being filled and how it is split.
type setterKey struct {
	typ   reflect.Type
	split string
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
// Long-form options require a double-dash (--flag).  Flag values can be set
// two ways: "--flag=value" or "--flag value".
//
// Multiple short-flags (-a -b -c) can be combined (-abc).  Short flags that are
// not booleans or counters have arguments that follow.  When combined they remain
// in the same order.  The following are the same, assuming -a and -b are both
// short form flags that take an argument:
//
//	-a x -b y
//	-ab x y
//
// Booleans are set with "--flag" or unset with "--no-flag".
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

// GoFlagHandler creates and configures a flaghandler that mirrors Go's native
// "flag" package in behavior.  Long-form flags can have a single dash or double
// dashes (-flag vs --flag).
//
// Assignment or positional args are both supported -flag=value and -flag value.
//
// Flags are found using struct tags.  See the comment FlagHandler for details
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

// PreConfigure is part of the Filler contract.  It is called by Registery.Configure
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

// ConfigureComplete is part of the Filler contract.  It is called by Registery.Configure
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

// FlaghandlerOptArg are options for flaghandlers
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

// OnStart is called at the end of configuration.  It does not need to return until
// the program terminates (assuming there are no other Fillers in use that take
// action during ConfigureComplete and also assuming that there isn't an OnStart in
// at the subommmand level also).
func OnStart(chain ...interface{}) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		return nject.Sequence("default-error-responder",
			nject.Provide("default-error", func() nject.TerminalError {
				return nil
			})).Append("on-start", chain...).Bind(&h.onStart, nil)
	}
}

// WithHelpText adds to the usage output and establishes a "--help" flag and
// also a "help" subcommand (if there are any other subcommands).  If there are
// other subcommands it is recommended that with WithHelpText be used to set
// help text for each one.
func WithHelpText(helpText string) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		h.helpText = &helpText
		return nil
	}
}

// PositionalHelp provides a help string for what to display in the usage
// summary after the flags, and options.  For example: "file(s)"
func PositionalHelp(positionalHelp string) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		h.positionalHelp = positionalHelp
		return nil
	}
}

// FlagHelpTag specifies the name of the tag to use for providing
// per-flag help summaries.  For example, you may want:
//
//	type MyConfig struct {
//		User string `flag:"u" help:"Email address"`
//	}
//
// The default is "help"
func FlagHelpTag(tagName string) FlaghandlerOptArg {
	return func(h *FlagHandler) error {
		h.helpTag = tagName
		return nil
	}
}

func (h *FlagHandler) addHelpFlagAndCommand() error {
	if h.helpAlreadyAdded || h.helpText == nil {
		return nil
	}
	h.helpAlreadyAdded = true
	if _, ok := h.longFlags["help"]; ok {
		return ProgrammerError(errors.New("cannot define a 'help' flag and use FlagHelpTag()"))
	}
	h.longFlags["help"] = &flagRef{
		flagRefComparable: flagRefComparable{
			isBool: true,
		},
		typ: reflect.TypeOf((*bool)(nil)).Elem(),
	}
	if len(h.subcommands) > 0 {
		if _, ok := h.subcommands["help"]; ok {
			return ProgrammerError(errors.New("cannot define a 'help' subcommand and use FlagHelpTag()"))
		}
		h.AddSubcommand("help", "provide this usage info", nil, OnActivate(
			func() {
				fmt.Print(h.Usage())
				os.Exit(0)
			}))
	}
	return nil
}

// AddSubcommand adds behavior around the non-flags found in the list of
// arguments.  An argument matching the "command" argument string will
// eventually trigger calling that subcommand.  After a subcommand, only
// flags defined in the "configModel" argument will be recognized.
// Use OnStart to invoke the subcommand.
//
// The "usageSummary" string is a one-line description of what the subcommand
// does.
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

func (h *FlagHandler) clearParse() {
	for _, fs := range []map[string]*flagRef{
		h.longFlags,
		h.shortFlags,
		h.mapFlags,
	} {
		for _, ref := range fs {
			ref.values = nil
			ref.used = nil
			ref.keys = nil
		}
	}
	for _, sub := range h.subcommands {
		sub.clearParse()
	}
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
	nonPointer reflect.Type
	primary    bool
	ref        flagRef
	alts       []*opt
}

func (o opt) format(doubleDash bool) string {
	debugf("o.ref: %+v", o.ref)
	var optional string
	var b strings.Builder
	b.WriteRune(' ')
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
			b.WriteString(o.describeArg(o.nonPointer.Key(), "key", ""))
			b.WriteString(">=<")
			b.WriteString(o.describeArg(o.nonPointer.Elem(), "value", ""))
			b.WriteRune('>')
		} else {
			b.WriteRune(' ')
			b.WriteString(o.describeArg(o.nonPointer, o.f.Name, o.ref.ArgName))
		}
	case parameterOpt:
		b.WriteRune('-')
		if doubleDash {
			b.WriteRune('-')
		}
		if o.ref.isBool {
			b.WriteString("[no-]")
		}
		b.WriteString(o.name)
		switch o.nonPointer.Kind() {
		case reflect.Bool:
		case reflect.Map:
			if o.ref.Map == "prefix" {
				b.WriteRune('<')
				b.WriteString(o.describeArg(o.nonPointer.Key(), "key", ""))
				b.WriteString(">=<")
				b.WriteString(o.describeArg(o.nonPointer.Elem(), "value", ""))
				b.WriteRune('>')
			} else {
				b.WriteRune(' ')
				b.WriteString(o.describeArg(o.nonPointer.Key(), "key", ""))
				b.WriteString(o.ref.Split)
				b.WriteString(o.describeArg(o.nonPointer.Elem(), "value", ""))
			}
		default:
			if doubleDash {
				b.WriteRune('=')
			} else {
				b.WriteRune(' ')
			}
			b.WriteString(o.describeArg(o.nonPointer, o.f.Name, o.ref.ArgName))
		}
	}
	b.WriteString(optional)
	return b.String()
}

func (o opt) describeArg(typ reflect.Type, name string, override string) string {
	if override != "" {
		debugf("argname %s", override)
	}
	switch typ.Kind() {
	case reflect.Slice:
		ed := o.describeArg(typ.Elem(), name, override)
		return ed + o.ref.Split + ed + "..."
	case reflect.Array:
		ed := o.describeArg(typ.Elem(), name, override)
		return strings.Join(repeatString(ed, typ.Len()), o.ref.Split)
	}
	if override != "" {
		return override
	}
	switch typ.Kind() {
	case reflect.Bool:
		return "true|false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "N.N"
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

// Usage produces a usage summary.  It is not called automatically unless
// WithHelpText is used in creation of the flag handler.
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
		ref, _, _, err := parseFlagRef(tagSet.Get(h.tagName), f.Type)
		if err != nil {
			panic(err.Error())
		}
		help := tagSet.Get(h.helpTag).Value
		if help == "" {
			help = fmt.Sprintf("set %s (%s)", f.Name, f.Type)
		}
		nonPointer := reflectutils.NonPointer(f.Type)
		var lead *opt
		for i, n := range ref.Name {
			o := &opt{
				name:       n,
				help:       help,
				f:          f,
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
				if ref.isBool {
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
	usage = append(usage, "Usage: "+os.Args[0])

	switch len(optional[flagOpt]) {
	case 0:
	default:
		if h.combineShort {
			usage = append(usage, " [-flags]")
		} else {
			usage = append(usage, " [flags]")
		}
	}
	usage = append(usage, h.formatOpts(required[flagOpt])...)

	switch len(optional[optionOpt]) {
	case 0:
	default:
		if h.combineShort {
			usage = append(usage, " [-options args]")
		} else {
			usage = append(usage, " [options]")
		}
	}
	usage = append(usage, h.formatOpts(required[optionOpt])...)

	switch len(optional[parameterOpt]) {
	case 0:
	default:
		usage = append(usage, " [parameters]")
	}
	usage = append(usage, h.formatOpts(required[parameterOpt])...)

	switch len(h.subcommands) {
	case 0:
	case 1, 2, 3, 4, 5, 6, 7:
		usage = append(usage, strings.Join(h.subcommandsOrder, "|")+" ")
	default:
		usage = append(usage, " subcommand")
	}

	if h.positionalHelp != "" {
		usage = append(usage, " ", h.positionalHelp)
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
							prependSpace(opt.help),
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

	if h.helpText != nil {
		usage = append(usage, "\n", *h.helpText, "\n")
	}

	return strings.Join(usage, "")
}

//
// program [flags] [--xyz number] subcommand
//
