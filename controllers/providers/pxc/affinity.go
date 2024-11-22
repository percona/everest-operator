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

package pxc

func (p *applier) configureEngineAffinity() {
	affinity := p.DB.Spec.Engine.Affinity
	if affinity == nil {
		return
	}

	p.Spec.PXC.Affinity.Advanced = affinity
}

func (p *applier) configureHAProxyAffinity() {
	affinity := p.DB.Spec.Proxy.Affinity
	if affinity == nil {
		return
	}

	haproxy := p.Spec.HAProxy
	if haproxy == nil {
		return
	}

	aff := haproxy.Affinity
	if aff == nil {
		return
	}

	aff.Advanced = affinity
}

func (p *applier) configureProxySQLAffinity() {
	affinity := p.DB.Spec.Proxy.Affinity
	if affinity == nil {
		return
	}

	proxysql := p.Spec.ProxySQL
	if proxysql == nil {
		return
	}

	aff := proxysql.Affinity
	if aff == nil {
		return
	}

	aff.Advanced = affinity
}
