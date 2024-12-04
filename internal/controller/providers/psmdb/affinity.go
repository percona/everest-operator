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

import psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"

func (p *applier) configureEngineAffinity() {
	affinity := p.DB.Spec.Engine.Affinity
	if affinity == nil {
		return
	}

	for _, rs := range p.PerconaServerMongoDB.Spec.Replsets {
		if rs.Affinity == nil {
			rs.Affinity = &psmdbv1.PodAffinity{}
		}
		rs.Affinity.Advanced = affinity
	}
}

func (p *applier) configureShardingAffinity() {
	affinity := p.DB.Spec.Sharding.ConfigServer.Affinity
	if affinity == nil {
		return
	}

	configsvr := p.PerconaServerMongoDB.Spec.Sharding.ConfigsvrReplSet
	if configsvr == nil {
		return
	}

	configsvr.Affinity.Advanced = affinity
}

func (p *applier) configureProxyAffinity() {
	affinity := p.DB.Spec.Proxy.Affinity
	if affinity == nil {
		return
	}

	mongos := p.PerconaServerMongoDB.Spec.Sharding.Mongos
	if mongos == nil {
		return
	}

	mongos.Affinity.Advanced = affinity
}
