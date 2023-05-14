/*
Copyright © 2023 Catalyst Squad <info@catalystsquad.com>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/catalystsquad/app-utils-go/logging"
	"github.com/catalystsquad/semver-tags/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the cli",
	Long:  `Runs the cli as a straight shot attempt.`,
	Run: func(cmd *cobra.Command, args []string) {
		config := initRunCmdConfig()

		runCommand(config)
	},
}

type runCmdConfig struct {
	DryRun           bool
	GithubAction     bool
	OutputJson       bool
	PreReleaseString string
	BuildString      string
	AllowedTypes     []string
	Directories      []string
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.PersistentFlags().Bool("dry_run", false, "when true, do not do any tagging or writing to changelog")
	runCmd.PersistentFlags().Bool("github_action", false, "when true, make github action outputs for use in other steps")
	runCmd.PersistentFlags().Bool("output_json", true, "when true, print a json object of results, including dry_run status")
	runCmd.PersistentFlags().String("pre_release_string", "", "the string that represents the pre-release part of the semver")
	runCmd.PersistentFlags().String("build_string", "", "the string that represents the build part of the semver")
	runCmd.PersistentFlags().StringArray("allowed_types", []string{"fix", "feat", "chore", "build", "docs", "ci"}, "conventional commit types allowed")
	runCmd.PersistentFlags().StringArray("directories", []string{}, "the subdirectories to apply tags for, which makes github action outputs comma separated")

	// bind flags
	err := viper.BindPFlags(runCmd.PersistentFlags())
	// die on error
	if err != nil {
		logging.Log.WithError(err).Error("error initializing configuration")
		panic(err)
	}
}

func initRunCmdConfig() *runCmdConfig {
	// instantiate config struct
	config := &runCmdConfig{}

	config.DryRun = viper.GetBool("dry_run")
	config.GithubAction = viper.GetBool("github_action")
	config.OutputJson = viper.GetBool("output_json")
	config.PreReleaseString = viper.GetString("pre_release_string")
	config.BuildString = viper.GetString("build_string")
	config.AllowedTypes = viper.GetStringSlice("allowed_types")
	config.Directories = viper.GetStringSlice("directories")

	logging.Log.WithField("settings", fmt.Sprintf("%+v", *config)).Debug("viper settings")

	return config
}

func runCommand(config *runCmdConfig) {
	logging.Log.WithField("settings", fmt.Sprintf("%+v", *config)).Info("command run with settings resolved")
	err := core.DoTagging(config.DryRun, config.GithubAction, config.OutputJson, config.PreReleaseString, config.BuildString, config.Directories)
	if err != nil {
		logging.Log.WithError(err).Error("error checking commits")
		os.Exit(1)
	}
}
