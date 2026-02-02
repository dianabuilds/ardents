package config

import "testing"

func TestApplyDefaults_Observability(t *testing.T) {
	cfg := Config{}
	ApplyDefaults(&cfg)
	if cfg.Observability.HealthAddr == "" {
		t.Fatal("expected health addr default")
	}
	if cfg.Observability.MetricsAddr == "" {
		t.Fatal("expected metrics addr default")
	}
}

func TestDefault_Observability(t *testing.T) {
	cfg := Default()
	if cfg.Observability.HealthAddr == "" {
		t.Fatal("expected health addr default")
	}
	if cfg.Observability.MetricsAddr == "" {
		t.Fatal("expected metrics addr default")
	}
}
