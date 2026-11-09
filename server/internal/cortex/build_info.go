package cortex

import (
	"golang.org/x/mod/module"
	"net/http"
	"runtime/debug"
)

type libraryBuild struct {
	Module  string  `json:"module"`
	Version *string `json:"version"`
	Commit  *string `json:"commit"`
}
type buildInformation struct {
	Libraries map[string]libraryBuild `json:"libraries"`
}

// linkedBuildInformation reports effective linked modules, never local paths or checksums.
func linkedBuildInformation(info *debug.BuildInfo) buildInformation {
	result := buildInformation{Libraries: map[string]libraryBuild{}}
	for name, path := range map[string]string{"c2j": "github.com/colony-2/c2j", "jobdb": "github.com/colony-2/jobdb"} {
		value := libraryBuild{Module: path}
		if info != nil {
			for _, dep := range info.Deps {
				if dep.Path != path {
					continue
				}
				effective := dep
				for effective.Replace != nil {
					effective = effective.Replace
				}
				if effective.Version == "" || effective.Version == "(devel)" {
					version := "local/development"
					value.Version = &version
				} else {
					value.Module = effective.Path
					version := effective.Version
					value.Version = &version
					if module.IsPseudoVersion(version) {
						if revision, err := module.PseudoVersionRev(version); err == nil {
							value.Commit = &revision
						}
					}
				}
				break
			}
		}
		result.Libraries[name] = value
	}
	return result
}
func (s *Server) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	info, _ := debug.ReadBuildInfo()
	writeJSON(w, http.StatusOK, linkedBuildInformation(info))
}
