package cli

import (
	"fmt"
	"strings"

	"github.com/DotNaos/moodle-services/internal/config"
	"github.com/spf13/cobra"
)

type Options struct {
	ConfigPath        string
	SessionPath       string
	CacheDBPath       string
	FileCacheDir      string
	StatePath         string
	MobileSessionPath string
	ExportDir         string
	Unsanitized       bool
}

var opts Options

var rootCmd = &cobra.Command{
	Use:   "moodle",
	Short: "CLI for FHGR Moodle",
	Long:  "Command-line access to Moodle for listing courses and files, downloading resources, exporting courses, and viewing your timetable.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOutputFlags(); err != nil {
			return err
		}
		if err := ensureMachineOutputAllowed(cmd); err != nil {
			return err
		}
		recordCommandInvocation(cmd)
		return maybeCheckForUpdates(cmd)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return launchTUI(selectorOptions{})
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", config.ConfigPath(), "Config file path")
	rootCmd.PersistentFlags().StringVar(&opts.SessionPath, "session", config.SessionPath(), "Session cookie file path")
	rootCmd.PersistentFlags().StringVar(&opts.CacheDBPath, "cache", config.CacheDBPath(), "SQLite cache path")
	rootCmd.PersistentFlags().StringVar(&opts.FileCacheDir, "files-cache", config.FileCacheDir(), "File cache directory")
	rootCmd.PersistentFlags().StringVar(&opts.StatePath, "state", config.StatePath(), "State file path")
	rootCmd.PersistentFlags().StringVar(&opts.MobileSessionPath, "mobile-session", config.MobileSessionPath(), "Mobile token session file path")
	rootCmd.PersistentFlags().StringVar(&opts.ExportDir, "output-dir", config.ExportDir(), "Output directory")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output machine-readable JSON")
	rootCmd.PersistentFlags().BoolVar(&outputYAML, "yaml", false, "Output machine-readable YAML")
	rootCmd.PersistentFlags().BoolVar(&outputYML, "yml", false, "Alias for --yaml")
	rootCmd.PersistentFlags().BoolVar(&opts.Unsanitized, "unsanitized", false, "Preserve raw scraped names instead of sanitized defaults")

	rootCmd.SetHelpTemplate(fmt.Sprintf("%s\n\nDefault paths:\n  config: %s\n  session: %s\n  mobile session: %s\n  cache: %s\n  files: %s\n  state: %s\n  output: %s\n", rootCmd.HelpTemplate(), config.ConfigPath(), config.SessionPath(), config.MobileSessionPath(), config.CacheDBPath(), config.FileCacheDir(), config.StatePath(), config.ExportDir()))
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	markInteractiveOnly(rootCmd)
	installMachineHelp()

	rootCmd.AddCommand(
		completionCmd,
		configCmd,
		bootstrapCmd,
		loginCmd,
		mobileCmd,
		listCmd,
		navCmd,
		openCmd,
		downloadCmd,
		exportCmd,
		printCmd,
		tuiCmd,
		skillCmd,
		logsCmd,
		versionCmd,
		updateCmd,
		serveCmd,
	)
}

func Execute() error {
	err := rootCmd.Execute()
	logCommandResult(err)
	if err != nil {
		writeCommandError(err)
	}
	return err
}

func commandPathHas(cmd *cobra.Command, name string) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if strings.EqualFold(current.Name(), name) {
			return true
		}
	}
	return false
}
