package api

import "context"

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

type DeleteTokenInput struct {
	ID string `path:"id"`
}

func (a *API) CreateToken(ctx context.Context, input *CreateTokenInput) (*TokenOutput, error) {
	token, err := a.tokens.Create(input.Body.AllowedFS, input.Body.AllowedFileset)
	if err != nil {
		return nil, err
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
			AllowedFS:      t.AllowedFS,
			AllowedFileset: t.AllowedFileset,
		})
	}
	return out, nil
}

func (a *API) DeleteToken(ctx context.Context, input *DeleteTokenInput) (*struct{}, error) {
	return nil, a.tokens.Delete(input.ID)
}
