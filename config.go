package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	Probes []Probe `yaml:"probes"`
}

func (c *Config) Validate() error {
	for i := range c.Probes {
		if err := c.Probes[i].Validate(); err != nil {
			return fmt.Errorf("probe %d: %w", i, err)
		}
	}
	return nil
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

type Probe struct {
	Name                string            `yaml:"name"`
	Proto               string            `yaml:"proto"`
	Target              string            `yaml:"target"`
	Timeout             time.Duration     `yaml:"timeout"`
	Labels              map[string]string `yaml:"labels"`
	PrecalculatedLabels prometheus.Labels `yaml:"-"`
	SuccessCodes        string            `yaml:"success_codes"`
	Contain             string            `yaml:"contain"`
	NotContain          string            `yaml:"not_contain"`
	InsecureSkipVerify  bool              `yaml:"insecure_skip_verify"`
	TLS                 bool              `yaml:"tls"`
}

func (p *Probe) Validate() error {
	if p.Name == "" {
		return errors.New("name is required")
	}
	if p.Target == "" {
		return errors.New("target is required")
	}
	proto := strings.ToLower(p.Proto)
	if proto != "http" && proto != "tcp" {
		return fmt.Errorf("invalid proto %q, must be 'http' or 'tcp'", p.Proto)
	}
	return nil
}
