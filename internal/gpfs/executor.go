package gpfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

const gpfsBinDir = "/usr/lpp/mmfs/bin/"

func (e *Executor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, gpfsBinDir+name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s failed: %w\nstderr: %s", name, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (e *Executor) CreateFilesystem(ctx context.Context, fsName string, opts map[string]string) error {
	args := []string{fsName}
	for k, v := range opts {
		args = append(args, fmt.Sprintf("-%s", k), v)
	}
	_, err := e.Run(ctx, "mmcrfs", args...)
	return err
}

func (e *Executor) ListFilesystems(ctx context.Context) ([]FilesystemInfo, error) {
	out, err := e.Run(ctx, "mmlsfs", "all", "-Y")
	if err != nil {
		return nil, err
	}
	return parseFilesystemList(out)
}

func (e *Executor) CreateFileset(ctx context.Context, device, filesetName string, opts map[string]string) error {
	args := []string{device, filesetName}
	for k, v := range opts {
		args = append(args, fmt.Sprintf("--%s", k), v)
	}
	_, err := e.Run(ctx, "mmcrfileset", args...)
	return err
}

func (e *Executor) LinkFileset(ctx context.Context, device, filesetName, junctionPath string) error {
	_, err := e.Run(ctx, "mmlinkfileset", device, filesetName, "-J", junctionPath)
	return err
}

func (e *Executor) ListFilesets(ctx context.Context, device string) ([]FilesetInfo, error) {
	out, err := e.Run(ctx, "mmlsfileset", device, "-Y")
	if err != nil {
		return nil, err
	}
	return parseFilesetList(out)
}

func (e *Executor) DeleteFileset(ctx context.Context, device, filesetName string) error {
	_, err := e.Run(ctx, "mmdelfileset", device, filesetName, "-f")
	return err
}

func (e *Executor) UnlinkFileset(ctx context.Context, device, filesetName string) error {
	_, err := e.Run(ctx, "mmunlinkfileset", device, filesetName)
	return err
}

func (e *Executor) SetFilesetQuota(ctx context.Context, device, filesetName string, blockSoftLimit, blockHardLimit string) error {
	_, err := e.Run(ctx, "mmsetquota", device+":"+filesetName, "--block",
		fmt.Sprintf("%s:%s", blockSoftLimit, blockHardLimit))
	return err
}

type FilesystemInfo struct {
	Name       string `json:"name"`
	MountPoint string `json:"mountPoint"`
	Status     string `json:"status"`
}

type FilesetInfo struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Path         string `json:"path"`
	InodeSpace   string `json:"inodeSpace"`
	MaxInodes    string `json:"maxInodes"`
	AllocInodes  string `json:"allocInodes"`
}

func parseFilesystemList(data []byte) ([]FilesystemInfo, error) {
	var results []FilesystemInfo
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if len(fields) < 7 || fields[0] == "mmlsfs" {
			continue
		}
		results = append(results, FilesystemInfo{
			Name:       fields[6],
			MountPoint: "",
			Status:     "",
		})
	}
	return results, nil
}

func parseFilesetList(data []byte) ([]FilesetInfo, error) {
	var results []FilesetInfo
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if len(fields) < 12 || fields[0] == "mmlsfileset" {
			continue
		}
		results = append(results, FilesetInfo{
			Name:   fields[7],
			Status: fields[8],
			Path:   fields[9],
		})
	}
	return results, nil
}

// MarshalJSON is unused but keeps json import used
var _ = json.Marshal
