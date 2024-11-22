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
	affinity := p.DB.Spec.Engine.Affinity
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
