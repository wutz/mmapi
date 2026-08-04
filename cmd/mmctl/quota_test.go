package main

import (
	"strings"
	"testing"
)

func TestParseQuotaTarget(t *testing.T) {
	tests := []struct {
		arg     string
		fs      string
		fileset string
		wantErr bool
	}{
		{arg: "fs0", fs: "fs0"},
		{arg: "fs0:fset1", fs: "fs0", fileset: "fset1"},
		{arg: "fs0:pvc-2d88b637-3579-492c-aa62-310e9863cfc7", fs: "fs0", fileset: "pvc-2d88b637-3579-492c-aa62-310e9863cfc7"},
		{arg: "", wantErr: true},
		{arg: ":fset1", wantErr: true},
		{arg: "fs0:", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseQuotaTarget(tt.arg)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseQuotaTarget(%q) = %+v, want error", tt.arg, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseQuotaTarget(%q) returned %v", tt.arg, err)
			continue
		}
		if got.fs != tt.fs || got.fileset != tt.fileset {
			t.Errorf("parseQuotaTarget(%q) = %+v, want fs=%q fileset=%q", tt.arg, got, tt.fs, tt.fileset)
		}
	}
}

func TestQuotaTargetPath(t *testing.T) {
	tests := []struct {
		target quotaTarget
		want   string
	}{
		{quotaTarget{fs: "fs0"}, "filesystems/fs0/quotas"},
		{quotaTarget{fs: "fs0", fileset: "fset1"}, "filesystems/fs0/filesets/fset1/quotas"},
	}

	for _, tt := range tests {
		if got := tt.target.path(); got != tt.want {
			t.Errorf("%+v.path() = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestLimitPair(t *testing.T) {
	tests := []struct {
		arg  string
		soft string
		hard string
	}{
		{"10G:20G", "10G", "20G"},
		{"10G", "10G", "0"}, // an omitted hard limit means no limit
		{"0:0", "0", "0"},
		{"", "0", "0"},
	}

	for _, tt := range tests {
		soft, hard := limitPair(tt.arg)
		if soft != tt.soft || hard != tt.hard {
			t.Errorf("limitPair(%q) = %q, %q, want %q, %q", tt.arg, soft, hard, tt.soft, tt.hard)
		}
	}
}

// The layout tests below compare against output captured from mmlsquota and
// mmrepquota on a cluster node, so a change in column widths shows up as a
// failing test rather than as output that no longer lines up with GPFS.

func TestQuotaLayoutFilesetQuota(t *testing.T) {
	blocks, err := newBlockFormatter("")
	if err != nil {
		t.Fatal(err)
	}

	entry := quotaEntry{
		QuotaType: "FILESET", ObjectName: "fset1", FilesystemName: "fs0",
		BlockUsage: 83873808, BlockQuota: 209715200, BlockLimit: 209715200,
		BlockInDoubt: 489408, BlockGrace: "none",
		FilesUsage: 7, FilesQuota: 2048000, FilesLimit: 2048000,
		FilesInDoubt: 346, FilesGrace: "none",
	}

	wantHeader := "                         Block Limits                                    |     File Limits\n" +
		"Filesystem type             KB      quota      limit   in_doubt    grace |    files   quota    limit in_doubt    grace  Remarks"
	if got := quotaHeader(blocks, "Filesystem", "", true); got != wantHeader {
		t.Errorf("quotaHeader mismatch:\ngot:\n%s\nwant:\n%s", got, wantHeader)
	}

	wantRow := "fs0        FILESET    83873808  209715200  209715200     489408     none |        7 2048000  2048000      346     none "
	if got := quotaRow(entry, blocks, false, entry.FilesystemName, true); got != wantRow {
		t.Errorf("quotaRow mismatch:\ngot:  %q\nwant: %q", got, wantRow)
	}
}

func TestQuotaLayoutUserQuota(t *testing.T) {
	blocks, err := newBlockFormatter("")
	if err != nil {
		t.Fatal(err)
	}

	entry := quotaEntry{
		QuotaType: "USR", ObjectName: "root", FilesetName: "fset1", FilesystemName: "fs0",
		BlockUsage: 20971520, BlockGrace: "none", FilesGrace: "none",
	}

	wantHeader := "                         Block Limits                                               |     File Limits\n" +
		"Filesystem Fileset    type             KB      quota      limit   in_doubt    grace |    files   quota    limit in_doubt    grace  Remarks"
	if got := quotaHeader(blocks, "Filesystem", "Fileset", true); got != wantHeader {
		t.Errorf("quotaHeader mismatch:\ngot:\n%s\nwant:\n%s", got, wantHeader)
	}

	wantRow := "fs0        fset1      USR        20971520          0          0          0     none |        0       0        0        0     none "
	if got := quotaRow(entry, blocks, true, entry.FilesystemName, true); got != wantRow {
		t.Errorf("quotaRow mismatch:\ngot:  %q\nwant: %q", got, wantRow)
	}
}

func TestQuotaLayoutRepquota(t *testing.T) {
	blocks, err := newBlockFormatter("")
	if err != nil {
		t.Fatal(err)
	}

	entry := quotaEntry{
		QuotaType: "USR", ObjectName: "root", FilesetName: "fset1", FilesystemName: "fs0",
		BlockUsage: 20971520, BlockGrace: "none", FilesGrace: "none",
	}

	wantHeader := "Name       fileset    type             KB      quota      limit   in_doubt    grace |    files   quota    limit in_doubt    grace"
	header := quotaHeader(blocks, "Name", "fileset", false)
	_, got, _ := strings.Cut(header, "\n")
	if got != wantHeader {
		t.Errorf("quotaHeader mismatch:\ngot:  %q\nwant: %q", got, wantHeader)
	}

	wantRow := "root       fset1      USR        20971520          0          0          0     none |        0       0        0        0     none"
	if got := quotaRow(entry, blocks, true, entry.ObjectName, false); got != wantRow {
		t.Errorf("quotaRow mismatch:\ngot:  %q\nwant: %q", got, wantRow)
	}
}

func TestQuotaEntryFileset(t *testing.T) {
	// File system scoped quotas belong to the root fileset, which is how mm
	// commands report them.
	if got := (quotaEntry{}).fileset(); got != "root" {
		t.Errorf("fileset() = %q, want %q", got, "root")
	}
	if got := (quotaEntry{FilesetName: "fset1"}).fileset(); got != "fset1" {
		t.Errorf("fileset() = %q, want %q", got, "fset1")
	}
}

func TestQuotaEntryMatches(t *testing.T) {
	entry := quotaEntry{ObjectName: "ubuntu", ObjectID: 1000}
	for _, object := range []string{"ubuntu", "1000"} {
		if !entry.matches(object) {
			t.Errorf("matches(%q) = false, want true", object)
		}
	}
	if entry.matches("root") {
		t.Error(`matches("root") = true, want false`)
	}
}
