/*
Copyright © 2023 Catalyst Squad <info@catalystcommunity.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "semver-tags",
	Short: "Create semantic version Git tags from conventional commits",
	Long: `Calculate the next semantic version from conventional commits.

The command can create tags for the full repository or for separate release
targets. It can also write JSON or GitHub Actions outputs.

Set LOG_LEVEL=ERROR when another command must parse the JSON output. Always
check for a zero exit status before you use the output.`,
}

// Execute runs the command-line interface.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .semver-tags.yaml in the current directory)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		cwd, err := os.Getwd()
		cobra.CheckErr(err)

		viper.AddConfigPath(cwd)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".semver-tags")
	}

	viper.AutomaticEnv()

	// A configuration file is optional. Report other read errors.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			cobra.CheckErr(err)
		}
	}
}
