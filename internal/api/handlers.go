package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
)

var jobCounter atomic.Int64

// Scale API response wrapper
type ScaleStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ScaleJob struct {
	JobID  int64  `json:"jobId"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

func scaleError(code int, msg string) *JobOutput {
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: code, Message: msg}
	return out
}

// Cluster

type GetClusterOutput struct {
	Body struct {
		Cluster struct {
			ClusterSummary struct {
				ClusterID   uint64 `json:"clusterId"`
				ClusterName string `json:"clusterName"`
			} `json:"clusterSummary"`
		} `json:"cluster"`
		Status ScaleStatus `json:"status"`
	}
}

func (a *API) GetCluster(ctx context.Context, input *struct{}) (*GetClusterOutput, error) {
	out := &GetClusterOutput{}
	out.Body.Cluster.ClusterSummary.ClusterID = a.cfg.ClusterID
	out.Body.Cluster.ClusterSummary.ClusterName = a.cfg.ClusterName
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}
	return out, nil
}

// Filesystems

type ListFilesystemsOutput struct {
	Body struct {
		Filesystems []FilesystemDetail `json:"filesystems"`
		Status      ScaleStatus        `json:"status"`
	}
}

type FilesystemDetail struct {
	Name              string           `json:"name"`
	FilesystemName    string           `json:"filesystemName"`
	DefaultMountPoint string           `json:"defaultMountPoint"`
	Mount             *FilesystemMount `json:"mount,omitempty"`
	Quota             *QuotaInfo       `json:"quota,omitempty"`
	TotalDataInKB     int64            `json:"totalDataInKB,omitempty"`
	FreeDataInKB      int64            `json:"freeDataInKB,omitempty"`
}

type QuotaInfo struct {
	QuotasAccountingEnabled string `json:"quotasAccountingEnabled,omitempty"`
	QuotasEnforced          string `json:"quotasEnforced,omitempty"`
	DefaultQuotasEnabled    string `json:"defaultQuotasEnabled,omitempty"`
	PerfilesetQuotas        bool   `json:"perfilesetQuotas,omitempty"`
	FilesetdfEnabled        bool   `json:"filesetdfEnabled,omitempty"`
}

type FilesystemMount struct {
	MountPoint       string `json:"mountPoint"`
	NodesMounted     int    `json:"nodesMounted,omitempty"`
	Status           string `json:"status"`
	AutomaticMount   string `json:"automaticMountOption,omitempty"`
	RemoteDeviceName string `json:"remoteDeviceName"`
}

func (a *API) ListFilesystems(ctx context.Context, input *struct{}) (*ListFilesystemsOutput, error) {
	token := tokenFromCtx(ctx)
	out := &ListFilesystemsOutput{}
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}

	for _, fs := range token.AllowedFS {
		mountPoint, err := a.executor.GetMountPoint(ctx, fs)
		if err != nil {
			mountPoint = "/" + fs
		}
		out.Body.Filesystems = append(out.Body.Filesystems, FilesystemDetail{
			Name:              fs,
			FilesystemName:    fs,
			DefaultMountPoint: mountPoint,
		})
	}
	return out, nil
}

type GetFilesystemInput struct {
	Filesystem string `path:"filesystem"`
}

type GetFilesystemOutput struct {
	Body struct {
		Filesystems []FilesystemDetail `json:"filesystems"`
		Status      ScaleStatus        `json:"status"`
	}
}

func (a *API) GetFilesystem(ctx context.Context, input *GetFilesystemInput) (*GetFilesystemOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, ""); err != nil {
		slog.Warn("access denied", "error", err)
	}

	mountPoint, err := a.executor.GetMountPoint(ctx, input.Filesystem)
	if err != nil {
		slog.Warn("failed to get mount point", "error", err)
		mountPoint = "/" + input.Filesystem
	}

	out := &GetFilesystemOutput{}
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}
	out.Body.Filesystems = []FilesystemDetail{{
		Name:              input.Filesystem,
		FilesystemName:    input.Filesystem,
		DefaultMountPoint: mountPoint,
		Mount: &FilesystemMount{
			MountPoint:       mountPoint,
			Status:           "mounted",
			AutomaticMount:   "yes",
			RemoteDeviceName: input.Filesystem,
		},
		Quota: &QuotaInfo{
			QuotasAccountingEnabled: "user;group;fileset",
			QuotasEnforced:          "user;group;fileset",
			DefaultQuotasEnabled:    "none",
			PerfilesetQuotas:        true,
			FilesetdfEnabled:        false,
		},
	}}
	return out, nil
}

// Filesets

type CreateFilesetInput struct {
	Filesystem string `path:"filesystem"`
	Body       struct {
		FilesetName  string `json:"filesetName"`
		InodeSpace   string `json:"inodeSpace,omitempty"`
		MaxNumInodes string `json:"maxNumInodes,omitempty"`
		AllocInodes  string `json:"allocInodes,omitempty"`
		Comment      string `json:"comment,omitempty"`
	}
}

type JobOutput struct {
	Body struct {
		Status ScaleStatus `json:"status"`
		Jobs   []ScaleJob  `json:"jobs"`
	}
}

func (a *API) CreateFileset(ctx context.Context, input *CreateFilesetInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Body.FilesetName); err != nil {
		return scaleError(403, err.Error()), nil
	}

	opts := map[string]string{}
	if input.Body.InodeSpace != "" {
		opts["inodeSpace"] = input.Body.InodeSpace
	}
	if input.Body.MaxNumInodes != "" {
		opts["maxNumInodes"] = input.Body.MaxNumInodes
	}
	if input.Body.AllocInodes != "" {
		opts["allocInodes"] = input.Body.AllocInodes
	}

	if err := a.executor.CreateFileset(ctx, input.Filesystem, input.Body.FilesetName, opts); err != nil {
		slog.Error("create fileset failed", "error", err)
		return scaleError(500, "failed to create fileset: "+err.Error()), nil
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "created"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

type GetFilesetInput struct {
	Filesystem string `path:"filesystem"`
	Fileset    string `path:"fileset"`
}

type GetFilesetOutput struct {
	Body struct {
		Filesets []FilesetDetail `json:"filesets"`
		Status  ScaleStatus     `json:"status"`
	}
}

type FilesetDetail struct {
	FilesetName    string             `json:"filesetName"`
	FilesystemName string             `json:"filesystemName"`
	Path           string             `json:"path"`
	Status         string             `json:"status"`
	Config         FilesetConfigDetail `json:"config"`
	InodeSpace     string             `json:"inodeSpace,omitempty"`
	MaxInodes      int64              `json:"maxInodes,omitempty"`
	AllocInodes    int64              `json:"allocInodes,omitempty"`
}

type FilesetConfigDetail struct {
	FilesetName    string `json:"filesetName,omitempty"`
	FilesystemName string `json:"filesystemName,omitempty"`
	Path           string `json:"path,omitempty"`
	Status         string `json:"status,omitempty"`
}

func (a *API) GetFileset(ctx context.Context, input *GetFilesetInput) (*GetFilesetOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Fileset); err != nil {
		slog.Warn("access denied", "error", err)
	}

	filesets, err := a.executor.ListFilesets(ctx, input.Filesystem)
	if err != nil {
		slog.Error("list filesets failed", "error", err)
	}

	out := &GetFilesetOutput{}
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}
		for _, f := range filesets {
		if f.Name == input.Fileset {
			out.Body.Filesets = append(out.Body.Filesets, FilesetDetail{
				FilesetName:    f.Name,
				FilesystemName: input.Filesystem,
				Path:           f.Path,
				Status:         f.Status,
				Config: FilesetConfigDetail{
					FilesetName:    f.Name,
					FilesystemName: input.Filesystem,
					Path:           f.Path,
					Status:         f.Status,
				},
			})
			break
		}
	}

	if len(out.Body.Filesets) == 0 {
		out.Body.Status = ScaleStatus{Code: 200, Message: fmt.Sprintf("fileset %q not found", input.Fileset)}
	}
	return out, nil
}

type DeleteFilesetInput struct {
	Filesystem string `path:"filesystem"`
	Fileset    string `path:"fileset"`
}

func (a *API) DeleteFileset(ctx context.Context, input *DeleteFilesetInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Fileset); err != nil {
		return scaleError(403, err.Error()), nil
	}

	if err := a.executor.DeleteFileset(ctx, input.Filesystem, input.Fileset); err != nil {
		slog.Error("delete fileset failed", "error", err)
		return scaleError(500, "failed to delete fileset: "+err.Error()), nil
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "deleted"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

// Link/Unlink

type LinkFilesetInput struct {
	Filesystem string `path:"filesystem"`
	Fileset    string `path:"fileset"`
	Body       struct {
		Path string `json:"path"`
	}
}

func (a *API) LinkFileset(ctx context.Context, input *LinkFilesetInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Fileset); err != nil {
		return scaleError(403, err.Error()), nil
	}

	if err := a.executor.LinkFileset(ctx, input.Filesystem, input.Fileset, input.Body.Path); err != nil {
		// Ignore "already linked" errors
		if strings.Contains(err.Error(), "exit status 56") || strings.Contains(err.Error(), "already linked") || strings.Contains(err.Error(), "already exists") {
			slog.Info("fileset already linked", "fileset", input.Fileset)
		} else {
			slog.Error("link fileset failed", "error", err)
			return scaleError(500, "failed to link fileset: "+err.Error()), nil
		}
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "linked"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

type UnlinkFilesetInput struct {
	Filesystem string `path:"filesystem"`
	Fileset    string `path:"fileset"`
}

func (a *API) UnlinkFileset(ctx context.Context, input *UnlinkFilesetInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Fileset); err != nil {
		return scaleError(403, err.Error()), nil
	}

	if err := a.executor.UnlinkFileset(ctx, input.Filesystem, input.Fileset); err != nil {
		slog.Error("unlink fileset failed", "error", err)
		return scaleError(500, "failed to unlink fileset: "+err.Error()), nil
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "unlinked"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

// Quotas

type SetQuotaInput struct {
	Filesystem string `path:"filesystem"`
	Body       struct {
		OperationType  string `json:"operationType"`
		QuotaType      string `json:"quotaType"`
		ObjectName     string `json:"objectName"`
		BlockHardLimit string `json:"blockHardLimit"`
		BlockSoftLimit string `json:"blockSoftLimit"`
	}
}

func (a *API) SetQuota(ctx context.Context, input *SetQuotaInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, input.Body.ObjectName); err != nil {
		return scaleError(403, err.Error()), nil
	}

	if err := a.executor.SetFilesetQuota(ctx, input.Filesystem, input.Body.ObjectName, input.Body.BlockSoftLimit, input.Body.BlockHardLimit); err != nil {
		slog.Error("set quota failed", "error", err)
		return scaleError(500, "failed to set quota: "+err.Error()), nil
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "quota set"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

type GetQuotaInput struct {
	Filesystem string `path:"filesystem"`
	Filter     string `query:"filter"`
}

type GetQuotaOutput struct {
	Body struct {
		Quotas []QuotaDetail `json:"quotas"`
		Status ScaleStatus   `json:"status"`
	}
}

type QuotaDetail struct {
	QuotaType      string `json:"quotaType"`
	ObjectName     string `json:"objectName"`
	BlockUsage     int64  `json:"blockUsage"`
	BlockHardLimit int64  `json:"blockHardLimit"`
	BlockSoftLimit int64  `json:"blockSoftLimit"`
	FilesystemName string `json:"filesystemName"`
}

func (a *API) GetQuota(ctx context.Context, input *GetQuotaInput) (*GetQuotaOutput, error) {
	out := &GetQuotaOutput{}
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}
	out.Body.Quotas = []QuotaDetail{}
	return out, nil
}

// Jobs

type GetJobInput struct {
	JobID string `path:"jobId"`
}

type GetJobOutput struct {
	Body struct {
		Jobs   []ScaleJob  `json:"jobs"`
		Status ScaleStatus `json:"status"`
	}
}

func (a *API) GetJob(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	out := &GetJobOutput{}
	out.Body.Status = ScaleStatus{Code: 200, Message: ""}
	out.Body.Jobs = []ScaleJob{{JobID: 0, Status: "COMPLETED"}}
	return out, nil
}

type CreateDirectoryInput struct {
	Filesystem   string `path:"filesystem"`
	RelativePath string `path:"relativePath"`
	Body         struct {
		Uid   int    `json:"uid,omitempty"`
		Gid   int    `json:"gid,omitempty"`
		Perms string `json:"permissions,omitempty"`
	}
}

func (a *API) CreateDirectory(ctx context.Context, input *CreateDirectoryInput) (*JobOutput, error) {
	token := tokenFromCtx(ctx)
	if err := a.tokens.CheckAccess(token, input.Filesystem, ""); err != nil {
		return scaleError(403, err.Error()), nil
	}

	// Decode the URL-encoded path
	relativePath := input.RelativePath
	if decoded, err := url.PathUnescape(relativePath); err == nil {
		relativePath = decoded
	}

	if err := a.executor.CreateDirectory(ctx, input.Filesystem, relativePath); err != nil {
		// Ignore "already exists" errors
		if strings.Contains(err.Error(), "File exists") || strings.Contains(err.Error(), "already exists") {
			slog.Info("directory already exists", "path", relativePath)
		} else {
			slog.Error("create directory failed", "error", err)
			return scaleError(500, "failed to create directory: "+err.Error()), nil
		}
	}

	jobID := jobCounter.Add(1)
	out := &JobOutput{}
	out.Body.Status = ScaleStatus{Code: 202, Message: "created"}
	out.Body.Jobs = []ScaleJob{{JobID: jobID, Status: "COMPLETED"}}
	return out, nil
}

