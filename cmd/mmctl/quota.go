package main

import (
	"fmt"
	"net/url"
	"os/user"
	"sort"
	"strconv"
	"strings"
)

// quotaTarget is a "Device" or "Device:Fileset" operand, the form every mm
// quota command uses.
//
// The distinction is not cosmetic: when a file system has --perfileset-quota
// enabled (the default for file systems serving CSI) the GUI rejects user and
// group quota writes at file system scope with "Per fileset quota enabled on
// this filesystem", and such quotas are only readable per fileset.
type quotaTarget struct {
	fs      string
	fileset string
}

func parseQuotaTarget(s string) (quotaTarget, error) {
	fs, fileset, hasFileset := strings.Cut(s, ":")
	if fs == "" || (hasFileset && fileset == "") {
		return quotaTarget{}, usagef("Invalid device: %s", s)
	}
	return quotaTarget{fs: fs, fileset: fileset}, nil
}

// path returns the scalemgmt quota collection for the target.
func (t quotaTarget) path() string {
	if t.fileset != "" {
		return "filesystems/" + t.fs + "/filesets/" + t.fileset + "/quotas"
	}
	return "filesystems/" + t.fs + "/quotas"
}

func (t quotaTarget) String() string {
	if t.fileset != "" {
		return t.fs + ":" + t.fileset
	}
	return t.fs
}

type quotaEntry struct {
	QuotaType      string `json:"quotaType"`
	ObjectName     string `json:"objectName"`
	ObjectID       int64  `json:"objectId"`
	FilesetName    string `json:"filesetName"`
	FilesystemName string `json:"filesystemName"`
	BlockUsage     int64  `json:"blockUsage"`
	BlockQuota     int64  `json:"blockQuota"`
	BlockLimit     int64  `json:"blockLimit"`
	BlockInDoubt   int64  `json:"blockInDoubt"`
	BlockGrace     string `json:"blockGrace"`
	FilesUsage     int64  `json:"filesUsage"`
	FilesQuota     int64  `json:"filesQuota"`
	FilesLimit     int64  `json:"filesLimit"`
	FilesInDoubt   int64  `json:"filesInDoubt"`
	FilesGrace     string `json:"filesGrace"`
	IsDefaultQuota bool   `json:"isDefaultQuota"`
}

// fileset names the fileset a quota belongs to. File system scoped quotas
// belong to the root fileset, which is how mm quota commands report them.
func (q quotaEntry) fileset() string {
	if q.FilesetName == "" {
		return "root"
	}
	return q.FilesetName
}

// matches reports whether the entry is the user, group or fileset named on the
// command line, which may be given as a name or as a numeric id.
func (q quotaEntry) matches(object string) bool {
	return q.ObjectName == object || strconv.FormatInt(q.ObjectID, 10) == object
}

func listQuotas(target quotaTarget, quotaType string) ([]quotaEntry, error) {
	scopes, err := quotaScopes(target, quotaType)
	if err != nil {
		return nil, err
	}

	var entries []quotaEntry
	for _, scope := range scopes {
		query := url.Values{}
		if quotaType != "" {
			query.Set("filter", "quotaType="+quotaType)
		}
		var resp struct {
			Quotas []quotaEntry `json:"quotas"`
		}
		if err := scaleGetInto(scope.path(), query, &resp); err != nil {
			return nil, err
		}
		entries = append(entries, resp.Quotas...)
	}
	return entries, nil
}

// quotaScopes expands a target into the quota collections that hold the
// requested type. On a file system with per-fileset quotas enabled, user and
// group quotas exist only inside filesets, so a bare Device fans out over its
// filesets — which is the report mmlsquota and mmrepquota give there. A
// fileset's own quota always lives in the file system's collection.
func quotaScopes(target quotaTarget, quotaType string) ([]quotaTarget, error) {
	if target.fileset != "" || quotaType == "FILESET" {
		return []quotaTarget{target}, nil
	}

	fs, err := getFilesystem(target.fs)
	if err != nil {
		return nil, err
	}
	if !fs.Quota.PerfilesetQuotas {
		return []quotaTarget{target}, nil
	}

	filesets, err := listFilesets(target.fs)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(filesets, func(i, j int) bool {
		return filesets[i].Config.ID < filesets[j].Config.ID
	})

	scopes := make([]quotaTarget, 0, len(filesets))
	for i := range filesets {
		scopes = append(scopes, quotaTarget{fs: target.fs, fileset: filesets[i].FilesetName})
	}
	return scopes, nil
}

// quotaLine renders one row of a quota report, and — given the header strings
// instead of values — its column header too. The field widths are those of
// mmlsquota and mmrepquota, so the output lines up with what the cluster
// prints even when a fileset name overruns its column.
func quotaLine(filesetColumn bool, name, fileset, quotaType string, block, files [5]string, remarks bool, remarksText string) string {
	l := &line{}
	l.cell(11, name)
	if filesetColumn {
		l.cell(11, fileset)
	}
	l.cell(9, quotaType)
	l.rcell(10, block[0])
	l.rcell(11, block[1])
	l.rcell(11, block[2])
	l.rcell(11, block[3])
	l.rcell(9, block[4])
	l.rcell(2, "|")
	l.rcell(9, files[0])
	l.rcell(8, files[1])
	l.rcell(9, files[2])
	l.rcell(9, files[3])
	l.rcell(9, files[4])

	out := l.String()
	switch {
	case remarks && remarksText != "":
		out += "  " + remarksText
	case remarks:
		// The GUI does not report the enforcement remarks mmlsquota shows, so
		// the column stays empty — but the separating space is still there.
		out += " "
	}
	return out
}

// quotaHeader renders the two header lines. filesetHeader is empty when the
// report has no fileset column; mmlsquota and mmrepquota spell that column
// differently ("Fileset" against "fileset"), so the caller passes its own.
func quotaHeader(blocks blockFormatter, nameHeader, filesetHeader string, remarks bool) string {
	head1 := "                         Block Limits                                    |     File Limits"
	if filesetHeader != "" {
		head1 = "                         Block Limits                                               |     File Limits"
	}
	if !remarks {
		// mmrepquota keeps the block caption where mmlsquota's narrow layout has
		// it and pushes the file caption right.
		head1 = "                         Block Limits                                    |                     File Limits"
	}
	head2 := quotaLine(filesetHeader != "", nameHeader, filesetHeader, "type",
		[5]string{blocks.header, "quota", "limit", "in_doubt", "grace"},
		[5]string{"files", "quota", "limit", "in_doubt", "grace"},
		remarks, "Remarks")
	return head1 + "\n" + head2
}

func quotaRow(q quotaEntry, blocks blockFormatter, filesetColumn bool, name string, remarks bool) string {
	return quotaLine(filesetColumn, name, q.fileset(), q.QuotaType,
		[5]string{blocks.format(q.BlockUsage), blocks.format(q.BlockQuota),
			blocks.format(q.BlockLimit), blocks.format(q.BlockInDoubt), q.BlockGrace},
		[5]string{num(q.FilesUsage), num(q.FilesQuota), num(q.FilesLimit),
			num(q.FilesInDoubt), q.FilesGrace},
		remarks, "")
}

// quotaYRecord is the -Y layout of the quota commands. mmrepquota carries two
// extra fields for the file system's quota configuration.
func quotaYRecord(cmd, section string) *yRecord {
	fields := []string{
		"filesystemName", "quotaType", "id", "name", "blockUsage", "blockQuota",
		"blockLimit", "blockInDoubt", "blockGrace", "filesUsage", "filesQuota",
		"filesLimit", "filesInDoubt", "filesGrace", "remarks",
	}
	if cmd == "mmrepquota" {
		fields = append(fields, "quota", "defQuota")
	}
	return newYRecord(cmd, section, append(fields, "fid", "filesetname")...)
}

// quotaState is the per-file-system quota configuration mmrepquota reports
// alongside each entry: whether quotas of that type are enforced, and whether
// default quotas are enabled.
type quotaState struct {
	enforced       string // e.g. "user;group;fileset"
	defaultEnabled string // e.g. "none" or "user;group"
}

func (s quotaState) enforcedFor(quotaType string) string {
	return onOff(strings.Contains(s.enforced, quotaTypeWord(quotaType)))
}

func (s quotaState) defaultFor(quotaType string) string {
	return onOff(strings.Contains(s.defaultEnabled, quotaTypeWord(quotaType)))
}

func quotaTypeWord(quotaType string) string {
	switch quotaType {
	case "USR":
		return "user"
	case "GRP":
		return "group"
	default:
		return "fileset"
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// quotaStatesFor reads the quota configuration of the file systems in targets.
func quotaStatesFor(targets []quotaTarget) map[string]quotaState {
	states := map[string]quotaState{}
	for _, target := range targets {
		if _, ok := states[target.fs]; ok {
			continue
		}
		fs, err := getFilesystem(target.fs)
		if err != nil {
			continue
		}
		states[target.fs] = quotaState{
			enforced:       fs.Quota.QuotasEnforced,
			defaultEnabled: fs.Quota.DefaultQuotasEnabled,
		}
	}
	return states
}

// printQuotaYRow writes one -Y record. filesetIDs maps fileset names to their
// ids for the fid field; fileset quotas carry neither, as mm commands report
// them.
func printQuotaYRow(y *yRecord, q quotaEntry, filesetIDs map[string]int64, state quotaState) {
	values := []string{
		q.FilesystemName, q.QuotaType, num(q.ObjectID), q.ObjectName,
		num(q.BlockUsage), num(q.BlockQuota), num(q.BlockLimit), num(q.BlockInDoubt),
		q.BlockGrace, num(q.FilesUsage), num(q.FilesQuota), num(q.FilesLimit),
		num(q.FilesInDoubt), q.FilesGrace, "",
	}
	if y.cmd == "mmrepquota" {
		values = append(values, state.enforcedFor(q.QuotaType), state.defaultFor(q.QuotaType))
	}

	fid, filesetName := "", ""
	if q.QuotaType != "FILESET" {
		filesetName = q.fileset()
		if id, ok := filesetIDs[filesetName]; ok {
			fid = num(id)
		}
	}
	y.printRow(append(values, fid, filesetName)...)
}

// filesetIDsFor collects the fileset name to id mapping of the file systems in
// the given targets, which -Y output reports alongside each quota.
func filesetIDsFor(targets []quotaTarget) map[string]int64 {
	ids := map[string]int64{}
	seen := map[string]bool{}
	for _, target := range targets {
		if seen[target.fs] {
			continue
		}
		seen[target.fs] = true
		filesets, err := listFilesets(target.fs)
		if err != nil {
			// The ids are decoration; a failure here must not cost the report.
			continue
		}
		for i := range filesets {
			ids[filesets[i].FilesetName] = filesets[i].Config.ID
		}
	}
	return ids
}

func cmdMmlsquota() *command {
	return &command{
		name:    "mmlsquota",
		summary: "Display quota limits of a user, group or fileset",
		usage: `Usage:
  mmlsquota [-u User | -g Group] [-e] [-Y] [--block-size {BlockSize | auto}]
            [Device[:Fileset] ...]
    or
  mmlsquota -j Fileset [-e] [-Y] [--block-size {BlockSize | auto}] Device ...`,
		spec: optSpec{
			"-u":           valueOpt,
			"-g":           valueOpt,
			"-j":           valueOpt,
			"-e":           flagOpt,
			"-Y":           flagOpt,
			"--block-size": valueOpt,
		},
		run: runMmlsquota,
	}
}

func runMmlsquota(o options) error {
	if o.has("-u") && o.has("-g") {
		return usagef("Options -u and -g are mutually exclusive.")
	}
	if o.has("-j") && (o.has("-u") || o.has("-g")) {
		return usagef("Option -j cannot be combined with -u or -g.")
	}

	blocks, err := newBlockFormatter(o.value("--block-size"))
	if err != nil {
		return err
	}

	quotaType, object, section := "USR", o.value("-u"), "user"
	switch {
	case o.has("-j"):
		quotaType, object, section = "FILESET", o.value("-j"), "fileset"
	case o.has("-g"):
		quotaType, object, section = "GRP", o.value("-g"), "group"
	case !o.has("-u"):
		// Without -u or -g, mmlsquota reports the invoking user's quotas.
		current, err := user.Current()
		if err != nil {
			return fmt.Errorf("cannot determine the current user; use -u User")
		}
		object = current.Username
	}

	targets, err := quotaTargets(o.operands)
	if err != nil {
		return err
	}

	var entries []quotaEntry
	for _, target := range targets {
		if quotaType == "FILESET" && target.fileset != "" {
			return usagef("Option -j takes a Device without a fileset: %s", target)
		}
		found, err := listQuotas(target, quotaType)
		if err != nil {
			return err
		}
		for _, q := range found {
			if q.matches(object) {
				entries = append(entries, q)
			}
		}
	}
	sortQuotas(entries)

	if o.has("-Y") {
		y := quotaYRecord("mmlsquota", section)
		y.printHeader()
		filesetIDs := filesetIDsFor(targets)
		for _, q := range entries {
			printQuotaYRow(y, q, filesetIDs, quotaState{})
		}
		return nil
	}

	// User and group quotas are reported per fileset; a fileset's own quota is
	// not, which is how mmlsquota lays the two out.
	filesetHeader := "Fileset"
	if quotaType == "FILESET" {
		filesetHeader = ""
	}

	if o.has("-g") {
		// mmlsquota heads a group report with the group it resolved.
		fmt.Printf("\nDisk quotas for group %s (gid %s):\n", object, groupID(object, entries))
	}
	fmt.Println(quotaHeader(blocks, "Filesystem", filesetHeader, true))
	for _, q := range entries {
		fmt.Println(quotaRow(q, blocks, filesetHeader != "", q.FilesystemName, true))
	}
	return nil
}

// groupID reports the numeric id of the group being listed: the one the
// cluster attached to its quotas, the operand itself when it is numeric, or
// the local resolution as a last resort.
func groupID(object string, entries []quotaEntry) string {
	if len(entries) > 0 {
		return num(entries[0].ObjectID)
	}
	if _, err := strconv.ParseInt(object, 10, 64); err == nil {
		return object
	}
	if group, err := user.LookupGroup(object); err == nil {
		return group.Gid
	}
	return "-"
}

func cmdMmrepquota() *command {
	return &command{
		name:    "mmrepquota",
		summary: "Report quotas of every user, group or fileset",
		usage: `Usage:
  mmrepquota [-u] [-g] [-e] [-n] [-Y] [--block-size {BlockSize | auto}]
             {-a | Device[:Fileset] ...}
    or
  mmrepquota -j [-e] [-n] [-Y] [--block-size {BlockSize | auto}]
             {-a | Device ...}`,
		spec: optSpec{
			"-u":           flagOpt,
			"-g":           flagOpt,
			"-j":           flagOpt,
			"-a":           flagOpt,
			"-e":           flagOpt,
			"-n":           flagOpt,
			"-Y":           flagOpt,
			"--block-size": valueOpt,
		},
		run: runMmrepquota,
	}
}

func runMmrepquota(o options) error {
	if o.has("-j") && (o.has("-u") || o.has("-g")) {
		return usagef("Option -j cannot be combined with -u or -g.")
	}
	if o.has("-a") && len(o.operands) > 0 {
		return usagef("Option -a cannot be combined with a device.")
	}
	if !o.has("-a") && len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}

	blocks, err := newBlockFormatter(o.value("--block-size"))
	if err != nil {
		return err
	}

	// mmrepquota reports user and group quotas unless told otherwise.
	var quotaTypes []string
	switch {
	case o.has("-j"):
		quotaTypes = []string{"FILESET"}
	default:
		if o.has("-u") || !o.has("-g") {
			quotaTypes = append(quotaTypes, "USR")
		}
		if o.has("-g") || !o.has("-u") {
			quotaTypes = append(quotaTypes, "GRP")
		}
	}

	targets, err := quotaTargets(o.operands)
	if err != nil {
		return err
	}

	var entries []quotaEntry
	for _, target := range targets {
		for _, quotaType := range quotaTypes {
			found, err := listQuotas(target, quotaType)
			if err != nil {
				return err
			}
			sortQuotas(found)
			entries = append(entries, found...)
		}
	}

	filesetIDs := filesetIDsFor(targets)
	// -n reports numeric ids in place of names, in both the object and the
	// fileset column.
	name := func(q quotaEntry) string {
		if o.has("-n") {
			return num(q.ObjectID)
		}
		return q.ObjectName
	}
	fileset := func(q quotaEntry) string {
		if o.has("-n") {
			if id, ok := filesetIDs[q.fileset()]; ok {
				return num(id)
			}
		}
		return q.fileset()
	}

	if o.has("-Y") {
		y := quotaYRecord("mmrepquota", "")
		y.printHeader()
		states := quotaStatesFor(targets)
		for _, q := range entries {
			printQuotaYRow(y, q, filesetIDs, states[q.FilesystemName])
		}
		return nil
	}

	fmt.Println(quotaHeader(blocks, "Name", "fileset", false))
	for _, q := range entries {
		fmt.Println(quotaLine(true, name(q), fileset(q), q.QuotaType,
			[5]string{blocks.format(q.BlockUsage), blocks.format(q.BlockQuota),
				blocks.format(q.BlockLimit), blocks.format(q.BlockInDoubt), q.BlockGrace},
			[5]string{num(q.FilesUsage), num(q.FilesQuota), num(q.FilesLimit),
				num(q.FilesInDoubt), q.FilesGrace},
			false, ""))
	}
	return nil
}

// sortQuotas orders entries by object id, the order mm quota commands report.
// The sort is stable, so entries collected fileset by fileset keep that order
// within one object.
func sortQuotas(entries []quotaEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].ObjectID < entries[j].ObjectID
	})
}

// quotaTargets parses the Device[:Fileset] operands, defaulting to every file
// system the token can reach when none are given.
func quotaTargets(operands []string) ([]quotaTarget, error) {
	if len(operands) > 0 {
		targets := make([]quotaTarget, 0, len(operands))
		for _, operand := range operands {
			target, err := parseQuotaTarget(operand)
			if err != nil {
				return nil, err
			}
			targets = append(targets, target)
		}
		return targets, nil
	}

	filesystems, err := listFilesystems()
	if err != nil {
		return nil, err
	}
	targets := make([]quotaTarget, 0, len(filesystems))
	for _, fs := range filesystems {
		targets = append(targets, quotaTarget{fs: fs.Name})
	}
	return targets, nil
}

func cmdMmsetquota() *command {
	return &command{
		name:    "mmsetquota",
		summary: "Set quota limits, default limits or grace periods",
		usage: `Usage:
  mmsetquota Device{[:FilesetName]
              [--user IdOrName[,IdOrName]] [--group IdOrName[,IdOrName]]}
             {[--block SoftLimit[:HardLimit]] [--files SoftLimit[:HardLimit]]}
    or
  mmsetquota Device[:FilesetName] --default {user | group}
             {[--block SoftLimit[:HardLimit]] [--files SoftLimit[:HardLimit]]}
    or
  mmsetquota Device --default fileset
             {[--block SoftLimit[:HardLimit]] [--files SoftLimit[:HardLimit]]}
    or
  mmsetquota Device[:FilesetName] --grace {user | group | fileset}
             {[--block GracePeriod] [--files GracePeriod]}

Limits use GPFS syntax (10G, 512M, 1T); 0 means no limit. Omitting HardLimit
leaves the hard limit unset. GPFS has no delete operation for a quota: zero
every limit to remove one.`,
		spec: optSpec{
			"--user":    valueOpt,
			"--group":   valueOpt,
			"--block":   valueOpt,
			"--files":   valueOpt,
			"--default": valueOpt,
			"--grace":   valueOpt,
		},
		run: runMmsetquota,
	}
}

func runMmsetquota(o options) error {
	if len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}
	if len(o.operands) > 1 {
		return usagef("Incorrect operand: %s", o.operands[1])
	}
	target, err := parseQuotaTarget(o.operands[0])
	if err != nil {
		return err
	}
	if !o.has("--block") && !o.has("--files") {
		return usagef("Missing arguments.")
	}

	switch {
	case o.has("--grace"):
		return setGracePeriod(o, target)
	case o.has("--default"):
		return setDefaultQuota(o, target)
	}

	if o.has("--user") && o.has("--group") {
		return usagef("Options --user and --group are mutually exclusive.")
	}

	blockSoft, blockHard := limitPair(o.value("--block"))
	filesSoft, filesHard := limitPair(o.value("--files"))

	setOne := func(quotaType, objectName string, path string) error {
		body := map[string]string{
			"operationType": "setQuota",
			"quotaType":     quotaType,
			"objectName":    objectName,
		}
		if o.has("--block") {
			body["blockSoftLimit"] = blockSoft
			body["blockHardLimit"] = blockHard
		}
		if o.has("--files") {
			body["filesSoftLimit"] = filesSoft
			body["filesHardLimit"] = filesHard
		}
		return scaleWrite("POST", path, nil, body)
	}

	switch {
	case o.has("--user"), o.has("--group"):
		quotaType, ids := "USR", splitList(o.value("--user"))
		if o.has("--group") {
			quotaType, ids = "GRP", splitList(o.value("--group"))
		}
		if len(ids) == 0 {
			return usagef("Missing arguments.")
		}
		for _, id := range ids {
			if err := setOne(quotaType, id, target.path()); err != nil {
				return err
			}
		}
		return nil

	case target.fileset != "":
		// Device:Fileset without --user or --group addresses the fileset's own
		// quota, which lives in the file system's quota collection.
		return setOne("FILESET", target.fileset, quotaTarget{fs: target.fs}.path())

	default:
		return usagef("Specify --user, --group, or a Device:Fileset to set a fileset quota.")
	}
}

func setDefaultQuota(o options, target quotaTarget) error {
	quotaType, err := quotaTypeName(o.value("--default"))
	if err != nil {
		return err
	}
	if quotaType == "FILESET" && target.fileset != "" {
		return usagef("--default fileset takes a Device without a fileset: %s", target)
	}

	blockSoft, blockHard := limitPair(o.value("--block"))
	filesSoft, filesHard := limitPair(o.value("--files"))
	body := map[string]string{
		"operationType": "setDefaultQuota",
		"quotaType":     quotaType,
	}
	if o.has("--block") {
		body["blockSoftLimit"] = blockSoft
		body["blockHardLimit"] = blockHard
	}
	if o.has("--files") {
		body["filesSoftLimit"] = filesSoft
		body["filesHardLimit"] = filesHard
	}
	return scaleWrite("POST", target.path(), nil, body)
}

func setGracePeriod(o options, target quotaTarget) error {
	quotaType, err := quotaTypeName(o.value("--grace"))
	if err != nil {
		return err
	}

	body := map[string]string{
		"operationType": "setGracePeriod",
		"quotaType":     quotaType,
	}
	if o.has("--block") {
		body["blockGracePeriod"] = o.value("--block")
	}
	if o.has("--files") {
		body["filesGracePeriod"] = o.value("--files")
	}
	return scaleWrite("POST", target.path(), nil, body)
}

func quotaTypeName(s string) (string, error) {
	switch strings.ToLower(s) {
	case "user":
		return "USR", nil
	case "group":
		return "GRP", nil
	case "fileset":
		return "FILESET", nil
	}
	return "", usagef("Incorrect quota type: %s", s)
}

// limitPair splits a "SoftLimit[:HardLimit]" option value. mm commands leave
// the hard limit unset when it is omitted, which GPFS spells as 0.
func limitPair(s string) (string, string) {
	soft, hard, _ := strings.Cut(s, ":")
	if hard == "" {
		hard = "0"
	}
	if soft == "" {
		soft = "0"
	}
	return soft, hard
}
