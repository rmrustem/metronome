package main

import "time"

type Config struct {
	Probes []Probe `yaml:"probes"`
}

type Probe struct {
	Name               string            `yaml:"name"`
	Proto              string            `yaml:"proto"`
	Target             string            `yaml:"target"`
	Timeout            time.Duration     `yaml:"timeout"`
	Labels             map[string]string `yaml:"labels"`
	SuccessCodes       string            `yaml:"success_codes"`
	Contain            string            `yaml:"contain"`
	NotContain         string            `yaml:"not_contain"`
	InsecureSkipVerify bool              `yaml:"insecure_skip_verify"`
	TLS                bool              `yaml:"tls"`
}

func (p *Probe) Equal(other Probe) bool {
	if p.Name != other.Name ||
		p.Proto != other.Proto ||
		p.Target != other.Target ||
		p.Timeout != other.Timeout ||
		p.SuccessCodes != other.SuccessCodes ||
		p.Contain != other.Contain ||
		p.NotContain != other.NotContain ||
		p.InsecureSkipVerify != other.InsecureSkipVerify ||
		p.TLS != other.TLS {
		return false
	}
	if len(p.Labels) != len(other.Labels) {
		return false
	}
	for k, v := range p.Labels {
		if other.Labels[k] != v {
			return false
		}
	}
	return true
}
