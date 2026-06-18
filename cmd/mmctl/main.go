package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
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
  quota list <fs>            List quotas
  quota set <fs> <fileset> <soft> <hard>  Set quota
  token create <fs1,fs2,...> [fileset1,...]  Create access token
  token list                 List tokens
  token delete <id>          Delete token

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

func handleQuota(args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("usage: mmctl quota <list|set> <fs> [args]"))
	}

	switch args[0] {
	case "list":
		data, err := doScaleGet("filesystems/" + args[1] + "/quotas")
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	case "set":
		if len(args) < 5 {
			fatal(fmt.Errorf("usage: mmctl quota set <fs> <fileset> <softLimit> <hardLimit>"))
		}
		body := fmt.Sprintf(`{"operationType":"setQuota","quotaType":"fileset","objectName":"%s","blockSoftLimit":"%s","blockHardLimit":"%s"}`, args[2], args[3], args[4])
		data, err := doScalePost("filesystems/"+args[1]+"/quotas", body)
		if err != nil {
			fatal(err)
		}
		prettyPrint(data)

	default:
		fatal(fmt.Errorf("unknown quota command: %s", args[0]))
	}
}

func handleToken(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: mmctl token create <fs1,fs2,...> [fileset1,...]"))
		}
		fsList := strings.Split(args[1], ",")
		var fsetList []string
		if len(args) > 2 {
			fsetList = strings.Split(args[2], ",")
		}
		body, _ := json.Marshal(map[string]any{
			"allowedFs":      fsList,
			"allowedFileset": fsetList,
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
