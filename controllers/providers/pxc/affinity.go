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
