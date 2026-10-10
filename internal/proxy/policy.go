package proxy

import (
	"net/http"
	"strings"

	"github.com/wutz/mmapi/internal/config"
)

// Features that are denied unless the operator turns them on in the config.
// Each gates endpoints that change cluster or filesystem state and that the
// CSI driver only reaches in optional modes.
const (
	FeatureMount    = "mount"    // mount / unmount a filesystem on nodes
	FeaturePolicies = "policies" // install a filesystem placement policy
	FeatureAFM      = "afm"      // AFM / COS cache volumes and their bucket keys
)

// rule permits a set of methods on a path pattern under /scalemgmt/v2/.
// A pattern is a list of segments: "*" matches exactly one non-empty segment
// and a trailing "**" matches one or more remaining segments (file paths).
type rule struct {
	methods string // space separated, e.g. "GET POST"
	pattern []string
	feature string // empty: always on
}

func r(methods, pattern string) rule {
	return rule{methods: methods, pattern: strings.Split(pattern, "/")}
}

func gated(feature, methods, pattern string) rule {
	rl := r(methods, pattern)
	rl.feature = feature
	return rl
}

// rules is the complete set of endpoints the proxy forwards: what the IBM
// Storage Scale CSI driver and mmctl call. Everything else is denied, which
// keeps cluster-level changes (nodes, config, NSDs, filesystem create / delete
// / change) out of reach of a tenant token even when its filesystem matches.
var rules = []rule{
	// Cluster level probes. Read-only, and the CSI driver needs them at start
	// up and on every operation.
	r("GET", "cluster"),
	r("GET", "config"),
	r("GET", "info"),
	r("GET", "nodes"),
	r("GET", "nodes/*/health/states"),
	r("GET", "nodeclasses/*"),
	r("GET", "jobs/*"),
	r("GET", "filesystems"),
	// Asks the GUI to refresh its fileset cache after a fileset is created.
	r("POST", "refreshTask/enqueue"),

	// Filesystem.
	r("GET", "filesystems/*"),
	r("GET", "filesystems/*/pools"),
	r("GET", "filesystems/*/pools/*"),
	r("GET", "filesystems/*/partition/*"),
	r("GET POST", "filesystems/*/quotas"),

	// Filesets, their links and quotas.
	r("GET POST", "filesystems/*/filesets"),
	r("GET PUT DELETE", "filesystems/*/filesets/*"),
	r("POST DELETE", "filesystems/*/filesets/*/link"),
	r("GET POST", "filesystems/*/filesets/*/quotas"),

	// Snapshots and clones.
	r("GET POST", "filesystems/*/filesets/*/snapshots"),
	r("GET", "filesystems/*/filesets/*/snapshots/latest"),
	r("GET DELETE", "filesystems/*/filesets/*/snapshots/*"),
	r("PUT", "filesystems/*/filesets/*/snapshotCloneSplit"),
	r("GET", "filesystems/*/filesets/*/snapshotCloneChilds/*/path/**"),
	r("PUT", "filesystems/*/filesets/*/snapshotCopy/*/path/**"),
	r("PUT", "filesystems/*/filesets/*/snapshotCloneCopy/*/path/**"),
	r("PUT", "filesystems/*/filesets/*/directoryCopy/**"),
	r("PUT", "filesystems/*/directoryCopy/**"),

	// Lightweight volumes: directories and symlinks inside a filesystem.
	r("GET POST DELETE", "filesystems/*/directory/**"),
	r("POST DELETE", "filesystems/*/symlink/**"),
	r("GET", "filesystems/*/owner/**"),

	// Optional, off by default.
	gated(FeatureMount, "PUT", "filesystems/*/mount"),
	gated(FeatureMount, "PUT", "filesystems/*/unmount"),
	gated(FeaturePolicies, "PUT", "filesystems/*/policies"),
	gated(FeatureAFM, "POST", "filesystems/*/filesets/cos"),
	gated(FeatureAFM, "PUT", "bucket/keys"),
	gated(FeatureAFM, "DELETE", "bucket/keys/*"),
	gated(FeatureAFM, "POST", "nodes/afm/mapping"),
	gated(FeatureAFM, "DELETE", "nodes/afm/mapping/*"),
}

// policy decides whether a request may be forwarded at all, independent of
// which filesystem it names.
type policy struct {
	rules []rule
}

func newPolicy(cfg *config.Config) *policy {
	enabled := map[string]bool{}
	for _, f := range cfg.AllowFeatures {
		enabled[f] = true
	}
	p := &policy{}
	for _, rl := range rules {
		if rl.feature == "" || enabled[rl.feature] {
			p.rules = append(p.rules, rl)
		}
	}
	return p
}

const apiPrefix = "/scalemgmt/v2/"

// allows reports whether method on path (the decoded request path) matches a
// rule. Paths that do not sit under /scalemgmt/v2/, or that carry dot or empty
// segments, never match: a traversal segment could otherwise make the filesystem
// the proxy checked differ from the one the GUI resolves.
func (p *policy) allows(method, path string) bool {
	if !strings.HasPrefix(path, apiPrefix) {
		return false
	}
	segs := strings.Split(path[len(apiPrefix):], "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return false
		}
	}
	for _, rl := range p.rules {
		if matchMethod(rl.methods, method) && matchPattern(rl.pattern, segs) {
			return true
		}
	}
	return false
}

func matchMethod(methods, method string) bool {
	for _, m := range strings.Fields(methods) {
		if m == method {
			return true
		}
	}
	return false
}

func matchPattern(pattern, segs []string) bool {
	for i, p := range pattern {
		if p == "**" {
			return len(segs) > i
		}
		if i >= len(segs) {
			return false
		}
		if p != "*" && p != segs[i] {
			return false
		}
	}
	return len(segs) == len(pattern)
}

// readOnly reports whether the method cannot change state, so the proxy can
// log only the requests that matter for an audit.
func readOnly(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}
