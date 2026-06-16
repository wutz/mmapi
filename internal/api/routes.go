package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
	"github.com/wutz/mmapi/internal/gpfs"
)

type API struct {
	cfg       *config.Config
	executor  *gpfs.Executor
	tokens    *auth.TokenStore
}

func RegisterRoutes(api huma.API, cfg *config.Config, tokens *auth.TokenStore, authMw func(http.Handler) http.Handler) {
	a := &API{
		cfg:      cfg,
		executor: gpfs.NewExecutor(),
		tokens:   tokens,
	}

	humaAuth := func(ctx huma.Context, next func(huma.Context)) {
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing authorization")
			return
		}
		secret := authHeader
		if len(secret) > 7 && secret[:7] == "Bearer " {
			secret = secret[7:]
		}
		token, ok := tokens.Validate(secret)
		if !ok {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx = huma.WithValue(ctx, tokenContextKey{}, token)
		next(ctx)
	}

	// Token management (admin, no auth middleware)
	huma.Post(api, "/api/v1/tokens", a.CreateToken)
	huma.Get(api, "/api/v1/tokens", a.ListTokens)
	huma.Delete(api, "/api/v1/tokens/{id}", a.DeleteToken)

	// CSI-compatible filesystem operations (requires token auth)
	huma.Register(api, huma.Operation{
		OperationID: "createVolume",
		Method:      http.MethodPost,
		Path:        "/api/v1/volumes",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.CreateVolume)

	huma.Register(api, huma.Operation{
		OperationID: "deleteVolume",
		Method:      http.MethodDelete,
		Path:        "/api/v1/volumes/{name}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.DeleteVolume)

	huma.Register(api, huma.Operation{
		OperationID: "listVolumes",
		Method:      http.MethodGet,
		Path:        "/api/v1/volumes",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.ListVolumes)

	// Filesystem info
	huma.Register(api, huma.Operation{
		OperationID: "getFilesystem",
		Method:      http.MethodGet,
		Path:        "/api/v1/filesystems/{name}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetFilesystem)
}

type tokenContextKey struct{}

func tokenFromCtx(ctx context.Context) *auth.Token {
	t, _ := ctx.Value(tokenContextKey{}).(*auth.Token)
	return t
}

// Token management

type CreateTokenInput struct {
	Body struct {
		AllowedFS      []string `json:"allowedFs" doc:"Allowed filesystem names"`
		AllowedFileset []string `json:"allowedFileset,omitempty" doc:"Allowed fileset names (multi-fileset mode)"`
	}
}

type TokenInfo struct {
	ID             string   `json:"id"`
	Secret         string   `json:"secret,omitempty"`
	AllowedFS      []string `json:"allowedFs"`
	AllowedFileset []string `json:"allowedFileset,omitempty"`
}

type TokenOutput struct {
	Body TokenInfo
}

type TokenListItem struct {
	ID             string   `json:"id"`
	AllowedFS      []string `json:"allowedFs"`
	AllowedFileset []string `json:"allowedFileset,omitempty"`
}

type TokenListOutput struct {
	Body []TokenListItem
}

func (a *API) CreateToken(ctx context.Context, input *CreateTokenInput) (*TokenOutput, error) {
	token, err := a.tokens.Create(input.Body.AllowedFS, input.Body.AllowedFileset)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create token", err)
	}
	out := &TokenOutput{}
	out.Body.ID = token.ID
	out.Body.Secret = token.Secret
	out.Body.AllowedFS = token.AllowedFS
	out.Body.AllowedFileset = token.AllowedFileset
	return out, nil
}

func (a *API) ListTokens(ctx context.Context, input *struct{}) (*TokenListOutput, error) {
	tokens := a.tokens.List()
	out := &TokenListOutput{}
	for _, t := range tokens {
		out.Body = append(out.Body, TokenListItem{
			ID:             t.ID,
			AllowedFS:     t.AllowedFS,
			AllowedFileset: t.AllowedFileset,
		})
	}
	return out, nil
}

type DeleteTokenInput struct {
	ID string `path:"id"`
}

func (a *API) DeleteToken(ctx context.Context, input *DeleteTokenInput) (*struct{}, error) {
	if err := a.tokens.Delete(input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete token", err)
	}
	return nil, nil
}

// Volume (CSI-compatible) operations

type CreateVolumeInput struct {
	Body struct {
		Name       string            `json:"name" doc:"Volume name"`
		Filesystem string            `json:"filesystem" doc:"Target filesystem"`
		SizeBytes  int64             `json:"sizeBytes,omitempty" doc:"Requested capacity in bytes"`
		Parameters map[string]string `json:"parameters,omitempty" doc:"Additional parameters"`
	}
}

type VolumeOutput struct {
	Body struct {
		Name       string `json:"name"`
		Filesystem string `json:"filesystem"`
		Fileset    string `json:"fileset,omitempty"`
		Path       string `json:"path"`
		SizeBytes  int64  `json:"sizeBytes,omitempty"`
	}
}

func (a *API) CreateVolume(ctx context.Context, input *CreateVolumeInput) (*VolumeOutput, error) {
	token := tokenFromCtx(ctx)
	if token == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	fs := input.Body.Filesystem
	if fs == "" {
		fs = a.cfg.Device
	}

	if err := a.tokens.CheckAccess(token, fs, input.Body.Name); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}

	switch a.cfg.Mode {
	case config.ModeMultiFS:
		return a.createVolumeMultiFS(ctx, input, fs)
	case config.ModeMultiFileset:
		return a.createVolumeMultiFileset(ctx, input, fs)
	default:
		return nil, huma.Error500InternalServerError("unknown mode")
	}
}

func (a *API) createVolumeMultiFS(ctx context.Context, input *CreateVolumeInput, fs string) (*VolumeOutput, error) {
	// In multi-fs mode, each volume maps to a fileset within the allowed filesystem
	filesetName := input.Body.Name
	if err := a.executor.CreateFileset(ctx, fs, filesetName, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to create fileset", err)
	}

	junctionPath := fmt.Sprintf("/gpfs/%s/%s", fs, filesetName)
	if err := a.executor.LinkFileset(ctx, fs, filesetName, junctionPath); err != nil {
		return nil, huma.Error500InternalServerError("failed to link fileset", err)
	}

	if input.Body.SizeBytes > 0 {
		quota := fmt.Sprintf("%d", input.Body.SizeBytes)
		if err := a.executor.SetFilesetQuota(ctx, fs, filesetName, quota, quota); err != nil {
			return nil, huma.Error500InternalServerError("failed to set quota", err)
		}
	}

	out := &VolumeOutput{}
	out.Body.Name = input.Body.Name
	out.Body.Filesystem = fs
	out.Body.Fileset = filesetName
	out.Body.Path = junctionPath
	out.Body.SizeBytes = input.Body.SizeBytes
	return out, nil
}

func (a *API) createVolumeMultiFileset(ctx context.Context, input *CreateVolumeInput, fs string) (*VolumeOutput, error) {
	filesetName := input.Body.Name
	if err := a.executor.CreateFileset(ctx, fs, filesetName, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to create fileset", err)
	}

	junctionPath := fmt.Sprintf("/gpfs/%s/%s", fs, filesetName)
	if err := a.executor.LinkFileset(ctx, fs, filesetName, junctionPath); err != nil {
		return nil, huma.Error500InternalServerError("failed to link fileset", err)
	}

	if input.Body.SizeBytes > 0 {
		quota := fmt.Sprintf("%d", input.Body.SizeBytes)
		if err := a.executor.SetFilesetQuota(ctx, fs, filesetName, quota, quota); err != nil {
			return nil, huma.Error500InternalServerError("failed to set quota", err)
		}
	}

	out := &VolumeOutput{}
	out.Body.Name = input.Body.Name
	out.Body.Filesystem = fs
	out.Body.Fileset = filesetName
	out.Body.Path = junctionPath
	out.Body.SizeBytes = input.Body.SizeBytes
	return out, nil
}

type DeleteVolumeInput struct {
	Name string `path:"name"`
}

func (a *API) DeleteVolume(ctx context.Context, input *DeleteVolumeInput) (*struct{}, error) {
	token := tokenFromCtx(ctx)
	if token == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	fs := a.cfg.Device
	if err := a.tokens.CheckAccess(token, fs, input.Name); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}

	if err := a.executor.UnlinkFileset(ctx, fs, input.Name); err != nil {
		// ignore unlink errors if already unlinked
	}
	if err := a.executor.DeleteFileset(ctx, fs, input.Name); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete fileset", err)
	}

	return nil, nil
}

type ListVolumesInput struct {
	Filesystem string `query:"filesystem" doc:"Filter by filesystem"`
}

type VolumeListItem struct {
	Name       string `json:"name"`
	Filesystem string `json:"filesystem"`
	Path       string `json:"path"`
	Status     string `json:"status"`
}

type VolumeListOutput struct {
	Body []VolumeListItem
}

func (a *API) ListVolumes(ctx context.Context, input *ListVolumesInput) (*VolumeListOutput, error) {
	token := tokenFromCtx(ctx)
	if token == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	fs := input.Filesystem
	if fs == "" {
		fs = a.cfg.Device
	}

	if err := a.tokens.CheckAccess(token, fs, ""); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}

	filesets, err := a.executor.ListFilesets(ctx, fs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list filesets", err)
	}

	out := &VolumeListOutput{}
	for _, f := range filesets {
		if a.cfg.Mode == config.ModeMultiFileset && len(token.AllowedFileset) > 0 {
			if !contains(token.AllowedFileset, f.Name) {
				continue
			}
		}
		out.Body = append(out.Body, VolumeListItem{
			Name:       f.Name,
			Filesystem: fs,
			Path:       f.Path,
			Status:     f.Status,
		})
	}
	return out, nil
}

type GetFilesystemInput struct {
	Name string `path:"name"`
}

type FilesystemOutput struct {
	Body struct {
		Name       string `json:"name"`
		MountPoint string `json:"mountPoint"`
		Status     string `json:"status"`
	}
}

func (a *API) GetFilesystem(ctx context.Context, input *GetFilesystemInput) (*FilesystemOutput, error) {
	token := tokenFromCtx(ctx)
	if token == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if err := a.tokens.CheckAccess(token, input.Name, ""); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}

	out := &FilesystemOutput{}
	out.Body.Name = input.Name
	out.Body.MountPoint = fmt.Sprintf("/gpfs/%s", input.Name)
	out.Body.Status = "mounted"
	return out, nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
