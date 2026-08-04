package main

import "testing"

func TestBlockFormatterKB(t *testing.T) {
	blocks, err := newBlockFormatter("")
	if err != nil {
		t.Fatal(err)
	}
	if blocks.header != "KB" {
		t.Errorf("header = %q, want %q", blocks.header, "KB")
	}
	for _, kb := range []int64{0, 512, 83873808} {
		if got, want := blocks.format(kb), num(kb); got != want {
			t.Errorf("format(%d) = %q, want %q", kb, got, want)
		}
	}
}

func TestBlockFormatterAuto(t *testing.T) {
	blocks, err := newBlockFormatter("auto")
	if err != nil {
		t.Fatal(err)
	}
	if blocks.header != "blocks" {
		t.Errorf("header = %q, want %q", blocks.header, "blocks")
	}

	// Values taken from mmlsquota --block-size auto on a cluster node.
	tests := []struct {
		kb   int64
		want string
	}{
		{0, "0"},
		{512, "512K"},
		{83873808, "79.99G"},
		{209715200, "200G"},
		{489408, "477.9M"},
		{1073741824, "1T"},
	}

	for _, tt := range tests {
		if got := blocks.format(tt.kb); got != tt.want {
			t.Errorf("format(%d) = %q, want %q", tt.kb, got, tt.want)
		}
	}
}

func TestBlockFormatterFixedUnit(t *testing.T) {
	blocks, err := newBlockFormatter("1M")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := blocks.format(2097152), "2048"; got != want {
		t.Errorf("format(2097152) = %q, want %q", got, want)
	}
	if _, err := newBlockFormatter("banana"); err == nil {
		t.Error("newBlockFormatter(\"banana\") = nil error, want an error")
	}
}

func TestYEncode(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"Mon May 18 14:56:37 2026", "Mon May 18 14%3A56%3A37 2026"},
		{"100%", "100%25"},
		{"fset1", "fset1"},
		// Only path fields escape slashes; mmlscluster prints its remote shell
		// command unescaped.
		{"/usr/lpp/mmfs/bin/scaleadmremoteexecute", "/usr/lpp/mmfs/bin/scaleadmremoteexecute"},
	}

	for _, tt := range tests {
		if got := yEncode(tt.arg); got != tt.want {
			t.Errorf("yEncode(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestYEncodePath(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"/fs0/fset1", "%2Ffs0%2Ffset1"},
		{"/fs0/pvc-2d88b637", "%2Ffs0%2Fpvc%2D2d88b637"},
	}

	for _, tt := range tests {
		if got := yEncodePath(tt.arg); got != tt.want {
			t.Errorf("yEncodePath(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestGpfsTime(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"2026-05-18 14:56:37,000", "Mon May 18 14:56:37 2026"},
		{"2026-06-11 15:12:38,000", "Thu Jun 11 15:12:38 2026"},
		{"", ""},
		{"not a time", "not a time"},
	}

	for _, tt := range tests {
		if got := gpfsTime(tt.arg); got != tt.want {
			t.Errorf("gpfsTime(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestParseInodeCount(t *testing.T) {
	tests := []struct {
		arg     string
		want    int64
		wantErr bool
	}{
		{arg: "1048576", want: 1048576},
		{arg: "1M", want: 1048576},
		{arg: "50K", want: 51200},
		{arg: "", wantErr: true},
		{arg: "-1", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseInodeCount(tt.arg)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseInodeCount(%q) = %d, want error", tt.arg, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseInodeCount(%q) returned %v", tt.arg, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseInodeCount(%q) = %d, want %d", tt.arg, got, tt.want)
		}
	}
}

func TestNodeDesignation(t *testing.T) {
	// The GUI reports roles in camel case; mmlscluster prints them dashed.
	var node nodeInfo
	node.Roles.Designation = "quorumManager"
	node.Roles.OtherNodeRoles = "perfmonNode"
	if got, want := nodeDesignation(node), "quorum-manager-perfmon"; got != want {
		t.Errorf("nodeDesignation() = %q, want %q", got, want)
	}

	var client nodeInfo
	client.Roles.Designation = "client"
	if got, want := nodeDesignation(client), "client"; got != want {
		t.Errorf("nodeDesignation() = %q, want %q", got, want)
	}
}

func TestFilesetParentID(t *testing.T) {
	// The root fileset has no parent, which mmlsfileset prints as "--".
	var root filesetInfo
	if got, want := root.parentID(), "--"; got != want {
		t.Errorf("parentID() = %q, want %q", got, want)
	}

	var child filesetInfo
	parent := int64(0)
	child.Config.ParentID = &parent
	if got, want := child.parentID(), "0"; got != want {
		t.Errorf("parentID() = %q, want %q", got, want)
	}
}
