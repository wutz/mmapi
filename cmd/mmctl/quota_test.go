package main

import "testing"

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

func TestFormatKB(t *testing.T) {
	tests := []struct {
		kb   int64
		want string
	}{
		{0, "-"},
		{512, "512K"},
		{1024, "1M"},
		{1048576, "1G"},
		{2097152, "2G"},
		{1572864, "1.5G"},
		{1073741824, "1T"},
	}

	for _, tt := range tests {
		if got := formatKB(tt.kb); got != tt.want {
			t.Errorf("formatKB(%d) = %q, want %q", tt.kb, got, tt.want)
		}
	}
}
