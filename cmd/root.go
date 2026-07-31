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

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "semver-tags",
	Short: "Use git tags to add semantic-release style semver tags on conventional commits",
	Long: `Calculate the next semver tag to add based on semantic-release style semver tags,
	which will analyze conventional commits since the last relevant tags and
	perform the git tags while providing state outputs.
	Normal LOG_LEVEL env var rule applies, if you want clean output, set it to ERROR
	and check for the exit code being 0 before you parse outputs.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .semver-tags.yaml in the current directory)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find the working directory.
		cwd, err := os.Getwd()
		cobra.CheckErr(err)

		// Search config in current working directory with name ".semver-tags" (with extension).
		viper.AddConfigPath(cwd)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".semver-tags")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in. A missing file is normal, so only
	// a file that can not be read is an error.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			cobra.CheckErr(err)
		}
	}
}
