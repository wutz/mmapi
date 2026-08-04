package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type filesetInfo struct {
	FilesetName    string `json:"filesetName"`
	FilesystemName string `json:"filesystemName"`
	Config         struct {
		Comment              string `json:"comment"`
		Created              string `json:"created"`
		IAMMode              string `json:"iamMode"`
		ID                   int64  `json:"id"`
		InodeSpace           int64  `json:"inodeSpace"`
		InodeSpaceMask       int64  `json:"inodeSpaceMask"`
		IsInodeSpaceOwner    bool   `json:"isInodeSpaceOwner"`
		MaxNumInodes         int64  `json:"maxNumInodes"`
		ParentID             *int64 `json:"parentId"`
		Path                 string `json:"path"`
		PermissionChangeMode string `json:"permissionChangeMode"`
		RootInode            int64  `json:"rootInode"`
		SnapID               int64  `json:"snapId"`
		Status               string `json:"status"`
	} `json:"config"`
	Usage struct {
		AllocatedInodes      int64 `json:"allocatedInodes"`
		InodeSpaceFreeInodes int64 `json:"inodeSpaceFreeInodes"`
		InodeSpaceUsedInodes int64 `json:"inodeSpaceUsedInodes"`
		UsedBytes            int64 `json:"usedBytes"`
		UsedInodes           int64 `json:"usedInodes"`
	} `json:"usage"`
}

func (f *filesetInfo) parentID() string {
	if f.Config.ParentID == nil {
		return "--"
	}
	return num(*f.Config.ParentID)
}

// dataKB reports fileset usage the way mmlsfileset -d does, in KiB.
func (f *filesetInfo) dataKB() int64 { return f.Usage.UsedBytes / 1024 }

// allocatedInodes and maxInodes report the fileset's own inode space. A
// dependent fileset has none of its own — the GUI reports its owner's figures,
// where mmlsfileset reports zero.
func (f *filesetInfo) allocatedInodes() int64 {
	if !f.Config.IsInodeSpaceOwner {
		return 0
	}
	return f.Usage.AllocatedInodes
}

func (f *filesetInfo) maxInodes() int64 {
	if !f.Config.IsInodeSpaceOwner {
		return 0
	}
	return f.Config.MaxNumInodes
}

// freeInodes counts the inodes still free in the fileset's own inode space; a
// dependent fileset has none of its own.
func (f *filesetInfo) freeInodes() int64 {
	if !f.Config.IsInodeSpaceOwner {
		return 0
	}
	return f.Usage.InodeSpaceFreeInodes
}

// usedInodes counts what mmlsfileset -i counts: the inodes used in the inode
// space owned by the fileset, or the fileset's own inodes when it shares
// another fileset's inode space.
func (f *filesetInfo) usedInodes() int64 {
	if !f.Config.IsInodeSpaceOwner {
		return f.Usage.UsedInodes
	}
	return f.Usage.InodeSpaceUsedInodes
}

func cmdMmlsfileset() *command {
	return &command{
		name:    "mmlsfileset",
		summary: "Display filesets in a file system",
		usage: `Usage:
  mmlsfileset Device [Fileset[,Fileset...]] [-d] [-i] [-L] [-Y]`,
		spec: optSpec{
			"-d": flagOpt,
			"-i": flagOpt,
			"-L": flagOpt,
			"-Y": flagOpt,
		},
		run: runMmlsfileset,
	}
}

func runMmlsfileset(o options) error {
	if len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}
	device := o.operands[0]

	var wantedNames []string
	for _, operand := range o.operands[1:] {
		wantedNames = append(wantedNames, splitList(operand)...)
	}

	filesets, err := listFilesets(device)
	if err != nil {
		return err
	}

	selected := filesets
	if len(wantedNames) > 0 {
		selected = nil
		for _, name := range wantedNames {
			fileset := findFileset(filesets, name)
			if fileset == nil {
				return fmt.Errorf("Fileset name %s not found.", name)
			}
			selected = append(selected, *fileset)
		}
	}

	if o.has("-Y") {
		printFilesetsY(selected, o.has("-i") || o.has("-d"))
		return nil
	}

	if o.has("-i") || o.has("-d") {
		fmt.Println("Collecting fileset usage information ...")
	}
	fmt.Printf("Filesets in file system '%s':\n", device)

	switch {
	case o.has("-L"):
		fmt.Println("Name                            Id      RootInode  ParentId Created                      InodeSpace      MaxInodes    AllocInodes Comment")
		for i := range selected {
			f := &selected[i]
			l := &line{}
			l.at(0, f.FilesetName)
			l.rightAt(34, num(f.Config.ID))
			l.rightAt(49, num(f.Config.RootInode))
			l.rightAt(59, f.parentID())
			l.at(60, gpfsTime(f.Config.Created))
			l.rightAt(93, num(f.Config.InodeSpace))
			l.rightAt(114, num(f.maxInodes()))
			l.rightAt(129, num(f.allocatedInodes()))
			l.at(130, f.Config.Comment)
			fmt.Println(l.String())
		}

	case o.has("-i"), o.has("-d"):
		header := &line{}
		header.at(0, "Name").at(25, "Status").at(35, "Path")
		if o.has("-i") {
			header.at(80, "InodeSpace").at(96, "MaxInodes").at(109, "AllocInodes").at(125, "UsedInodes")
		}
		if o.has("-d") {
			if o.has("-i") {
				header.at(138, "Data (in KB)")
			} else {
				header.at(78, "Data (in KB)")
			}
		}
		fmt.Println(header.String())

		for i := range selected {
			f := &selected[i]
			l := &line{}
			l.at(0, f.FilesetName).at(25, f.Config.Status).at(35, f.Config.Path)
			if o.has("-i") {
				l.rightAt(84, num(f.Config.InodeSpace))
				l.rightAt(105, num(f.maxInodes()))
				l.rightAt(120, num(f.allocatedInodes()))
				l.rightAt(135, num(f.usedInodes()))
			}
			if o.has("-d") {
				if o.has("-i") {
					l.rightAt(150, num(f.dataKB()))
				} else {
					l.rightAt(90, num(f.dataKB()))
				}
			}
			fmt.Println(l.String())
		}

	default:
		fmt.Println((&line{}).at(0, "Name").at(25, "Status").at(35, "Path").padTo(75).String())
		for i := range selected {
			f := &selected[i]
			l := &line{}
			l.at(0, f.FilesetName).at(25, f.Config.Status).at(35, f.Config.Path).padTo(75)
			fmt.Println(l.String())
		}
	}
	return nil
}

// filesetYFields is mmlsfileset's -Y field list, in its order. Consumers look
// fields up by name, so mmctl emits the whole list and fills the fields the
// GUI reports; the rest carry the "-" that mmlsfileset itself prints for an
// attribute that does not apply.
var filesetYFields = strings.Split(
	"filesystemName:filesetName:id:rootInode:status:path:parentId:created:inodes:dataInKB:"+
		"comment:filesetMode:afmTarget:afmState:afmMode:afmFileLookupRefreshInterval:"+
		"afmFileOpenRefreshInterval:afmDirLookupRefreshInterval:afmDirOpenRefreshInterval:"+
		"afmAsyncDelay:afmNeedsRecovery:afmExpirationTimeout:afmRPO:afmLastPSnapId:inodeSpace:"+
		"isInodeSpaceOwner:maxInodes:allocInodes:inodeSpaceMask:afmShowHomeSnapshots:"+
		"afmNumReadThreads:reserved:afmReadBufferSize:afmWriteBufferSize:afmReadSparseThreshold:"+
		"afmParallelReadChunkSize:afmParallelReadThreshold:snapId:afmNumFlushThreads:"+
		"afmPrefetchThreshold:afmEnableAutoEviction:permChangeFlag:afmParallelWriteThreshold:"+
		"freeInodes:afmNeedsResync:afmParallelWriteChunkSize:afmNumWriteThreads:afmPrimaryID:"+
		"afmDRState:afmAssociatedPrimaryId:afmDIO:afmGatewayNode:afmIOFlags:afmVerifyDmapi:"+
		"afmSkipHomeACL:afmSkipHomeMtimeNsec:afmForceCtimeChange:afmSkipResyncRecovery:"+
		"afmSkipConflictQDrop:afmRefreshAsync:afmParallelMounts:afmRefreshOnce:"+
		"afmSkipHomeCtimeNsec:afmReaddirOnce:afmResyncVer2:afmSnapUncachedRead:afmFastCreate:"+
		"afmObjectXattr:afmObjectVHB:afmObjectNoDirectoryObj:afmSkipHomeRefresh:afmObjectGCS:"+
		"afmObjectUserKeys:afmWriteOnClose:afmObjectSSL:afmObjectACL:afmMUPromoted:"+
		"afmMUAutoRemove:afmObjectFastReaddir:afmObjectBlkIO:preventSnapshotRestore:"+
		"permInheritFlag:afmIOFlags2:afmRemoteUpdate:afmDisableReaddirOpt:afmObjectFastReaddir2:"+
		"afmObjDisableLargeAttr:afmRecoveryVer2:afmFailoverMap:afmSyncReadMount:afmObjectBlkIO:"+
		"afmObjectLazyMigrate:afmObjectAZ:afmObjectPreferDir:afmEvictRange:afmRecoveryUseFset:"+
		"afmCheckExtendedACL:afmNFSV4:afmNoCheckRefreshDisable:afmSyncNFSV4ACL:"+
		"afmObjectSyncOpenFiles:afmMULocalRemove:afmObjSCGlacier:afmDisallowRename:"+
		"afmSkipUnknownNFSV4Ids:afmObjectSQS:afmParallelUMounts:afmMigrate:falStatus", ":")

func printFilesetsY(filesets []filesetInfo, usage bool) {
	y := newYRecord("mmlsfileset", "", filesetYFields...).withPathFields("path")
	y.printHeader()

	for i := range filesets {
		f := &filesets[i]
		// mmlsfileset only fills the usage fields when asked for them with -i
		// or -d; without those options it prints "-".
		inodes, dataInKB := "-", "-"
		if usage {
			inodes, dataInKB = num(f.usedInodes()), num(f.dataKB())
		}

		values := map[string]string{
			"filesystemName":    f.FilesystemName,
			"filesetName":       f.FilesetName,
			"id":                num(f.Config.ID),
			"rootInode":         num(f.Config.RootInode),
			"status":            f.Config.Status,
			"path":              f.Config.Path,
			"parentId":          f.parentID(),
			"created":           gpfsTime(f.Config.Created),
			"inodes":            inodes,
			"dataInKB":          dataInKB,
			"comment":           f.Config.Comment,
			"filesetMode":       f.Config.IAMMode,
			"inodeSpace":        num(f.Config.InodeSpace),
			"isInodeSpaceOwner": boolDigit(f.Config.IsInodeSpaceOwner),
			"maxInodes":         num(f.maxInodes()),
			"allocInodes":       num(f.allocatedInodes()),
			"inodeSpaceMask":    num(f.Config.InodeSpaceMask),
			"snapId":            num(f.Config.SnapID),
			"permChangeFlag":    f.Config.PermissionChangeMode,
			"freeInodes":        num(f.freeInodes()),
		}

		row := make([]string, len(filesetYFields))
		for i, field := range filesetYFields {
			value, ok := values[field]
			if !ok {
				value = "-"
			}
			row[i] = value
		}
		y.printRow(row...)
	}
}

// boolDigit renders the 1/0 flags of -Y output, which use digits rather than
// the yes/no of the human-readable listings.
func boolDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func listFilesets(device string) ([]filesetInfo, error) {
	var resp struct {
		Filesets []filesetInfo `json:"filesets"`
	}
	if err := scaleGetInto("filesystems/"+device+"/filesets", queryAllFields(), &resp); err != nil {
		return nil, err
	}
	return resp.Filesets, nil
}

func findFileset(filesets []filesetInfo, name string) *filesetInfo {
	for i := range filesets {
		if filesets[i].FilesetName == name {
			return &filesets[i]
		}
	}
	return nil
}

func cmdMmcrfileset() *command {
	return &command{
		name:    "mmcrfileset",
		summary: "Create a fileset",
		usage: `Usage:
  mmcrfileset Device FilesetName [-t Comment] [-J JunctionPath]
     [--inode-space {new | ExistingFileset}] [--inode-limit MaxNumInodes]
     [--allow-permission-change PermissionChangeMode]`,
		spec: optSpec{
			"-t":                        valueOpt,
			"-J":                        valueOpt,
			"--inode-space":             valueOpt,
			"--inode-limit":             valueOpt,
			"--allow-permission-change": valueOpt,
		},
		run: runMmcrfileset,
	}
}

func runMmcrfileset(o options) error {
	// mmcrfileset always names the new fileset; -J is the junction to link it
	// at, not a way of identifying it.
	if len(o.operands) < 2 {
		return usagef("Missing arguments.")
	}
	device, fileset := o.operands[0], o.operands[1]
	body := map[string]any{"filesetName": fileset}
	if o.has("-t") {
		body["comment"] = o.value("-t")
	}
	if o.has("-J") {
		body["path"] = o.value("-J")
	}
	if o.has("--inode-space") {
		body["inodeSpace"] = o.value("--inode-space")
	}
	if o.has("--inode-limit") {
		// mmcrfileset takes MaxNumInodes[:NumInodesToPreallocate]; the GUI takes
		// the two apart.
		maxInodes, prealloc, _ := strings.Cut(o.value("--inode-limit"), ":")
		limit, err := parseInodeCount(maxInodes)
		if err != nil {
			return err
		}
		body["maxNumInodes"] = limit
		if prealloc != "" {
			allocated, err := parseInodeCount(prealloc)
			if err != nil {
				return err
			}
			body["inodeLimit"] = allocated
		}
	}
	if o.has("--allow-permission-change") {
		body["permissionChangeMode"] = o.value("--allow-permission-change")
	}

	return scaleWrite("POST", "filesystems/"+device+"/filesets", nil, body)
}

// parseInodeCount accepts the inode counts mm commands accept, plain or with a
// K/M/G suffix (--inode-limit 1M).
func parseInodeCount(s string) (int64, error) {
	units := map[byte]int64{'K': 1024, 'M': 1024 * 1024, 'G': 1024 * 1024 * 1024}
	value := strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	if len(value) > 0 {
		if u, ok := units[value[len(value)-1]]; ok {
			mult = u
			value = value[:len(value)-1]
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, usagef("Invalid inode limit: %s", s)
	}
	return n * mult, nil
}

func cmdMmchfileset() *command {
	return &command{
		name:    "mmchfileset",
		summary: "Change fileset attributes",
		usage: `Usage:
  mmchfileset Device {FilesetName | -J JunctionPath} [-j NewFilesetName]
     [-t NewComment] [--inode-limit MaxNumInodes]
     [--allow-permission-change PermissionChangeMode]`,
		spec: optSpec{
			"-j":                        valueOpt,
			"-t":                        valueOpt,
			"-J":                        valueOpt,
			"--inode-limit":             valueOpt,
			"--allow-permission-change": valueOpt,
		},
		run: runMmchfileset,
	}
}

func runMmchfileset(o options) error {
	device, fileset, err := deviceAndFileset(o)
	if err != nil {
		return err
	}

	body := map[string]any{}
	if o.has("-j") {
		body["newFilesetName"] = o.value("-j")
	}
	if o.has("-t") {
		body["comment"] = o.value("-t")
	}
	if o.has("--inode-limit") {
		maxInodes, _, _ := strings.Cut(o.value("--inode-limit"), ":")
		limit, err := parseInodeCount(maxInodes)
		if err != nil {
			return err
		}
		body["maxNumInodes"] = limit
	}
	if o.has("--allow-permission-change") {
		body["permissionChangeMode"] = o.value("--allow-permission-change")
	}
	if len(body) == 0 {
		return usagef("Missing arguments.")
	}

	return scaleWrite("PUT", "filesystems/"+device+"/filesets/"+fileset, nil, body)
}

func cmdMmdelfileset() *command {
	return &command{
		name:    "mmdelfileset",
		summary: "Delete a fileset",
		usage: `Usage:
  mmdelfileset Device FilesetName [-f]`,
		spec: optSpec{"-f": flagOpt},
		run:  runMmdelfileset,
	}
}

func runMmdelfileset(o options) error {
	device, fileset, err := deviceAndFileset(o)
	if err != nil {
		return err
	}
	return scaleWrite("DELETE", "filesystems/"+device+"/filesets/"+fileset, forceQuery(o), nil)
}

func cmdMmlinkfileset() *command {
	return &command{
		name:    "mmlinkfileset",
		summary: "Link a fileset into the namespace",
		usage: `Usage:
  mmlinkfileset Device FilesetName [-J JunctionPath]`,
		spec: optSpec{"-J": valueOpt},
		run:  runMmlinkfileset,
	}
}

func runMmlinkfileset(o options) error {
	device, fileset, err := deviceAndFileset(o)
	if err != nil {
		return err
	}

	junction := o.value("-J")
	if junction == "" {
		// mmlinkfileset defaults the junction to a directory of the fileset's
		// name below the file system's mount point.
		fs, err := getFilesystem(device)
		if err != nil {
			return err
		}
		if fs.Mount.MountPoint == "" {
			return fmt.Errorf("File system %s has no mount point; specify -J JunctionPath.", device)
		}
		junction = strings.TrimSuffix(fs.Mount.MountPoint, "/") + "/" + fileset
	}

	return scaleWrite("POST", "filesystems/"+device+"/filesets/"+fileset+"/link", nil,
		map[string]any{"path": junction})
}

func cmdMmunlinkfileset() *command {
	return &command{
		name:    "mmunlinkfileset",
		summary: "Unlink a fileset from the namespace",
		usage: `Usage:
  mmunlinkfileset Device {FilesetName | -J JunctionPath} [-f]`,
		spec: optSpec{"-f": flagOpt, "-J": valueOpt},
		run:  runMmunlinkfileset,
	}
}

func runMmunlinkfileset(o options) error {
	device, fileset, err := deviceAndFileset(o)
	if err != nil {
		return err
	}
	return scaleWrite("DELETE", "filesystems/"+device+"/filesets/"+fileset+"/link", forceQuery(o), nil)
}

// deviceAndFileset reads the "Device FilesetName" operand pair shared by the
// fileset commands. -J names the fileset by its junction path instead, which
// mmchfileset and mmunlinkfileset accept.
func deviceAndFileset(o options) (string, string, error) {
	if len(o.operands) == 0 {
		return "", "", usagef("Missing arguments.")
	}
	device := o.operands[0]

	if len(o.operands) > 1 {
		return device, o.operands[1], nil
	}

	junction := o.value("-J")
	if junction == "" {
		return "", "", usagef("Missing arguments.")
	}
	filesets, err := listFilesets(device)
	if err != nil {
		return "", "", err
	}
	for i := range filesets {
		if filesets[i].Config.Path == strings.TrimSuffix(junction, "/") {
			return device, filesets[i].FilesetName, nil
		}
	}
	return "", "", fmt.Errorf("Junction path %s not found.", junction)
}

func forceQuery(o options) url.Values {
	if !o.has("-f") {
		return nil
	}
	return url.Values{"force": []string{"true"}}
}
