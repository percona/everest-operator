package cmd

import (
	"github.com/percona/everest-operator/internal/data-importer/cmd/pg"
	"github.com/spf13/cobra"
)

var Root = &cobra.Command{
	Use: "data-importer",
}

func init() {
	Root.AddCommand(pg.Cmd)
}
