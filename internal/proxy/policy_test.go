package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wutz/mmapi/internal/config"
)

func TestPolicyAllows(t *testing.T) {
	p := newPolicy(&config.Config{})
	const v = "/scalemgmt/v2/"
	tests := []struct {
		method, path string
		want         bool
	}{
		// What the CSI driver calls.
		{"GET", v + "cluster", true},
		{"GET", v + "config", true},
		{"GET", v + "info", true},
		{"GET", v + "nodes", true},
		{"GET", v + "nodes/n1/health/states", true},
		{"GET", v + "nodeclasses/gw", true},
		{"GET", v + "jobs/42", true},
		{"GET", v + "filesystems", true},
		{"POST", v + "refreshTask/enqueue", true},
		{"GET", v + "filesystems/fs0", true},
		{"GET", v + "filesystems/fs0/quotas", true},
		{"POST", v + "filesystems/fs0/quotas", true},
		{"POST", v + "filesystems/fs0/filesets", true},
		{"GET", v + "filesystems/fs0/filesets/pvc-1", true},
		{"PUT", v + "filesystems/fs0/filesets/pvc-1", true},
		{"DELETE", v + "filesystems/fs0/filesets/pvc-1", true},
		{"POST", v + "filesystems/fs0/filesets/pvc-1/link", true},
		{"DELETE", v + "filesystems/fs0/filesets/pvc-1/link", true},
		{"POST", v + "filesystems/fs0/filesets/pvc-1/snapshots", true},
		{"GET", v + "filesystems/fs0/filesets/pvc-1/snapshots/latest", true},
		{"DELETE", v + "filesystems/fs0/filesets/pvc-1/snapshots/s1", true},
		{"PUT", v + "filesystems/fs0/filesets/pvc-1/snapshotCloneCopy/s1/path/a/b", true},
		{"PUT", v + "filesystems/fs0/filesets/pvc-1/directoryCopy/a/b", true},
		{"POST", v + "filesystems/fs0/directory/a/b/c", true},
		{"DELETE", v + "filesystems/fs0/directory/a", true},
		{"POST", v + "filesystems/fs0/symlink/a/b", true},
		{"GET", v + "filesystems/fs0/owner/a/b", true},

		// Cluster level changes.
		{"PUT", v + "cluster", false},
		{"POST", v + "nodes", false},
		{"DELETE", v + "nodes/n1", false},
		{"PUT", v + "config", false},
		{"POST", v + "nsds", false},
		{"GET", v + "nsds", false},
		{"POST", v + "filesystems", false},
		{"DELETE", v + "jobs/42", false},
		{"POST", v + "info", false},

		// Destructive filesystem calls on a filesystem the token may own.
		{"DELETE", v + "filesystems/fs0", false},
		{"PUT", v + "filesystems/fs0", false},
		{"POST", v + "filesystems/fs0", false},
		{"PUT", v + "filesystems/fs0/quotas", false},
		{"DELETE", v + "filesystems/fs0/quotas", false},
		{"DELETE", v + "filesystems/fs0/filesets", false},
		{"PUT", v + "filesystems/fs0/filesets/pvc-1/snapshots", false},
		{"GET", v + "filesystems/fs0/disks", false},

		// Gated features are denied by default.
		{"PUT", v + "filesystems/fs0/mount", false},
		{"PUT", v + "filesystems/fs0/unmount", false},
		{"PUT", v + "filesystems/fs0/policies", false},
		{"POST", v + "filesystems/fs0/filesets/cos", false},
		{"PUT", v + "bucket/keys", false},
		{"POST", v + "nodes/afm/mapping", false},

		// Malformed or traversal paths.
		{"GET", "/scalemgmt/v1/cluster", false},
		{"GET", v, false},
		{"GET", v + "filesystems//fs0", false},
		{"GET", v + "filesystems/fs0/directory/../../../fs1", false},
		{"GET", v + "filesystems/fs0/owner/./a", false},
		{"GET", v + "filesystems/fs0/owner", false},
		{"GET", v + "filesystems/fs0/filesets/pvc-1/snapshotCopy/s1/path", false},
	}
	for _, tt := range tests {
		if got := p.allows(tt.method, tt.path); got != tt.want {
			t.Errorf("allows(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestPolicyFeatureGates(t *testing.T) {
	const v = "/scalemgmt/v2/"
	tests := []struct {
		feature, method, path string
	}{
		{"mount", "PUT", v + "filesystems/fs0/mount"},
		{"mount", "PUT", v + "filesystems/fs0/unmount"},
		{"policies", "PUT", v + "filesystems/fs0/policies"},
		{"afm", "POST", v + "filesystems/fs0/filesets/cos"},
		{"afm", "PUT", v + "bucket/keys"},
		{"afm", "DELETE", v + "bucket/keys/b1"},
		{"afm", "POST", v + "nodes/afm/mapping"},
		{"afm", "DELETE", v + "nodes/afm/mapping/m1"},
	}
	for _, tt := range tests {
		on := newPolicy(&config.Config{AllowFeatures: []string{tt.feature}})
		if !on.allows(tt.method, tt.path) {
			t.Errorf("feature %q on: %s %s denied", tt.feature, tt.method, tt.path)
		}
		// Enabling a different feature must not open this endpoint.
		other := "mount"
		if tt.feature == "mount" {
			other = "afm"
		}
		off := newPolicy(&config.Config{AllowFeatures: []string{other}})
		if off.allows(tt.method, tt.path) {
			t.Errorf("feature %q off: %s %s allowed", tt.feature, tt.method, tt.path)
		}
	}
}

func TestProxyEndpointDeniedOnOwnFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)
	token, _ := tokens.Create([]string{"fs0"})

	for _, tt := range []struct{ method, path string }{
		{"DELETE", "/scalemgmt/v2/filesystems/fs0"},
		{"PUT", "/scalemgmt/v2/filesystems/fs0/mount"},
		{"PUT", "/scalemgmt/v2/cluster"},
		{"DELETE", "/scalemgmt/v2/nodes/n1"},
		{"GET", "/scalemgmt/v2/filesystems/fs0/directory/../../fs1"},
	} {
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, makeRequest(tt.method, tt.path, token.Secret))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403, got %d: %s", tt.method, tt.path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "endpoint not allowed") {
			t.Errorf("%s %s: unexpected body %s", tt.method, tt.path, w.Body.String())
		}
	}
}

func TestProxyUnauthenticatedBeforePolicy(t *testing.T) {
	_, proxy, _ := setupTestProxy(t)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, makeRequest("DELETE", "/scalemgmt/v2/filesystems/fs0", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
