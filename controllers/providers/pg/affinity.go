package pg

import (
	goversion "github.com/hashicorp/go-version"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

func (p *applier) configureEngineAffinity() {
	affinity := p.DB.Spec.Engine.Affinity
	if affinity == nil {
		return
	}

	for _, i := range p.Spec.InstanceSets {
		i.Affinity = affinity
	}
}

func (p *applier) configureProxyAffinity() {
	affinity := p.DB.Spec.Proxy.Affinity
	if affinity == nil {
		p.configureDefaultProxyAffinity()
		return
	}

	proxy := p.Spec.Proxy
	if proxy == nil {
		return
	}

	pgBouncer := proxy.PGBouncer
	if pgBouncer == nil {
		return
	}

	pgBouncer.Affinity = affinity
}

func (p *applier) configureDefaultProxyAffinity() {
	pg := p.PerconaPGCluster

	pg.Spec.Proxy.PGBouncer.Affinity = common.DefaultAffinitySettings().DeepCopy()
	// New affinity settings (added in 1.2.0) must be applied only when PG is upgraded to 2.4.1.
	// This is a temporary workaround to make sure we can make this change without an automatic restart.
	// TODO: fix this once https://perconadev.atlassian.net/browse/EVEREST-1413 is addressed.
	crVersion := goversion.Must(goversion.NewVersion(pg.Spec.CRVersion))
	if p.DB.Status.Status != everestv1alpha1.AppStateNew &&
		crVersion.LessThan(goversion.Must(goversion.NewVersion("2.4.1"))) {
		pg.Spec.Proxy.PGBouncer.Affinity = p.currentPGSpec.Proxy.PGBouncer.Affinity
	}
}
