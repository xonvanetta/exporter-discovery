package main

import (
	"os"
	"time"

	"github.com/xonvanetta/exporter-discovery/internal/k8s"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Networks  []string     `yaml:"networks"`
	Interval  string       `yaml:"interval"`
	Namespace string       `yaml:"namespace"`
	Workers   int          `yaml:"workers"`
	Modules   []k8s.Module `yaml:"modules"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Namespace: "monitoring",
		Interval:  "60m",
		Workers:   128,
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) getScanInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
