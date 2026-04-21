package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Spec string `yaml:"spec"`
}

type Gateway struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type Roles struct {
	AdminToken  string `yaml:"admin_token"`
	TesterToken string `yaml:"tester_token"`
	ViewerToken string `yaml:"viewer_token"`
}

type Config struct {
	Gateway  Gateway   `yaml:"gateway"`
	Services []Service `yaml:"services"`
	Roles    Roles     `yaml:"roles"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}
	if cfg.Gateway.Port == 0 {
		cfg.Gateway.Port = 9000
	}
	if cfg.Gateway.Host == "" {
		cfg.Gateway.Host = "0.0.0.0"
	}
	return &cfg, nil
}

func (c *Config) Service(name string) (*Service, bool) {
	for i := range c.Services {
		if c.Services[i].Name == name {
			return &c.Services[i], true
		}
	}
	return nil, false
}
