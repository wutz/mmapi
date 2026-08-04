package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	apiURL     string
	apiToken   string
	adminToken string
)

func init() {
	apiURL = os.Getenv("MMAPI_URL")
	apiToken = os.Getenv("MMAPI_TOKEN")
	adminToken = os.Getenv("MMAPI_ADMIN_TOKEN")
	if apiURL == "" {
		apiURL = "https://localhost:8443"
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "token":
		handleToken(args)
	case "fs", "filesystem":
		handleFilesystem(args)
	case "fileset":
		handleFileset(args)
	case "quota":
		handleQuota(args)
	case "cluster":
		handleCluster()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`mmctl - CLI for mmapi (GPFS multi-tenant API proxy)

Usage: mmctl <command> [subcommand] [args]

Commands:
  cluster                    Show cluster info
  fs list                    List filesystems
  fs get <name>              Get filesystem details
  fileset list <fs>          List filesets in filesystem
  fileset get <fs> <name>    Get fileset details
  fileset create <fs> <name> Create a fileset
  fileset delete <fs> <name> Delete a fileset
  fileset link <fs> <name> <path>   Link fileset
  fileset unlink <fs> <name>        Unlink fileset
  quota list <fs>[:<fileset>] [type]      List quotas (type: USR|GRP|FILESET)
  quota set <fs> <fileset> <blockSoft> <blockHard> [<filesSoft> <filesHard>]
                             Set a fileset quota
  quota user list <fs>[:<fileset>]        List user quotas
  quota user set <fs>[:<fileset>] <user> <blockSoft> <blockHard> [<filesSoft> <filesHard>]
                             Set a user quota
  quota user unset <fs>[:<fileset>] <user>    Remove a user quota
  quota group list <fs>[:<fileset>]       List group quotas
  quota group set <fs>[:<fileset>] <group> <blockSoft> <blockHard> [<filesSoft> <filesHard>]
                             Set a group quota
  quota group unset <fs>[:<fileset>] <group>  Remove a group quota
  token create <fs1,fs2,...>  Create access token
  token list                 List tokens
  token delete <id>          Delete token

Quota targets follow the mmsetquota Device[:Fileset] convention. Filesystems
with per-fileset quota enabled (mmlsfs --perfileset-quota) only accept user and
group quotas at fileset scope, i.e. <fs>:<fileset>.

Limits use GPFS syntax: 10G, 512M, 1T. Use 0 for unlimited.

Environment:
  MMAPI_URL         mmapi server URL (default: https://localhost:8443)
  MMAPI_TOKEN       mmapi access token (for /scalemgmt/ API)
  MMAPI_ADMIN_TOKEN mmapi admin token (for /api/v1/tokens management)`)
}

// HTTP client

func httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func doRequest(method, path string, body string) ([]byte, int, error) {
	url := apiURL + path
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	if apiToken != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+apiToken)))
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func doAdminRequest(method, path string, body string) ([]byte, int, error) {
	url := apiURL + path
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func doScaleGet(path string) ([]byte, error) {
	data, code, err := doRequest("GET", "/scalemgmt/v2/"+path, "")
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", code, string(data))
	}
	return data, nil
}

func doScalePost(path, body string) ([]byte, error) {
	data, code, err := doRequest("POST", "/scalemgmt/v2/"+path, body)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", code, string(data))
	}
	return data, nil
}

func doScaleDelete(path string) error {
	_, code, err := doRequest("DELETE", "/scalemgmt/v2/"+path, "")
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("HTTP %d", code)
	}
	return nil
}

// Command handlers

func handleCluster() {
	data, err := doScaleGet("cluster")
	if err != nil {
		fatal(err)
	}
	prettyPrint(data)
}

func handleFilesystem(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list":
		data, err := doScaleGet("filesystems")
		if err != nil {
			fatal(err)
		}
		var resp struct {
			Filesystems []struct {
				Name  string `json:"name"`
				Mount struct {
					MountPoint string `json:"mountPoint"`
					Status     string `json:"status"`
				} `json:"mount"`
			} `json:"filesystems"`
		}
		json.Unmarshal(data, &resp)

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tMOUNT\tSTATUS")
		for _, fs := range resp.Filesystems {
			fmt.Fprintf(w, "%s\t%s\t%s\n", fs.Name, fs.Mount.MountPoint, fs.Mount.Status)
		}
		w.Flush()

	case "get":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: mmctl fs get <name>"))
		}
		data, err := doScaleGet("filesystems/" + args[1])
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	default:
		fatal(fmt.Errorf("unknown filesystem command: %s", args[0]))
	}
}

func handleFileset(args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("usage: mmctl fileset <list|get|create|delete|link|unlink> <fs> [args]"))
	}

	subcmd := args[0]
	fs := args[1]

	switch subcmd {
	case "list":
		data, err := doScaleGet("filesystems/" + fs + "/filesets")
		if err != nil {
			fatal(err)
		}
		var resp struct {
			Filesets []struct {
				FilesetName string `json:"filesetName"`
				Config      struct {
					Path   string `json:"path"`
					Status string `json:"status"`
				} `json:"config"`
			} `json:"filesets"`
		}
		json.Unmarshal(data, &resp)

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPATH\tSTATUS")
		for _, f := range resp.Filesets {
			fmt.Fprintf(w, "%s\t%s\t%s\n", f.FilesetName, f.Config.Path, f.Config.Status)
		}
		w.Flush()

	case "get":
		if len(args) < 3 {
			fatal(fmt.Errorf("usage: mmctl fileset get <fs> <name>"))
		}
		data, err := doScaleGet("filesystems/" + fs + "/filesets/" + args[2])
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	case "create":
		if len(args) < 3 {
			fatal(fmt.Errorf("usage: mmctl fileset create <fs> <name>"))
		}
		body := fmt.Sprintf(`{"filesetName":"%s","inodeSpace":"new"}`, args[2])
		data, err := doScalePost("filesystems/"+fs+"/filesets", body)
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	case "delete":
		if len(args) < 3 {
			fatal(fmt.Errorf("usage: mmctl fileset delete <fs> <name>"))
		}
		if err := doScaleDelete("filesystems/" + fs + "/filesets/" + args[2]); err != nil {
			fatal(err)
		}
		fmt.Println("Fileset deleted.")

	case "link":
		if len(args) < 4 {
			fatal(fmt.Errorf("usage: mmctl fileset link <fs> <name> <path>"))
		}
		body := fmt.Sprintf(`{"path":"%s"}`, args[3])
		data, err := doScalePost("filesystems/"+fs+"/filesets/"+args[2]+"/link", body)
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	case "unlink":
		if len(args) < 3 {
			fatal(fmt.Errorf("usage: mmctl fileset unlink <fs> <name>"))
		}
		if err := doScaleDelete("filesystems/" + fs + "/filesets/" + args[2] + "/link"); err != nil {
			fatal(err)
		}
		fmt.Println("Fileset unlinked.")

	default:
		fatal(fmt.Errorf("unknown fileset command: %s", subcmd))
	}
}

// quotaTarget is a "<fs>" or "<fs>:<fileset>" operand. It mirrors the
// Device[:Fileset] operand of mmsetquota/mmlsquota so GPFS admins can reuse
// what they already know.
//
// The distinction is not cosmetic: when a filesystem has --perfileset-quota
// enabled (the default for CSI filesystems) the GUI rejects user and group
// quota writes at filesystem scope with "Per fileset quota enabled on this
// filesystem", and such quotas are only readable per fileset. Filesystems
// without per-fileset quota take the bare "<fs>" form instead.
type quotaTarget struct {
	fs      string
	fileset string
}

func parseQuotaTarget(s string) (quotaTarget, error) {
	fs, fileset, hasFileset := strings.Cut(s, ":")
	if fs == "" || (hasFileset && fileset == "") {
		return quotaTarget{}, fmt.Errorf("invalid quota target %q: expected <fs> or <fs>:<fileset>", s)
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

type quotaEntry struct {
	QuotaType   string `json:"quotaType"`
	ObjectName  string `json:"objectName"`
	FilesetName string `json:"filesetName"`
	BlockUsage  int64  `json:"blockUsage"`
	BlockQuota  int64  `json:"blockQuota"`
	BlockLimit  int64  `json:"blockLimit"`
	BlockGrace  string `json:"blockGrace"`
	FilesUsage  int64  `json:"filesUsage"`
	FilesQuota  int64  `json:"filesQuota"`
	FilesLimit  int64  `json:"filesLimit"`
}

func handleQuota(args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("usage: mmctl quota <list|set|user|group> [args]"))
	}

	switch args[0] {
	case "list":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: mmctl quota list <fs>[:<fileset>] [USR|GRP|FILESET]"))
		}
		target, err := parseQuotaTarget(args[1])
		if err != nil {
			fatal(err)
		}
		var quotaType string
		if len(args) >= 3 {
			quotaType = strings.ToUpper(args[2])
		}
		quotaList(target, quotaType)

	case "set":
		// Fileset quotas are addressed by filesystem plus the fileset as the
		// quota object, so this form keeps its own positional layout.
		if len(args) < 5 {
			fatal(fmt.Errorf("usage: mmctl quota set <fs> <fileset> <blockSoft> <blockHard> [<filesSoft> <filesHard>]"))
		}
		target, err := parseQuotaTarget(args[1])
		if err != nil {
			fatal(err)
		}
		quotaSet(target, "FILESET", args[2], args[3:])

	case "user":
		handleObjectQuota("user", "USR", args[1:])

	case "group":
		handleObjectQuota("group", "GRP", args[1:])

	default:
		fatal(fmt.Errorf("unknown quota command: %s", args[0]))
	}
}

// handleObjectQuota implements the user and group quota subcommands, which are
// identical apart from the quota type the GUI expects.
func handleObjectQuota(name, quotaType string, args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("usage: mmctl quota %s <list|set|unset> <fs>[:<fileset>] [args]", name))
	}

	target, err := parseQuotaTarget(args[1])
	if err != nil {
		fatal(err)
	}

	switch args[0] {
	case "list":
		quotaList(target, quotaType)

	case "set":
		if len(args) < 5 {
			fatal(fmt.Errorf("usage: mmctl quota %s set <fs>[:<fileset>] <%s> <blockSoft> <blockHard> [<filesSoft> <filesHard>]", name, name))
		}
		quotaSet(target, quotaType, args[2], args[3:])

	case "unset":
		if len(args) < 3 {
			fatal(fmt.Errorf("usage: mmctl quota %s unset <fs>[:<fileset>] <%s>", name, name))
		}
		// GPFS has no quota delete operation; zero limits mean "unlimited".
		quotaSet(target, quotaType, args[2], []string{"0", "0", "0", "0"})

	default:
		fatal(fmt.Errorf("unknown quota %s command: %s", name, args[0]))
	}
}

func quotaList(target quotaTarget, quotaType string) {
	path := target.path()
	if quotaType != "" {
		path += "?filter=quotaType=" + url.QueryEscape(quotaType)
	}
	data, err := doScaleGet(path)
	if err != nil {
		fatal(err)
	}

	var resp struct {
		Quotas []quotaEntry `json:"quotas"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		prettyPrint(data)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tOBJECT\tFILESET\tBLOCK_USED\tBLOCK_SOFT\tBLOCK_HARD\tFILES_USED\tFILES_SOFT\tFILES_HARD\tGRACE")
	for _, q := range resp.Quotas {
		fileset := q.FilesetName
		if fileset == "" {
			fileset = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			q.QuotaType, q.ObjectName, fileset,
			formatKB(q.BlockUsage), formatKB(q.BlockQuota), formatKB(q.BlockLimit),
			q.FilesUsage, formatCount(q.FilesQuota), formatCount(q.FilesLimit),
			q.BlockGrace)
	}
	w.Flush()
}

// quotaSet issues a setQuota against the target. limits is
// [blockSoft, blockHard] with an optional [filesSoft, filesHard] pair; each
// value takes the GPFS limit syntax (e.g. "10G", "512M", "0" for unlimited).
func quotaSet(target quotaTarget, quotaType, objectName string, limits []string) {
	if len(limits) < 2 {
		fatal(fmt.Errorf("quota set requires a block soft and hard limit"))
	}
	body := map[string]string{
		"operationType":  "setQuota",
		"quotaType":      quotaType,
		"objectName":     objectName,
		"blockSoftLimit": limits[0],
		"blockHardLimit": limits[1],
	}
	if len(limits) >= 4 {
		body["filesSoftLimit"] = limits[2]
		body["filesHardLimit"] = limits[3]
	}
	payload, err := json.Marshal(body)
	if err != nil {
		fatal(err)
	}

	data, err := doScalePost(target.path(), string(payload))
	if err != nil {
		fatal(err)
	}
	if err := waitJob(data); err != nil {
		fatal(err)
	}
}

// waitJob follows an asynchronous GUI job to completion. Quota writes answer
// with 202 and a job handle, so without this the CLI would report success
// before mmsetquota had even run — and never surface its failure.
func waitJob(data []byte) error {
	var resp struct {
		Jobs []struct {
			JobID  int64  `json:"jobId"`
			Status string `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || len(resp.Jobs) == 0 {
		// Synchronous response; show it as-is.
		prettyPrint(data)
		return nil
	}

	jobID := resp.Jobs[0].JobID
	deadline := time.Now().Add(2 * time.Minute)
	for {
		jobData, err := doScaleGet(fmt.Sprintf("jobs/%d", jobID))
		if err != nil {
			return err
		}
		var jobResp struct {
			Jobs []struct {
				Status string `json:"status"`
				Result struct {
					Commands []string `json:"commands"`
					Stdout   []string `json:"stdout"`
					Stderr   []string `json:"stderr"`
					ExitCode int      `json:"exitCode"`
				} `json:"result"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(jobData, &jobResp); err != nil || len(jobResp.Jobs) == 0 {
			return fmt.Errorf("job %d: unexpected response: %s", jobID, string(jobData))
		}

		job := jobResp.Jobs[0]
		if job.Status == "RUNNING" {
			if time.Now().After(deadline) {
				return fmt.Errorf("job %d still running after 2m", jobID)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		for _, line := range job.Result.Stdout {
			fmt.Println(strings.TrimSpace(line))
		}
		if job.Status != "COMPLETED" || job.Result.ExitCode != 0 {
			return fmt.Errorf("job %d %s (exit %d): %s", jobID, job.Status,
				job.Result.ExitCode, strings.Join(job.Result.Stderr, " "))
		}
		return nil
	}
}

// formatKB renders a GUI block value, which is always in KiB. Zero means
// unlimited in GPFS quota reporting.
func formatKB(kb int64) string {
	if kb == 0 {
		return "-"
	}
	units := []string{"K", "M", "G", "T", "P"}
	value := float64(kb)
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	if value >= 100 || value == float64(int64(value)) {
		return fmt.Sprintf("%.0f%s", value, units[i])
	}
	return fmt.Sprintf("%.1f%s", value, units[i])
}

func formatCount(n int64) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func handleToken(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: mmctl token create <fs1,fs2,...>"))
		}
		fsList := strings.Split(args[1], ",")
		body, _ := json.Marshal(map[string]any{
			"allowedFs": fsList,
		})
		data, code, err := doAdminRequest("POST", "/api/v1/tokens", string(body))
		if err != nil {
			fatal(err)
		}
		if code >= 400 {
			fatal(fmt.Errorf("HTTP %d: %s", code, string(data)))
		}
		prettyPrint(data)

	case "list":
		data, code, err := doAdminRequest("GET", "/api/v1/tokens", "")
		if err != nil {
			fatal(err)
		}
		if code >= 400 {
			fatal(fmt.Errorf("HTTP %d: %s", code, string(data)))
		}
		var tokens []struct {
			ID        string   `json:"id"`
			AllowedFS []string `json:"allowedFs"`
		}
		json.Unmarshal(data, &tokens)

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tALLOWED_FS")
		for _, t := range tokens {
			fmt.Fprintf(w, "%s\t%s\n", t.ID, strings.Join(t.AllowedFS, ","))
		}
		w.Flush()

	case "delete":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: mmctl token delete <id>"))
		}
		_, code, err := doAdminRequest("DELETE", "/api/v1/tokens/"+args[1], "")
		if err != nil {
			fatal(err)
		}
		if code >= 400 {
			fatal(fmt.Errorf("HTTP %d", code))
		}
		fmt.Println("Token deleted.")

	default:
		fatal(fmt.Errorf("unknown token command: %s", args[0]))
	}
}

func prettyPrint(data []byte) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Println(string(data))
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
