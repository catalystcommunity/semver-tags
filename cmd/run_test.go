package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/catalystcommunity/semver-tags/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func targetTestCommand(t *testing.T, values ...string) *cobra.Command {
	t.Helper()
	command := &cobra.Command{Use: "test"}
	command.Flags().StringArray("target", nil, "")
	for _, value := range values {
		require.NoError(t, command.Flags().Set("target", value))
	}
	return command
}

func withoutTargetsEnvironment(t *testing.T) {
	t.Helper()
	value, found := os.LookupEnv("TARGETS")
	require.NoError(t, os.Unsetenv("TARGETS"))
	t.Cleanup(func() {
		if found {
			require.NoError(t, os.Setenv("TARGETS", value))
			return
		}
		require.NoError(t, os.Unsetenv("TARGETS"))
	})
}

func TestResolveTargetConfigsFromYamlValue(t *testing.T) {
	withoutTargetsEnvironment(t)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(strings.NewReader(`
targets:
  - name: public-api
    paths:
      - services/api
      - libs/shared
`)))

	targets, err := resolveTargetConfigs(targetTestCommand(t))

	require.NoError(t, err)
	assert.Equal(t, []core.TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api", "libs/shared"},
	}}, targets)
}

func TestResolveTargetConfigsFromEnvironment(t *testing.T) {
	t.Setenv(
		"TARGETS",
		"public-api=services/api,libs/shared worker=services/worker",
	)

	targets, err := resolveTargetConfigs(targetTestCommand(t))

	require.NoError(t, err)
	assert.Equal(t, []core.TargetConfig{
		{Name: "public-api", Paths: []string{"services/api", "libs/shared"}},
		{Name: "worker", Paths: []string{"services/worker"}},
	}, targets)
}

func TestTargetFlagReplacesEnvironmentAndFile(t *testing.T) {
	t.Setenv("TARGETS", "environment=services/worker")
	viper.Set("targets", []map[string]any{{
		"name":  "file",
		"paths": []string{"libs/shared"},
	}})
	t.Cleanup(func() { viper.Set("targets", nil) })

	targets, err := resolveTargetConfigs(targetTestCommand(
		t,
		"first=services/api",
		"second=services/worker,libs/shared",
	))

	require.NoError(t, err)
	assert.Equal(t, []core.TargetConfig{
		{Name: "first", Paths: []string{"services/api"}},
		{Name: "second", Paths: []string{"services/worker", "libs/shared"}},
	}, targets)
}

func TestTargetsEnvironmentReplacesFile(t *testing.T) {
	t.Setenv("TARGETS", "environment=services/worker")
	viper.Set("targets", []map[string]any{{
		"name":  "file",
		"paths": []string{"libs/shared"},
	}})
	t.Cleanup(func() { viper.Set("targets", nil) })

	targets, err := resolveTargetConfigs(targetTestCommand(t))

	require.NoError(t, err)
	assert.Equal(t, []core.TargetConfig{{
		Name: "environment", Paths: []string{"services/worker"},
	}}, targets)
}
