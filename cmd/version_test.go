package cmd

import (
	"bytes"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildInfoWithVersion(value string) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		info := &debug.BuildInfo{}
		info.Main.Version = value
		return info, true
	}
}

func noBuildInfo() (*debug.BuildInfo, bool) {
	return nil, false
}

func TestResolveVersionPrefersLinkerValue(t *testing.T) {
	assert.Equal(t, "1.2.3", resolveVersion("1.2.3", buildInfoWithVersion("v9.9.9")))
	assert.Equal(t, "1.2.3", resolveVersion(" v1.2.3 ", noBuildInfo))
}

func TestResolveVersionUsesModuleVersion(t *testing.T) {
	assert.Equal(t, "2.0.1", resolveVersion("", buildInfoWithVersion("v2.0.1")))
}

func TestResolveVersionFallsBackToDev(t *testing.T) {
	assert.Equal(t, "dev", resolveVersion("", noBuildInfo))
	assert.Equal(t, "dev", resolveVersion("", buildInfoWithVersion("(devel)")))
	assert.Equal(t, "dev", resolveVersion("", buildInfoWithVersion("")))
}

func TestVersionFlagOutputFormat(t *testing.T) {
	previous := rootCmd.Version
	rootCmd.Version = "4.5.6"
	t.Cleanup(func() { rootCmd.Version = previous })

	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	require.NoError(t, rootCmd.Execute())
	assert.Equal(t, "semver-tags 4.5.6\n", output.String())
}
