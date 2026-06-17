package api

import (
	"context"
	"net/http"
)

type ScaleInfo struct {
	ServerVersion string `json:"serverVersion"`
	APIVersion    string `json:"apiVersion"`
}

type GetInfoOutput struct {
	Body struct {
		Info   ScaleInfo   `json:"info"`
		Status ScaleStatus `json:"status"`
	}
}

// GetInfo returns GPFS cluster version information
func (a *API) GetInfo(ctx context.Context, input *struct{}) (*GetInfoOutput, error) {
	resp := &GetInfoOutput{}

	// Return a compatible version that supports the CSI driver's required features
	resp.Body.Info = ScaleInfo{
		ServerVersion: "5.1.9.0",  // IBM Storage Scale version
		APIVersion:    "2.0",       // REST API version
	}
	resp.Body.Status = ScaleStatus{Code: http.StatusOK, Message: "ok"}

	return resp, nil
}
