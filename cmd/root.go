package cmd

import (
	"github.com/spf13/cobra"
)

func Execute() {

	var rootCmd = &cobra.Command{
		Version: "v1.0.0",
		Use:     "ops",
		Short:   "A command-line tool helps with something operation and maintenance work",
		Long:    `ops是用于进行相关运维工作的CLI工具`,
	}

	rootCmd.AddCommand(buildInitCmd())
	//rootCmd.AddCommand(buildSecCmd())
	//buildSecCmd.AddCommand(buildSecDetect)

	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}
