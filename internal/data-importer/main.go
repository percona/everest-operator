// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/percona/everest-operator/internal/data-importer/cmd"
)

func main() {
	// We must notify our context for SIGINT and SIGTERM signals.
	// This is required so that the importer can shutdown gracefully
	// and clean up any resources it has created.
	// Note that this works only because the Job starts the importer process as PID 1.
	pCtx := context.Background()
	ctx, stop := signal.NotifyContext(pCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd.Root.SetContext(ctx)
	if err := cmd.Root.Execute(); err != nil {
		panic(err)
	}
}
