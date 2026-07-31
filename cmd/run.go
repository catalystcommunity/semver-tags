/*
Copyright © 2023 Catalyst Squad <info@catalystcommunity.com>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/semver-tags/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the cli",
	Long: `Runs the cli as a straight shot attempt.

With no --directories and no --dir_group, the command analyzes the whole
repository and makes one tag, for example v1.2.3.

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

The outputs hold one value for each group, separated by commas. The order is
every --directories value first, then every --dir_group value.`,
	Run: func(cmd *cobra.Command, args []string) {
		runCommand(initRunConfig())
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.PersistentFlags().Bool("dry_run", false, "when true, do not do any tagging")
	runCmd.PersistentFlags().Bool("github_action", false, "when true, make github action outputs for use in other steps")
	runCmd.PersistentFlags().Bool("output_json", true, "when true, print a json object of results, including dry_run status")
	runCmd.PersistentFlags().Bool("atomic", true, "when true, uses the --atomic flag with git push, otherwise uses a regular push")
	runCmd.PersistentFlags().String("pre_release_string", "", "the string that represents the pre-release part of the semver")
	runCmd.PersistentFlags().String("build_string", "", "the string that represents the build part of the semver")
	runCmd.PersistentFlags().String("remote", "origin", "the name of the remote to push to")
	runCmd.PersistentFlags().String("branch", "main", "the name of the branch to push to, set it empty to push only the tags")
	runCmd.PersistentFlags().StringArray("allowed_types", core.DefaultAllowedTypes(), "the conventional commit types that change a version, a type outside the list changes nothing")
	runCmd.PersistentFlags().StringArray("directories", []string{}, "one subdirectory to apply its own tag for, named after the last part of the path, repeat the flag for more tags, which makes github action outputs comma separated")
	runCmd.PersistentFlags().StringArray("dir_group", []string{}, "a comma separated list of subdirectories that share one tag, named after the first directory in the list, repeat the flag for more tag groups, which makes github action outputs comma separated")

	// bind flags
	err := viper.BindPFlags(runCmd.PersistentFlags())
	// die on error
	if err != nil {
		logging.Log.WithError(err).Error("error initializing configuration")
		panic(err)
	}
}

func initRunConfig() core.Config {
	config := core.Config{
		DryRun:           viper.GetBool("dry_run"),
		GithubAction:     viper.GetBool("github_action"),
		OutputJson:       viper.GetBool("output_json"),
		Atomic:           viper.GetBool("atomic"),
		PreReleaseString: viper.GetString("pre_release_string"),
		BuildString:      viper.GetString("build_string"),
		Remote:           viper.GetString("remote"),
		Branch:           viper.GetString("branch"),
		AllowedTypes:     viper.GetStringSlice("allowed_types"),
		Directories:      viper.GetStringSlice("directories"),
		DirGroups:        viper.GetStringSlice("dir_group"),
	}

	logging.Log.WithField("settings", fmt.Sprintf("%+v", config)).Debug("viper settings")

	return config
}

func runCommand(config core.Config) {
	logging.Log.WithField("settings", fmt.Sprintf("%+v", config)).Info("command run with settings resolved")
	if err := core.DoTagging(config); err != nil {
		logging.Log.WithError(err).Error("error checking commits")
		os.Exit(1)
	}
}
