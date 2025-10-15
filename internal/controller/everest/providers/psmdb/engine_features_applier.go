// // everest-operator
// // Copyright (C) 2022 Percona LLC
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// // http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.

// // everest-operator
// // Copyright (C) 2022 Percona LLC
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// // http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.

// // everest-operator
// // Copyright (C) 2022 Percona LLC
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// // http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.

// // everest-operator
// // Copyright (C) 2022 Percona LLC
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// // http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.

// everest
// Copyright (C) 2025 Percona LLC
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

	"github.com/AlekSi/pointer"
)

var errShardingNotSupported = errors.New("sharding is not supported for SplitHorizon DNS feature")

type engineFeaturesApplier struct {
	*Provider

	ctx context.Context //nolint:containedctx
}

func (a *engineFeaturesApplier) applyFeatures() error {
	if pointer.Get(a.DB.Spec.EngineFeatures).PSMDB == nil {
		// Nothing to do
		return nil
	}

	var err error
	if err = a.applySplitHorizonDNS(); err != nil {
		return err
	}
	return nil
}

func (a *engineFeaturesApplier) applySplitHorizonDNS() error {
	database := a.DB
	// psmdb := a.PerconaServerMongoDB
	shdcName := database.Spec.EngineFeatures.PSMDB.SplitHorizonDNSConfigName
	if shdcName == "" {
		// SplitHorizon DNS feature is not enabled
		return nil
	}

	if pointer.Get(database.Spec.Sharding).Enabled {
		// SplitHorizon DNS feature can be enabled for unsharded PSMDB clusters so far.
		return errShardingNotSupported
	}

	// psmdb.Spec.Replsets[0].Horizons = psmdbv1.HorizonsSpec{
	// 	"cluster1-rs0-0": {
	// 		"external": "1",
	// 	},
	// }
	return nil
}
