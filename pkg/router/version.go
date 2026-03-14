package router

import "runtime/debug"

// version is the embedded version of go-trpc, updated at each release.
const version = "0.5.3"

// Version is the current version of go-trpc.
// Resolution order:
//  1. ldflags override (-X ...Version=...)
//  2. go install / go get build info
//  3. Dependency build info (non-replaced only)
//  4. Embedded const (always correct for the released source)
var Version = "v" + version

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	// When used as the main module with a real version (e.g. go install ...@v1.0.0)
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = v
		return
	}
	// When imported as a dependency (non-replaced)
	for _, dep := range info.Deps {
		if dep.Path == "github.com/sebasusnik/go-trpc" {
			if dep.Replace != nil {
				return // replaced module — keep the embedded const
			}
			if dep.Version != "" {
				Version = dep.Version
			}
			return
		}
	}
}
