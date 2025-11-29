package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/spf13/cobra"
)

var versionJSON bool

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print the version, commit, and build date of tvarr.",
	Run: func(cmd *cobra.Command, args []string) {
		info := version.GetInfo()

		if versionJSON {
			output, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(output))
			return
		}

		fmt.Println(version.String())
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output version information as JSON")
	rootCmd.AddCommand(versionCmd)
}
