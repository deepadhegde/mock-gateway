package config

import (
	"crypto/subtle"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Spec string `yaml:"spec"`
}

type Gateway struct {
	Port           int    `yaml:"port"`
	Host           string `yaml:"host"`
	GoogleClientID string `yaml:"google_client_id"`
}

type User struct {
	Name  string   `yaml:"name"`
	Token string   `yaml:"token"`
	Role  string   `yaml:"role"` // admin | tester | viewer
	Envs  []string `yaml:"envs"`
}

type Config struct {
	Gateway  Gateway   `yaml:"gateway"`
	Services []Service `yaml:"services"`
	Users    []User    `yaml:"users"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	expanded := os.Expand(string(data), func(key string) string {
		if idx := strings.Index(key, ":-"); idx >= 0 {
			if v := os.Getenv(key[:idx]); v != "" {
				return v
			}
			return key[idx+2:]
		}
		return os.Getenv(key)
	})
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
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

func (c *Config) UserByToken(token string) (*User, bool) {
	for i := range c.Users {
		if c.Users[i].Token == token {
			return &c.Users[i], true
		}
	}
	return nil, false
}

// UserByTokenSafe is timing-safe — prevents token enumeration via response time.
func (c *Config) UserByTokenSafe(token string) (*User, bool) {
	var found *User
	for i := range c.Users {
		if subtle.ConstantTimeCompare([]byte(c.Users[i].Token), []byte(token)) == 1 {
			found = &c.Users[i]
		}
	}
	if found != nil {
		return found, true
	}
	return nil, false
}

func (c *Config) OpenMode() bool {
	return len(c.Users) == 0
}
