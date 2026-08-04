package main

import (
	"errors"
	"testing"
)

func TestParseOptions(t *testing.T) {
	spec := optSpec{
		"-u":           valueOpt,
		"-Y":           flagOpt,
		"-j":           valueOpt,
		"--block":      valueOpt,
		"--block-size": valueOpt,
	}

	tests := []struct {
		name     string
		argv     []string
		vals     map[string]string
		flags    []string
		operands []string
		wantErr  bool
	}{
		{
			name:     "options before operands",
			argv:     []string{"-u", "ubuntu", "-Y", "fs0:fset1"},
			vals:     map[string]string{"-u": "ubuntu"},
			flags:    []string{"-Y"},
			operands: []string{"fs0:fset1"},
		},
		{
			// mm commands accept options after the operands too.
			name:     "options after operands",
			argv:     []string{"fs0", "-j", "fset1"},
			vals:     map[string]string{"-j": "fset1"},
			operands: []string{"fs0"},
		},
		{
			name: "attached and separated values",
			argv: []string{"-uubuntu", "--block=1G:2G", "--block-size", "auto"},
			vals: map[string]string{"-u": "ubuntu", "--block": "1G:2G", "--block-size": "auto"},
		},
		{
			name:     "double dash ends option parsing",
			argv:     []string{"--", "-Y"},
			operands: []string{"-Y"},
		},
		{name: "unknown short option", argv: []string{"-x"}, wantErr: true},
		{name: "unknown long option", argv: []string{"--nope"}, wantErr: true},
		{name: "missing value", argv: []string{"-u"}, wantErr: true},
		{name: "value given to a flag", argv: []string{"-Yes"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseOptions(spec, tt.argv)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOptions(%q) = %+v, want error", tt.argv, o)
				}
				var usageErr *usageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("parseOptions(%q) returned %v, want a usage error", tt.argv, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOptions(%q) returned %v", tt.argv, err)
			}
			for name, want := range tt.vals {
				if !o.has(name) {
					t.Errorf("option %s not set", name)
				}
				if got := o.value(name); got != want {
					t.Errorf("value(%s) = %q, want %q", name, got, want)
				}
			}
			for _, name := range tt.flags {
				if !o.has(name) {
					t.Errorf("flag %s not set", name)
				}
			}
			if len(o.operands) != len(tt.operands) {
				t.Fatalf("operands = %q, want %q", o.operands, tt.operands)
			}
			for i, want := range tt.operands {
				if o.operands[i] != want {
					t.Errorf("operands = %q, want %q", o.operands, tt.operands)
				}
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		arg  string
		want []string
	}{
		{"fs0", []string{"fs0"}},
		{"fs0,fs1", []string{"fs0", "fs1"}},
		{"u1, u2 ,", []string{"u1", "u2"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := splitList(tt.arg)
		if len(got) != len(tt.want) {
			t.Fatalf("splitList(%q) = %q, want %q", tt.arg, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("splitList(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		}
	}
}

func TestDispatch(t *testing.T) {
	tests := []struct {
		argv []string
		name string
		args []string
	}{
		{[]string{"mmctl", "mmlsquota", "fs0"}, "mmlsquota", []string{"fs0"}},
		// A symlink named after the command invokes it directly.
		{[]string{"/usr/local/bin/mmlsquota", "fs0"}, "mmlsquota", []string{"fs0"}},
		{[]string{"mmctl"}, "", nil},
	}

	for _, tt := range tests {
		name, args := dispatch(tt.argv)
		if name != tt.name {
			t.Errorf("dispatch(%q) name = %q, want %q", tt.argv, name, tt.name)
		}
		if len(args) != len(tt.args) {
			t.Errorf("dispatch(%q) args = %q, want %q", tt.argv, args, tt.args)
		}
	}
}

func TestEveryCommandHasUsage(t *testing.T) {
	for _, c := range commands() {
		if c.usage == "" || c.summary == "" || c.run == nil {
			t.Errorf("command %s is incomplete", c.name)
		}
		if lookup(c.name) == nil {
			t.Errorf("command %s is not reachable through lookup", c.name)
		}
	}
}
