package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type filesystemInfo struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Version    string `json:"version"`
	CreateTime string `json:"createTime"`
	Block      struct {
		BlockSize           int64  `json:"blockSize"`
		Disks               string `json:"disks"`
		IndirectBlockSize   int64  `json:"indirectBlockSize"`
		InodeSize           int64  `json:"inodeSize"`
		LogfileSize         int64  `json:"logfileSize"`
		MinFragmentSize     int64  `json:"minFragmentSize"`
		Pools               string `json:"pools"`
		WriteCacheThreshold int64  `json:"writeCacheThreshold"`
	} `json:"block"`
	Mount struct {
		AdditionalMountOptions string `json:"additionalMountOptions"`
		AutomaticMountOption   string `json:"automaticMountOption"`
		MountPoint             string `json:"mountPoint"`
		MountPriority          int64  `json:"mountPriority"`
		Status                 string `json:"status"`
	} `json:"mount"`
	Replication struct {
		DefaultDataReplicas     int64  `json:"defaultDataReplicas"`
		DefaultMetadataReplicas int64  `json:"defaultMetadataReplicas"`
		LogReplicas             int64  `json:"logReplicas"`
		MaxDataReplicas         int64  `json:"maxDataReplicas"`
		MaxMetadataReplicas     int64  `json:"maxMetadataReplicas"`
		StrictReplication       string `json:"strictReplication"`
	} `json:"replication"`
	Quota struct {
		DefaultQuotasEnabled    string `json:"defaultQuotasEnabled"`
		FilesetdfEnabled        bool   `json:"filesetdfEnabled"`
		PerfilesetQuotas        bool   `json:"perfilesetQuotas"`
		QuotasAccountingEnabled string `json:"quotasAccountingEnabled"`
		QuotasEnforced          string `json:"quotasEnforced"`
	} `json:"quota"`
	Settings struct {
		ACLSemantics         string `json:"aclSemantics"`
		BlockAllocationType  string `json:"blockAllocationType"`
		DMAPIEnabled         bool   `json:"dmapiEnabled"`
		Encryption           bool   `json:"encryption"`
		ExactMTime           bool   `json:"exactMTime"`
		FastEAEnabled        bool   `json:"fastEAEnabled"`
		FileAuditLogEnabled  bool   `json:"fileAuditLogEnabled"`
		FileLockingSemantics string `json:"fileLockingSemantics"`
		Is4KAligned          bool   `json:"is4KAligned"`
		MaxNumberOfInodes    int64  `json:"maxNumberOfInodes"`
		NumNodes             int64  `json:"numNodes"`
		RapidRepairEnabled   bool   `json:"rapidRepairEnabled"`
		SuppressATime        string `json:"suppressATime"`
	} `json:"settings"`
}

// fsAttr is one row of mmlsfs output. option is the command line option that
// selects the row, flag what the flag column shows (blank for the continuation
// rows of -Q), and field the name used by -Y.
type fsAttr struct {
	option string
	flag   string
	field  string
	desc   string
	value  func(*filesystemInfo) string
}

// mountOptions are the mount attributes, which mmlsfs prints after all the
// others no matter where their options appeared on the command line.
var mountOptions = map[string]bool{"-A": true, "-o": true, "-T": true, "--mount-priority": true}

// fsAttrs lists the attributes the GUI exposes, in mmlsfs order. Attributes
// mmlsfs prints from the local daemon but the REST API does not report (for
// instance --subblocks-per-full-block) are absent rather than shown empty.
func fsAttrs() []fsAttr {
	return []fsAttr{
		{"-f", "-f", "minFragmentSize", "Minimum fragment (subblock) size in bytes", func(f *filesystemInfo) string { return num(f.Block.MinFragmentSize) }},
		{"-i", "-i", "inodeSize", "Inode size in bytes", func(f *filesystemInfo) string { return num(f.Block.InodeSize) }},
		{"-I", "-I", "indirectBlockSize", "Indirect block size in bytes", func(f *filesystemInfo) string { return num(f.Block.IndirectBlockSize) }},
		{"-m", "-m", "defaultMetadataReplicas", "Default number of metadata replicas", func(f *filesystemInfo) string { return num(f.Replication.DefaultMetadataReplicas) }},
		{"-M", "-M", "maxMetadataReplicas", "Maximum number of metadata replicas", func(f *filesystemInfo) string { return num(f.Replication.MaxMetadataReplicas) }},
		{"-r", "-r", "defaultDataReplicas", "Default number of data replicas", func(f *filesystemInfo) string { return num(f.Replication.DefaultDataReplicas) }},
		{"-R", "-R", "maxDataReplicas", "Maximum number of data replicas", func(f *filesystemInfo) string { return num(f.Replication.MaxDataReplicas) }},
		{"-j", "-j", "blockAllocationType", "Block allocation type", func(f *filesystemInfo) string { return f.Settings.BlockAllocationType }},
		{"-D", "-D", "fileLockingSemantics", "File locking semantics in effect", func(f *filesystemInfo) string { return f.Settings.FileLockingSemantics }},
		{"-k", "-k", "ACLSemantics", "ACL semantics in effect", func(f *filesystemInfo) string { return f.Settings.ACLSemantics }},
		{"-n", "-n", "numNodes", "Estimated number of nodes that will mount file system", func(f *filesystemInfo) string { return num(f.Settings.NumNodes) }},
		{"-B", "-B", "blockSize", "Block size", func(f *filesystemInfo) string { return num(f.Block.BlockSize) }},
		{"-Q", "-Q", "quotasAccountingEnabled", "Quotas accounting enabled", func(f *filesystemInfo) string { return f.Quota.QuotasAccountingEnabled }},
		{"-Q", "", "quotasEnforced", "Quotas enforced", func(f *filesystemInfo) string { return f.Quota.QuotasEnforced }},
		{"-Q", "", "defaultQuotasEnabled", "Default quotas enabled", func(f *filesystemInfo) string { return f.Quota.DefaultQuotasEnabled }},
		{"--perfileset-quota", "--perfileset-quota", "perfilesetQuotas", "Per-fileset quota enforcement", func(f *filesystemInfo) string { return yesNo(f.Quota.PerfilesetQuotas) }},
		{"--filesetdf", "--filesetdf", "filesetdfEnabled", "Fileset df enabled?", func(f *filesystemInfo) string { return yesNo(f.Quota.FilesetdfEnabled) }},
		{"-V", "-V", "filesystemVersion", "File system version", func(f *filesystemInfo) string { return f.Version }},
		{"--create-time", "--create-time", "create-time", "File system creation time", func(f *filesystemInfo) string { return gpfsTime(f.CreateTime) }},
		{"-z", "-z", "DMAPIEnabled", "Is DMAPI enabled?", func(f *filesystemInfo) string { return yesNo(f.Settings.DMAPIEnabled) }},
		{"-L", "-L", "logfileSize", "Logfile size", func(f *filesystemInfo) string { return num(f.Block.LogfileSize) }},
		{"-E", "-E", "exactMtime", "Exact mtime mount option", func(f *filesystemInfo) string { return yesNo(f.Settings.ExactMTime) }},
		{"-S", "-S", "suppressAtime", "Suppress atime mount option", func(f *filesystemInfo) string { return f.Settings.SuppressATime }},
		{"-K", "-K", "strictReplication", "Strict replica allocation option", func(f *filesystemInfo) string { return f.Replication.StrictReplication }},
		{"--fastea", "--fastea", "fastEAenabled", "Fast external attributes enabled?", func(f *filesystemInfo) string { return yesNo(f.Settings.FastEAEnabled) }},
		{"--encryption", "--encryption", "encryption", "Encryption enabled?", func(f *filesystemInfo) string { return yesNo(f.Settings.Encryption) }},
		{"--inode-limit", "--inode-limit", "maxNumberOfInodes", "Maximum number of inodes in all inode spaces", func(f *filesystemInfo) string { return num(f.Settings.MaxNumberOfInodes) }},
		{"--uid", "--uid", "UID", "File system UID", func(f *filesystemInfo) string { return f.UUID }},
		{"--log-replicas", "--log-replicas", "logReplicas", "Number of log replicas", func(f *filesystemInfo) string { return num(f.Replication.LogReplicas) }},
		{"--is4KAligned", "--is4KAligned", "is4KAligned", "is4KAligned?", func(f *filesystemInfo) string { return yesNo(f.Settings.Is4KAligned) }},
		{"--rapid-repair", "--rapid-repair", "rapidRepairEnabled", "rapidRepair enabled?", func(f *filesystemInfo) string { return yesNo(f.Settings.RapidRepairEnabled) }},
		{"--write-cache-threshold", "--write-cache-threshold", "write-cache-threshold", "HAWC Threshold (max 65536)", func(f *filesystemInfo) string { return num(f.Block.WriteCacheThreshold) }},
		{"-P", "-P", "storagePools", "Disk storage pools in file system", func(f *filesystemInfo) string { return f.Block.Pools }},
		{"--file-audit-log", "--file-audit-log", "file-audit-log", "File Audit Logging enabled?", func(f *filesystemInfo) string { return yesNo(f.Settings.FileAuditLogEnabled) }},
		{"-d", "-d", "disks", "Disks in file system", func(f *filesystemInfo) string { return f.Block.Disks }},
		{"-A", "-A", "automaticMountOption", "Automatic mount option", func(f *filesystemInfo) string { return f.Mount.AutomaticMountOption }},
		{"-o", "-o", "additionalMountOptions", "Additional mount options", func(f *filesystemInfo) string { return f.Mount.AdditionalMountOptions }},
		{"-T", "-T", "defaultMountPoint", "Default mount point", func(f *filesystemInfo) string { return f.Mount.MountPoint }},
		{"--mount-priority", "--mount-priority", "mountPriority", "Mount priority", func(f *filesystemInfo) string { return num(f.Mount.MountPriority) }},
	}
}

func num(n int64) string { return strconv.FormatInt(n, 10) }

func cmdMmlsfs() *command {
	spec := optSpec{"-Y": flagOpt}
	for _, attr := range fsAttrs() {
		spec[attr.option] = flagOpt
	}

	return &command{
		name:    "mmlsfs",
		summary: "Display file system attributes",
		usage: `Usage:
  mmlsfs {Device | all} [-A] [-B] [-d] [-D] [-E] [-f] [-i] [-I] [-j] [-k]
         [-K] [-L] [-m] [-M] [-n] [-o] [-P] [-Q] [-r] [-R] [-S] [-T] [-V]
         [-Y] [-z] [--create-time] [--encryption] [--fastea] [--file-audit-log]
         [--filesetdf] [--inode-limit] [--is4KAligned] [--log-replicas]
         [--mount-priority] [--perfileset-quota] [--rapid-repair] [--uid]
         [--write-cache-threshold]`,
		spec: spec,
		run:  runMmlsfs,
	}
}

func queryAllFields() url.Values {
	return url.Values{"fields": []string{":all:"}}
}

func runMmlsfs(o options) error {
	if len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}

	filesystems, err := listFilesystems()
	if err != nil {
		return err
	}

	var wanted []*filesystemInfo
	titled := false
	for _, operand := range o.operands {
		if operand == "all" || operand == "all_local" {
			// mmlsfs heads each file system with its name when asked for "all",
			// and prints the bare attribute table for a named device.
			titled = true
			for i := range filesystems {
				wanted = append(wanted, &filesystems[i])
			}
			continue
		}
		fs := findFilesystem(filesystems, operand)
		if fs == nil {
			return fmt.Errorf("File system %s is not known to the GPFS cluster.", operand)
		}
		wanted = append(wanted, fs)
	}
	if len(wanted) > 1 {
		titled = true
	}

	selected := selectFsAttrs(o)

	if o.has("-Y") {
		y := newYRecord("mmlsfs", "", "deviceName", "fieldName", "data", "remarks")
		// The mount point is a path, and mm commands escape those more heavily
		// than other values.
		yPath := newYRecord("mmlsfs", "", "deviceName", "fieldName", "data", "remarks").
			withPathFields("data")
		y.printHeader()
		for _, fs := range wanted {
			for _, attr := range selected {
				record := y
				if attr.option == "-T" {
					record = yPath
				}
				record.printRow(fs.Name, attr.field, attr.value(fs), "")
			}
		}
		return nil
	}

	for _, fs := range wanted {
		if titled {
			title := fmt.Sprintf("File system attributes for /dev/%s:", fs.Name)
			fmt.Println()
			fmt.Println(title)
			fmt.Println(strings.Repeat("=", len(title)))
		}
		fmt.Println((&line{}).at(0, "flag").at(20, "value").at(45, "description").String())
		fmt.Printf("%s %s %s\n", strings.Repeat("-", 19), strings.Repeat("-", 24), strings.Repeat("-", 35))
		for _, attr := range selected {
			fmt.Println((&line{}).at(1, attr.flag).at(20, attr.value(fs)).at(45, attr.desc).String())
		}
	}
	return nil
}

// selectFsAttrs picks the attributes to print. mmlsfs prints those it was
// asked for in the order the options were given — with the mount attributes
// last — and all of them when no attribute option was given.
func selectFsAttrs(o options) []fsAttr {
	attrs := fsAttrs()
	var selected, mountAttrs []fsAttr
	for _, option := range o.given() {
		for _, attr := range attrs {
			if attr.option != option {
				continue
			}
			if mountOptions[attr.option] {
				mountAttrs = append(mountAttrs, attr)
			} else {
				selected = append(selected, attr)
			}
		}
	}
	selected = append(selected, mountAttrs...)
	if len(selected) == 0 {
		return attrs
	}
	return selected
}

func listFilesystems() ([]filesystemInfo, error) {
	var resp struct {
		Filesystems []filesystemInfo `json:"filesystems"`
	}
	if err := scaleGetInto("filesystems", queryAllFields(), &resp); err != nil {
		return nil, err
	}
	return resp.Filesystems, nil
}

func findFilesystem(filesystems []filesystemInfo, name string) *filesystemInfo {
	for i := range filesystems {
		if filesystems[i].Name == name {
			return &filesystems[i]
		}
	}
	return nil
}

// getFilesystem fetches one filesystem, which several commands need for the
// mount point that anchors default junction paths.
func getFilesystem(name string) (*filesystemInfo, error) {
	var resp struct {
		Filesystems []filesystemInfo `json:"filesystems"`
	}
	if err := scaleGetInto("filesystems/"+name, queryAllFields(), &resp); err != nil {
		return nil, err
	}
	if len(resp.Filesystems) == 0 {
		return nil, fmt.Errorf("File system %s is not known to the GPFS cluster.", name)
	}
	return &resp.Filesystems[0], nil
}
