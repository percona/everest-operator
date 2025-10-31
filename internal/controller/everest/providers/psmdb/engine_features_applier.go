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

package psmdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlekSi/pointer"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	"k8s.io/apimachinery/pkg/types"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/enginefeatures.everest/v1alpha1"
)

var errShardingNotSupported = errors.New("sharding is not supported for SplitHorizon DNS feature")

// EngineFeaturesApplier is responsible for applying engine features to the PerconaServerMongoDB spec.
type EngineFeaturesApplier struct {
	*Provider
}

// NewEngineFeaturesApplier creates a new engineFeaturesApplier.
func NewEngineFeaturesApplier(p *Provider) *EngineFeaturesApplier {
	return &EngineFeaturesApplier{
		Provider: p,
	}
}

// ApplyFeatures applies the engine features to the PerconaServerMongoDB spec.
func (a *EngineFeaturesApplier) ApplyFeatures(ctx context.Context) error {
	if pointer.Get(a.DB.Spec.EngineFeatures).PSMDB == nil {
		// Nothing to do
		return nil
	}

	var err error
	if err = a.applySplitHorizonDNSConfig(ctx); err != nil {
		return err
	}
	return nil
}

// applySplitHorizonDNSConfig applies the SplitHorizon DNS configuration to the PerconaServerMongoDB spec.
func (a *EngineFeaturesApplier) applySplitHorizonDNSConfig(ctx context.Context) error {
	database := a.DB
	psmdb := a.PerconaServerMongoDB
	shdcName := database.Spec.EngineFeatures.PSMDB.SplitHorizonDNSConfigName
	if shdcName == "" {
		// SplitHorizon DNS feature is not enabled
		return nil
	}

	if pointer.Get(database.Spec.Sharding).Enabled {
		// For the time being SplitHorizon DNS feature can be applied for unsharded PSMDB clusters only.
		// Later we may consider adding support for sharded clusters as well.
		return errShardingNotSupported
	}

	shdc := &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{}
	if err := a.C.Get(ctx, types.NamespacedName{Namespace: a.DB.GetNamespace(), Name: shdcName}, shdc); err != nil {
		return err
	}

	horSpec := psmdbv1.HorizonsSpec{}
	for i := range int(database.Spec.Engine.Replicas) {
		horSpec[fmt.Sprintf("%s-rs0-%d", database.GetName(), i)] = map[string]string{
			"external": fmt.Sprintf("%s-rs0-%d-%s.%s",
				database.GetName(),
				i,
				database.GetNamespace(),
				shdc.Spec.BaseDomainNameSuffix),
		}
	}
	psmdb.Spec.Replsets[0].Horizons = horSpec

	// set reference to secret with certificate for external domain
	if psmdb.Spec.Secrets == nil {
		psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{}
	}
	psmdb.Spec.Secrets.SSL = shdc.Spec.TLS.SecretName

	return nil
}
