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

// Package main ...
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/percona/everest-operator/internal/migrate/migrator"
)

/*
	How to test migrations locally.

Tilt installation does not contain the migration initContainer. To test the migration locally:

 1. everest-operator Makefile has 2 make targets "k3d-cluster-up" and "k3d-cluster-down" which you can use to setup/tear down k3d clusters
 2. Use the make target "docker-build" to locally build a percona/everest-operator:0.0.0 image based off your local code changes
 3. Use the make target "k3d-upload-image" to copy the locally built docker image onto the k3d cluster
 4. Install dev-latest normally, either via Helm or everestctl. It will use the locally copied 0.0.0 image instead of the one from Docker.
    helm local installation example:
    $ cd percona-helm-charts/charts/everest
    $ helm install everest . --namespace everest-system --create-namespace

When you need to re-build the code, simply run step 2 and 3 and restart the everest-operator manually. */

const contextTimetout = 30 * time.Second

func main() {
	pCtx := context.Background()

	ctx, stop := signal.NotifyContext(pCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	root.SetContext(ctx)

	if err := root.Execute(); err != nil {
		panic(err)
	}
}

var root = &cobra.Command{
	Use: "migrator",
	Run: func(_ *cobra.Command, _ []string) {
		logger := zap.New(zap.UseDevMode(true))
		m, err := migrator.NewMigrator(logger)
		if err != nil {
			logger.Error(err, "failed to create migrator")
			os.Exit(1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), contextTimetout)
		defer cancel()

		info, err := m.Migrate(ctx)
		if err != nil {
			logger.Error(err, "failed to migrate")
			os.Exit(1)
		}
		if info != "" {
			logger.Info(info)
		}
	},
}
