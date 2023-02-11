package cmd

import (
	"os"
	"path"

	"github.com/spf13/cobra"
)

var RootCommand = &cobra.Command{
	Use:   path.Base(os.Args[0]),
	Short: "gRPC/Rest service Open Policy Agent (OPA)",
	Long:  "gRPC/Rest service Open Policy Agent (OPA)",
}
