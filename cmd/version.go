/*
Copyright © 2023 Catalyst Squad <info@catalystcommunity.com>
*/
package cmd

import (
	"runtime/debug"
	"strings"
)

// version holds the release version of the binary. The release build sets it
// with the Go linker:
//
//	go build -ldflags "-X github.com/catalystcommunity/semver-tags/cmd.version=1.2.3" .
//
// A local build leaves it empty, and the binary then reports the module
// version from the Go build information or the word dev.
var version string

// developmentVersion is the version that a local build reports.
const developmentVersion = "dev"

// versionTemplate is the exact output of the --version flag. Installers parse
// the second word, so keep the output on one line.
const versionTemplate = "{{.Name}} {{.Version}}\n"

func init() {
	rootCmd.Version = resolveVersion(version, debug.ReadBuildInfo)
	rootCmd.SetVersionTemplate(versionTemplate)
}

// resolveVersion returns the version that the binary reports. The linker
// value wins. A go install build reports the module version without the v
// prefix. Every other build reports dev.
func resolveVersion(
	linkerVersion string,
	readBuildInfo func() (*debug.BuildInfo, bool),
) string {
	if value := strings.TrimSpace(linkerVersion); value != "" {
		return strings.TrimPrefix(value, "v")
	}

	if info, ok := readBuildInfo(); ok && info != nil {
		value := strings.TrimSpace(info.Main.Version)
		if value != "" && value != "(devel)" {
			return strings.TrimPrefix(value, "v")
		}
	}

	return developmentVersion
}
