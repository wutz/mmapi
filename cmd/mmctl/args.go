package main

import (
	"fmt"
	"strings"
)

// GPFS mm* commands accept single-dash options that are either boolean (-Y,
// -L) or take a value (-u User, -j Fileset), plus double-dash long options
// (--block 1G:2G). Options may appear before, between, or after the operands:
// "mmlsfileset fs0 -L" is as valid as "mmlsfileset -L fs0". Each command
// declares its own spec because the same letter differs between commands —
// -u takes a user name in mmlsquota but is a boolean type selector in
// mmrepquota.

type optKind int

const (
	flagOpt optKind = iota
	valueOpt
)

type optSpec map[string]optKind

type options struct {
	set  map[string]bool
	vals map[string]string
	// order lists the options in the order they were given. mmlsfs prints the
	// attributes it was asked for in exactly that order.
	order    []string
	operands []string
}

func (o options) has(name string) bool { return o.set[name] }

// given lists the options that were used, in command line order.
func (o options) given() []string { return o.order }

func (o options) value(name string) string { return o.vals[name] }

// usageError is an operator mistake — an unknown option, a missing operand.
// mm commands answer those with a one-line diagnostic followed by the command
// synopsis, which is what fatal does with this type.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

func incorrectOption(opt string) error {
	return usagef("Incorrect option: %s", opt)
}

// mark records that an option was given, keeping the command line order.
func (o *options) mark(name string) {
	if !o.set[name] {
		o.order = append(o.order, name)
	}
	o.set[name] = true
}

func parseOptions(spec optSpec, argv []string) (options, error) {
	o := options{set: map[string]bool{}, vals: map[string]string{}}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]

		switch {
		case arg == "--":
			o.operands = append(o.operands, argv[i+1:]...)
			return o, nil

		case strings.HasPrefix(arg, "--"):
			name, val, hasVal := strings.Cut(arg, "=")
			kind, ok := spec[name]
			if !ok {
				return o, incorrectOption(name)
			}
			if kind == flagOpt {
				if hasVal {
					return o, usagef("Option %s does not take a value.", name)
				}
				o.mark(name)
				continue
			}
			if !hasVal {
				i++
				if i >= len(argv) {
					return o, usagef("Missing argument for option %s.", name)
				}
				val = argv[i]
			}
			o.mark(name)
			o.vals[name] = val

		case len(arg) > 1 && arg[0] == '-':
			name := arg[:2]
			kind, ok := spec[name]
			if !ok {
				return o, incorrectOption(arg)
			}
			if kind == flagOpt {
				if len(arg) > 2 {
					return o, incorrectOption(arg)
				}
				o.mark(name)
				continue
			}
			// Both "-u root" and "-uroot" are accepted, as mm commands do.
			val := arg[2:]
			if val == "" {
				i++
				if i >= len(argv) {
					return o, usagef("Missing argument for option %s.", name)
				}
				val = argv[i]
			}
			o.mark(name)
			o.vals[name] = val

		default:
			o.operands = append(o.operands, arg)
		}
	}

	return o, nil
}

// splitList splits an "a,b,c" option value, the form mm commands use for
// multi-valued options such as mmsetquota --user or mmlsfileset's fileset
// operand.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
