// Copyright 2023 Honyrik.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	versionCommand := &cobra.Command{
		Use:   "version",
		Short: "version",
		Long:  `Version`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("v0.0.3")
		},
	}
	RootCommand.AddCommand(versionCommand)
}
