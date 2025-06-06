package main

import "github.com/percona/everest-operator/internal/data-importer/cmd"

func main() {
	if err := cmd.Root.Execute(); err != nil {
		panic(err)
	}
}
