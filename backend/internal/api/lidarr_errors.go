package api

import (
	"encoding/json"
	"net/http"

	"github.com/groovarr/groovarr/backend/internal/connections"
)

// LidarrErrorResponse is the body returned by any handler that depends on a
// connected Lidarr when the connection is missing or in an error state. It
// carries enough information for the frontend to render a useful message
// ("Lidarr is disconnected", "Lidarr error: <reason>", "Lidarr is connecting…")
// and a link to the Settings page without making a second round-trip.
type LidarrErrorResponse struct {
	Error      string `json:"error"`      // human-readable summary
	Code       string `json:"code"`       // machine-readable: "not_connected" | "error" | "connecting"
	Service    string `json:"service"`    // always "lidarr" for now
	Status     string `json:"status"`     // ServiceStatus.Status: disconnected | connecting | connected | error
	LastCheck  string `json:"last_check"` // ServiceStatus.LastCheck (RFC3339)
	Detail     string `json:"detail,omitempty"` // underlying error from Lidarr if any
}

// WriteLidarrUnavailable inspects the connection manager and writes a 503 with
// a structured LidarrErrorResponse body. Use this from any handler that needs
// Lidarr to be up. Returns true if a response was written (caller should bail),
// false if Lidarr was available and the caller can proceed.
func WriteLidarrUnavailable(w http.ResponseWriter, cm *connections.Manager) bool {
	status := cm.GetStatus(connections.ServiceLidarr)
	if status == nil || status.Status == connections.StatusConnected {
		return false
	}

	resp := LidarrErrorResponse{
		Service:   connections.ServiceLidarr,
		Status:    status.Status,
		LastCheck: status.LastCheck.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Detail:    status.Error,
	}
	switch status.Status {
	case connections.StatusConnecting:
		resp.Code = "connecting"
		resp.Error = "Lidarr is still connecting. Please try again in a moment."
	case connections.StatusError:
		resp.Code = "error"
		resp.Error = "Lidarr connection error"
		if status.Error != "" {
			resp.Error = "Lidarr error: " + status.Error
		}
	default:
		resp.Code = "not_connected"
		resp.Error = "Lidarr is not connected. Open Settings to configure it."
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(resp)
	return true
}
