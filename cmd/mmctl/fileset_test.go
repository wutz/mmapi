package main

import (
	"strings"
	"testing"
)

// The layout tests compare against output captured from mmlsfileset on a
// cluster node, so a change in column positions shows up as a failing test
// rather than as output that no longer lines up with GPFS.

func TestFilesetLineLayout(t *testing.T) {
	var root filesetInfo
	root.FilesetName = "root"
	root.Config.Status = "Linked"
	root.Config.Path = "/fs0"

	got := (&line{}).at(0, root.FilesetName).at(25, root.Config.Status).
		at(35, root.Config.Path).padTo(75).String()
	want := "root                     Linked    /fs0                                    "
	if got != want {
		t.Errorf("fileset row mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	// A name that overruns its column pushes the rest of the row right by a
	// single space, the way mmlsfileset prints CSI's pvc-<uuid> filesets.
	long := "pvc-2d88b637-3579-492c-aa62-310e9863cfc7"
	got = (&line{}).at(0, long).at(25, "Linked").at(35, "/fs0/"+long).padTo(75).String()
	want = long + " Linked /fs0/" + long
	if got != want {
		t.Errorf("long fileset row mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFilesetLongNameLayoutL(t *testing.T) {
	// mmlsfileset -L, row for a fileset whose name overruns the Name column.
	l := &line{}
	l.at(0, "pvc-2d88b637-3579-492c-aa62-310e9863cfc7")
	l.rightAt(34, "6")
	l.rightAt(49, "8388611")
	l.rightAt(59, "0")
	l.at(60, "Wed Jun 17 10:38:18 2026")
	l.rightAt(93, "4")
	l.rightAt(114, "100352")
	l.rightAt(129, "9216")
	l.at(130, "Fileset created by IBM Container Storage Interface driver")

	want := "pvc-2d88b637-3579-492c-aa62-310e9863cfc7 6 8388611        0 Wed Jun 17 10:38:18 2026" +
		"        4               100352           9216 Fileset created by IBM Container Storage Interface driver"
	if got := l.String(); got != want {
		t.Errorf("mmlsfileset -L row mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFilesetInodeCountsOfDependentFileset(t *testing.T) {
	// A dependent fileset has no inode space of its own; the GUI reports its
	// owner's figures, where mmlsfileset reports zero and the fileset's own
	// inode usage.
	var dependent filesetInfo
	dependent.Config.MaxNumInodes = 1225728
	dependent.Usage.AllocatedInodes = 1225728
	dependent.Usage.InodeSpaceFreeInodes = 1221671
	dependent.Usage.InodeSpaceUsedInodes = 4057
	dependent.Usage.UsedInodes = 1

	if got := dependent.maxInodes(); got != 0 {
		t.Errorf("maxInodes() = %d, want 0", got)
	}
	if got := dependent.allocatedInodes(); got != 0 {
		t.Errorf("allocatedInodes() = %d, want 0", got)
	}
	if got := dependent.freeInodes(); got != 0 {
		t.Errorf("freeInodes() = %d, want 0", got)
	}
	if got := dependent.usedInodes(); got != 1 {
		t.Errorf("usedInodes() = %d, want 1", got)
	}

	owner := dependent
	owner.Config.IsInodeSpaceOwner = true
	if got := owner.maxInodes(); got != 1225728 {
		t.Errorf("maxInodes() = %d, want 1225728", got)
	}
	if got := owner.usedInodes(); got != 4057 {
		t.Errorf("usedInodes() = %d, want 4057", got)
	}
}

// mmlsfs prints the attributes it was asked for in the order the options were
// given, except that the mount attributes always come last.
func TestMmlsfsAttributeOrder(t *testing.T) {
	tests := []struct {
		argv []string
		want []string
	}{
		{[]string{"fs0", "-Q", "-B", "-V"}, []string{"-Q", "-Q", "-Q", "-B", "-V"}},
		{[]string{"fs0", "-V", "-B"}, []string{"-V", "-B"}},
		{[]string{"fs0", "-T", "-Q"}, []string{"-Q", "-Q", "-Q", "-T"}},
		{[]string{"fs0", "-T", "-d", "-B"}, []string{"-d", "-B", "-T"}},
	}

	cmd := cmdMmlsfs()
	for _, tt := range tests {
		o, err := parseOptions(cmd.spec, tt.argv)
		if err != nil {
			t.Fatalf("parseOptions(%q) returned %v", tt.argv, err)
		}

		var got []string
		for _, attr := range selectFsAttrs(o) {
			got = append(got, attr.option)
		}
		if strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("mmlsfs %q selected %q, want %q", tt.argv, got, tt.want)
		}
	}

	// No option selects every attribute.
	o, err := parseOptions(cmd.spec, []string{"fs0"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(selectFsAttrs(o)), len(fsAttrs()); got != want {
		t.Errorf("mmlsfs with no option selected %d attributes, want %d", got, want)
	}
}
