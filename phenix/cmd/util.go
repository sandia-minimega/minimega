package cmd

import (
	"encoding/json"
	"fmt"

	"phenix/api/experiment"
	"phenix/util"

	"github.com/spf13/cobra"
)

func newUtilCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "util",
		Short: "Utility commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newUtilAppJsonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app-json <experiment name>",
		Short: "Print application JSON input for given experiment to STDOUT",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("There was no experiment provided")
			}

			name := args[0]

			exp, err := experiment.Get(name)
			if err != nil {
				err := util.HumanizeError(err, "Unable to get the "+name+" experiment")
				return err.Humanized()
			}

			var m []byte

			if MustGetBool(cmd.Flags(), "pretty") {
				m, err = json.MarshalIndent(exp, "", "  ")
			} else {
				m, err = json.Marshal(exp)
			}

			if err != nil {
				err := util.HumanizeError(err, "Unable to convert experiment to JSON")
				return err.Humanized()
			}

			fmt.Println(string(m))

			return nil
		},
	}

	cmd.Flags().BoolP("pretty", "p", false, "Pretty print the JSON output")

	return cmd
}

func init() {
	utilCmd := newUtilCmd()

	utilCmd.AddCommand(newUtilAppJsonCmd())

	rootCmd.AddCommand(utilCmd)
}
