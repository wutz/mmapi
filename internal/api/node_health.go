package api

import (
	"context"
	"net/http"
)

type NodeHealthState struct {
	State      string `json:"state"`
	EntityType string `json:"entityType"`
	Component  string `json:"component"`
}

type GetNodeHealthStatesOutput struct {
	Body struct {
		States []NodeHealthState `json:"states"`
		Status ScaleStatus       `json:"status"`
	}
}

// GetNodeHealthStates returns health states for a node
// The CSI driver checks if the States array is non-empty to determine if the node is healthy
func (a *API) GetNodeHealthStates(ctx context.Context, input *struct {
	NodeName string `path:"nodeName"`
	Filter   string `query:"filter"`
}) (*GetNodeHealthStatesOutput, error) {
	resp := &GetNodeHealthStatesOutput{}

	// For now, always report the node as healthy
	// A real implementation would query mmhealth or mmlsnode status
	resp.Body.States = []NodeHealthState{
		{
			State:      "HEALTHY",
			EntityType: "NODE",
			Component:  "GPFS",
		},
	}
	resp.Body.Status = ScaleStatus{Code: http.StatusOK, Message: "ok"}

	return resp, nil
}
