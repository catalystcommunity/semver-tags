package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/catalystcommunity/semver-tags/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	config := viper.New()
	config.SetConfigType("yaml")
	require.NoError(t, config.ReadConfig(strings.NewReader(`
targets:
  - name: public-api
    paths:
      - services/api
      - libs/shared
`)))

	targets, err := configuredTargets(config.UnmarshalKey)

	require.NoError(t, err)
	assert.Equal(t, []core.TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api", "libs/shared"},
	}}, targets)
}

func replaceStringArrayFlag(t *testing.T, name string, values []string) {
	t.Helper()
	flag := runCmd.PersistentFlags().Lookup(name)
	require.NotNil(t, flag)
	sliceValue, ok := flag.Value.(pflag.SliceValue)
	require.True(t, ok)
	previousValues := sliceValue.GetSlice()
	previousChanged := flag.Changed
	require.NoError(t, sliceValue.Replace(values))
	flag.Changed = true
	t.Cleanup(func() {
		require.NoError(t, sliceValue.Replace(previousValues))
		flag.Changed = previousChanged
	})
}

func setBoolFlag(t *testing.T, name string, value string) {
	t.Helper()
	flag := runCmd.PersistentFlags().Lookup(name)
	require.NotNil(t, flag)
	previousValue := flag.Value.String()
	previousChanged := flag.Changed
	require.NoError(t, flag.Value.Set(value))
	flag.Changed = true
	t.Cleanup(func() {
		require.NoError(t, flag.Value.Set(previousValue))
		flag.Changed = previousChanged
	})
}

func TestRunConfigUsesAllowedTypesFlag(t *testing.T) {
	withoutTargetsEnvironment(t)
	replaceStringArrayFlag(t, "allowed_types", []string{"fix", "holiday"})

	config, err := initRunConfig(targetTestCommand(t))

	require.NoError(t, err)
	assert.Equal(t, []string{"fix", "holiday"}, config.AllowedTypes)
}

func TestRunConfigUsesBumpTypeEnvironmentVariables(t *testing.T) {
	withoutTargetsEnvironment(t)
	t.Setenv("PATCH_TYPES", "fix holiday")
	t.Setenv("MINOR_TYPES", "feat meatball")
	t.Setenv("MAJOR_TYPES", "earthquake")
	t.Setenv("ALLOWED_TYPES", "fix holiday BREAKING-CHANGE")
	viper.AutomaticEnv()

	config, err := initRunConfig(targetTestCommand(t))

	require.NoError(t, err)
	assert.Equal(t, []string{"fix", "holiday"}, config.PatchTypes)
	assert.Equal(t, []string{"feat", "meatball"}, config.MinorTypes)
	assert.Equal(t, []string{"earthquake"}, config.MajorTypes)
	assert.Equal(t, []string{"fix", "holiday", "BREAKING-CHANGE"}, config.AllowedTypes)
}

func TestRunConfigUsesShortVersionFlags(t *testing.T) {
	withoutTargetsEnvironment(t)
	setBoolFlag(t, "short-versions", "true")

	config, err := initRunConfig(targetTestCommand(t))

	require.NoError(t, err)
	assert.True(t, config.ShortVersions)
	assert.False(t, config.SkipShortVersions)
}

func TestRunConfigUsesShortVersionEnvironmentVariables(t *testing.T) {
	withoutTargetsEnvironment(t)
	t.Setenv("SHORT_VERSIONS", "true")
	t.Setenv("SKIP_SHORT_VERSIONS", "false")
	viper.AutomaticEnv()

	config, err := initRunConfig(targetTestCommand(t))

	require.NoError(t, err)
	assert.True(t, config.ShortVersions)
	assert.False(t, config.SkipShortVersions)
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

func TestRunRejectsPositionalArguments(t *testing.T) {
	err := runCmd.Args(runCmd, []string{"unexpected"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}
