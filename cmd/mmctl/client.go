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

func httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func doRequest(method, path, body string, bearer bool) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, apiURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	switch {
	case bearer && adminToken != "":
		req.Header.Set("Authorization", "Bearer "+adminToken)
	case !bearer && apiToken != "":
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

// scaleError turns a GUI error response into the message the GUI itself
// reports, so "Per fileset quota enabled on this filesystem" reaches the
// operator instead of a bare HTTP status.
func scaleError(code int, data []byte) error {
	var resp struct {
		Status struct {
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &resp); err == nil && resp.Status.Message != "" {
		return fmt.Errorf("%s", resp.Status.Message)
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return fmt.Errorf("HTTP %d", code)
	}
	return fmt.Errorf("HTTP %d: %s", code, body)
}

func scaleRequest(method, path string, query url.Values, body string) ([]byte, error) {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	data, code, err := doRequest(method, "/scalemgmt/v2/"+path, body, false)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, scaleError(code, data)
	}
	return data, nil
}

func scaleGet(path string, query url.Values) ([]byte, error) {
	return scaleRequest("GET", path, query, "")
}

func scaleGetInto(path string, query url.Values, v any) error {
	data, err := scaleGet(path, query)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// scaleWrite posts, puts or deletes and follows the job the GUI creates.
// Writes are asynchronous: the GUI answers 202 with a job handle and only the
// job carries the outcome of the mm command it ran, so without following it
// the CLI would report success before mmsetquota had even started.
func scaleWrite(method, path string, query url.Values, body any) error {
	payload := ""
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = string(encoded)
	}

	data, err := scaleRequest(method, path, query, payload)
	if err != nil {
		return err
	}
	return waitJob(data)
}

// jobFailedError marks a job whose mm command failed. mm commands print the
// underlying diagnostics and then a "Command failed" summary line, and main
// reproduces that split.
type jobFailedError struct{ msg string }

func (e *jobFailedError) Error() string { return e.msg }

func waitJob(data []byte) error {
	var resp struct {
		Jobs []struct {
			JobID int64 `json:"jobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || len(resp.Jobs) == 0 {
		// Synchronous response; nothing to follow.
		return nil
	}

	jobID := resp.Jobs[0].JobID
	deadline := time.Now().Add(2 * time.Minute)
	for {
		jobData, err := scaleGet(fmt.Sprintf("jobs/%d", jobID), nil)
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
			if line = strings.TrimSpace(line); line != "" {
				fmt.Println(line)
			}
		}
		if job.Status == "COMPLETED" && job.Result.ExitCode == 0 {
			return nil
		}
		for _, line := range job.Result.Stderr {
			if line = strings.TrimSpace(line); line != "" {
				fmt.Fprintln(os.Stderr, line)
			}
		}
		return &jobFailedError{msg: fmt.Sprintf("job %d %s (exit %d)", jobID, job.Status, job.Result.ExitCode)}
	}
}

// adminRequest talks to the mmapi token management API, which is mmapi's own
// and not part of the Scale GUI.
func adminRequest(method, path, body string) ([]byte, error) {
	data, code, err := doRequest(method, path, body, true)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		var resp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &resp); err == nil && resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", code, strings.TrimSpace(string(data)))
	}
	return data, nil
}
