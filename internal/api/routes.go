package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
	"github.com/wutz/mmapi/internal/gpfs"
)

type API struct {
	cfg      *config.Config
	executor *gpfs.Executor
	tokens   *auth.TokenStore
}

func RegisterRoutes(api huma.API, cfg *config.Config, tokens *auth.TokenStore, authMw func(http.Handler) http.Handler) {
	a := &API{
		cfg:      cfg,
		executor: gpfs.NewExecutor(),
		tokens:   tokens,
	}

	humaAuth := func(ctx huma.Context, next func(huma.Context)) {
		user, pass, ok := parseBasicAuth(ctx.Header("Authorization"))
		if !ok {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing or invalid authorization")
			return
		}
		token, valid := tokens.ValidateBasicAuth(user, pass)
		if !valid {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid credentials")
			return
		}
		ctx = huma.WithValue(ctx, tokenContextKey{}, token)
		next(ctx)
	}

	// scalemgmt/v2 compatible endpoints

	huma.Register(api, huma.Operation{
		OperationID: "getClusterInfo",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/cluster",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetCluster)

	huma.Register(api, huma.Operation{
		OperationID: "listFilesystems",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/filesystems",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.ListFilesystems)

	huma.Register(api, huma.Operation{
		OperationID: "getFilesystem",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetFilesystem)

	huma.Register(api, huma.Operation{
		OperationID: "createFileset",
		Method:      http.MethodPost,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/filesets",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.CreateFileset)

	huma.Register(api, huma.Operation{
		OperationID: "getFileset",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/filesets/{fileset}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetFileset)

	huma.Register(api, huma.Operation{
		OperationID: "deleteFileset",
		Method:      http.MethodDelete,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/filesets/{fileset}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.DeleteFileset)

	huma.Register(api, huma.Operation{
		OperationID: "linkFileset",
		Method:      http.MethodPost,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/filesets/{fileset}/link",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.LinkFileset)

	huma.Register(api, huma.Operation{
		OperationID: "unlinkFileset",
		Method:      http.MethodDelete,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/filesets/{fileset}/link",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.UnlinkFileset)

	huma.Register(api, huma.Operation{
		OperationID: "setQuota",
		Method:      http.MethodPost,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/quotas",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.SetQuota)

	huma.Register(api, huma.Operation{
		OperationID: "getQuota",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/quotas",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetQuota)

	huma.Register(api, huma.Operation{
		OperationID: "getJob",
		Method:      http.MethodGet,
		Path:        "/scalemgmt/v2/jobs/{jobId}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.GetJob)

	huma.Register(api, huma.Operation{
		OperationID: "createDirectory",
		Method:      http.MethodPost,
		Path:        "/scalemgmt/v2/filesystems/{filesystem}/directory/{relativePath...}",
		Middlewares: huma.Middlewares{humaAuth},
	}, a.CreateDirectory)

	// Admin token management
	huma.Post(api, "/api/v1/tokens", a.CreateToken)
	huma.Get(api, "/api/v1/tokens", a.ListTokens)
	huma.Delete(api, "/api/v1/tokens/{id}", a.DeleteToken)
}

type tokenContextKey struct{}

func tokenFromCtx(ctx context.Context) *auth.Token {
	t, _ := ctx.Value(tokenContextKey{}).(*auth.Token)
	return t
}

func parseBasicAuth(header string) (string, string, bool) {
	if !strings.HasPrefix(header, "Basic ") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(header[6:])
	if err != nil {
		return "", "", false
	}
	user, pass, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", false
	}
	return user, pass, true
}

var _ = fmt.Sprintf
