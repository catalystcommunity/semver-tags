/*
Copyright © 2023 Catalyst Squad <info@catalystcommunity.com>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/semver-tags/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Calculate and create semantic version tags",
	Long: `Calculate and create semantic version tags.

With no --directories, --dir_group, or --target value, the command analyzes
the whole repository and makes one tag, for example v1.2.3.

Use --directories to give one subdirectory its own tag. Give the flag one time
for each tag you want. The last part of the directory path names the tag.

  semver-tags run --directories services/api --directories services/worker

That command can make the tags api/v1.2.3 and worker/v0.4.1. A commit that
changes only services/api releases only api.

Use --dir_group to give more than one directory the same tag. Put the
directories in one flag and separate them with commas. The first directory in
the list names the tag. Give the flag one time for each tag group.

  semver-tags run \
    --dir_group "services/api,libs/shared" \
    --dir_group "services/worker,libs/shared"

That command still makes only the tags api and worker. The directory
libs/shared does not get its own tag, because no group starts with it. A commit
in libs/shared releases both api and worker. A commit in only services/api
releases only api. You can start a different job from each tag event, because
each group makes its own tag.

You can use both flags in the same run. Every group must make a different tag
name. The command stops with an error if two groups make the same tag name.

Use --target when a public tag name must not depend on a path name. Put the
name before an equals sign. Separate multiple paths with commas. Repeat the
flag for more targets.

  semver-tags run \
    --target "public-api=services/api,libs/shared" \
    --target "public-worker=services/worker,libs/shared"

The first target can make public-api/v1.2.3. A commit in libs/shared affects
both targets. A target path can name a file or a directory.

The fix type always makes a patch release. The feat type always makes a minor
release. Use --patch_types, --minor_types, and --major_types to configure other
types. For example:

  semver-tags run --patch_types fix --patch_types holiday

Use --allowed_types to limit which configured types can make a release. The
flag is repeatable and also accepts comma-separated values. BREAKING CHANGE is
allowed by default and makes a major release.

Use --short-versions to update mutable major and minor tags with each release.
For v1.3.7, the command also updates v1.3 and v1. This behavior will become the
default in the next major version. Until then, the command writes a migration
warning when neither short-version flag is set. Use --skip-short-versions to
keep only full tags and suppress the warning.

Most output fields hold one comma-separated value for each group or target.
The order is every --directories value first, then every --dir_group value,
and then every --target value.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := initRunConfig(cmd)
		if err != nil {
			logging.Log.WithError(err).Error("error resolving configuration")
			os.Exit(1)
		}
		runCommand(config)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.PersistentFlags().Bool("dry_run", false, "calculate results without creating or pushing tags")
	runCmd.PersistentFlags().Bool("github_action", false, "write outputs for later GitHub Actions steps")
	runCmd.PersistentFlags().Bool("output_json", true, "write the results as a JSON object")
	runCmd.PersistentFlags().Bool("atomic", true, "push the branch and all tags as one atomic operation")
	runCmd.PersistentFlags().String("pre_release_string", "", "set the semantic version pre-release identifier")
	runCmd.PersistentFlags().String("build_string", "", "set the semantic version build identifier")
	runCmd.PersistentFlags().String("remote", "origin", "push tags to this Git remote")
	runCmd.PersistentFlags().String("branch", "main", "push this branch with the tags; set an empty value to push only tags")
	runCmd.PersistentFlags().StringArray("allowed_types", []string{}, "allow only these commit types to change a version; repeat the flag or use commas; the default allows all configured types and BREAKING CHANGE")
	runCmd.PersistentFlags().StringArray("patch_types", core.DefaultPatchTypes(), "make a patch release for these commit types; repeat the flag or use commas; fix is always a patch type")
	runCmd.PersistentFlags().StringArray("minor_types", core.DefaultMinorTypes(), "make a minor release for these commit types; repeat the flag or use commas; feat is always a minor type")
	runCmd.PersistentFlags().StringArray("major_types", core.DefaultMajorTypes(), "make a major release for these commit types; repeat the flag or use commas")
	runCmd.PersistentFlags().Bool("short-versions", false, "also update mutable vMAJOR.MINOR and vMAJOR tags")
	runCmd.PersistentFlags().Bool("skip-short-versions", false, "keep full version tags only and suppress the short-version migration warning")
	runCmd.PersistentFlags().StringArray("directories", []string{}, "tag one path by its base name; repeat the flag for more paths")
	runCmd.PersistentFlags().StringArray("dir_group", []string{}, "tag a comma-separated path group by the first path's base name; repeat the flag for more groups")
	runCmd.PersistentFlags().StringArray("target", []string{}, "define a release target as name=path[,path...]; repeat the flag for more targets")

	err := viper.BindPFlags(runCmd.PersistentFlags())
	if err != nil {
		logging.Log.WithError(err).Error("error initializing configuration")
		panic(err)
	}
	for key, flagName := range map[string]string{
		"short_versions":      "short-versions",
		"skip_short_versions": "skip-short-versions",
	} {
		if err := viper.BindPFlag(key, runCmd.PersistentFlags().Lookup(flagName)); err != nil {
			logging.Log.WithError(err).Error("error initializing configuration")
			panic(err)
		}
	}
}

func resolveTargetConfigs(cmd *cobra.Command) ([]core.TargetConfig, error) {
	if cmd.Flags().Changed("target") {
		values, err := cmd.Flags().GetStringArray("target")
		if err != nil {
			return nil, fmt.Errorf("can not read --target: %w", err)
		}
		return core.ParseTargetSpecifications(values)
	}

	if value, found := os.LookupEnv("TARGETS"); found {
		return core.ParseTargetSpecifications(strings.Fields(value))
	}

	return configuredTargets(viper.UnmarshalKey)
}

func configuredTargets(
	unmarshalKey func(string, any, ...viper.DecoderConfigOption) error,
) ([]core.TargetConfig, error) {
	var targets []core.TargetConfig
	if err := unmarshalKey("targets", &targets); err != nil {
		return nil, fmt.Errorf("can not read targets from the configuration file: %w", err)
	}
	return targets, nil
}

func initRunConfig(cmd *cobra.Command) (core.Config, error) {
	targets, err := resolveTargetConfigs(cmd)
	if err != nil {
		return core.Config{}, err
	}

	config := core.Config{
		DryRun:            viper.GetBool("dry_run"),
		GithubAction:      viper.GetBool("github_action"),
		OutputJson:        viper.GetBool("output_json"),
		Atomic:            viper.GetBool("atomic"),
		PreReleaseString:  viper.GetString("pre_release_string"),
		BuildString:       viper.GetString("build_string"),
		Remote:            viper.GetString("remote"),
		Branch:            viper.GetString("branch"),
		AllowedTypes:      viper.GetStringSlice("allowed_types"),
		PatchTypes:        viper.GetStringSlice("patch_types"),
		MinorTypes:        viper.GetStringSlice("minor_types"),
		MajorTypes:        viper.GetStringSlice("major_types"),
		ShortVersions:     viper.GetBool("short_versions"),
		SkipShortVersions: viper.GetBool("skip_short_versions"),
		Directories:       viper.GetStringSlice("directories"),
		DirGroups:         viper.GetStringSlice("dir_group"),
		Targets:           targets,
	}

	logging.Log.WithField("settings", fmt.Sprintf("%+v", config)).Debug("viper settings")

	return config, nil
}

func runCommand(config core.Config) {
	logging.Log.WithField("settings", fmt.Sprintf("%+v", config)).Info("command run with settings resolved")
	if err := core.DoTagging(config); err != nil {
		logging.Log.WithError(err).Error("error checking commits")
		os.Exit(1)
	}
}
