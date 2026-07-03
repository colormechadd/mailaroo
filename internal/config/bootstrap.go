package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type BootstrapConfig struct {
	Domains   []BootstrapDomain  `yaml:"domains"`
	Users     []BootstrapUser    `yaml:"users"`
	Mailboxes []BootstrapMailbox `yaml:"mailboxes"`
}

type BootstrapDomain struct {
	Domain   string `yaml:"domain"`
	Selector string `yaml:"selector"`
}

type BootstrapUser struct {
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	IsAdmin       bool   `yaml:"is_admin"`
	RecoveryEmail string `yaml:"recovery_email"`
}

type BootstrapMailbox struct {
	Name             string                      `yaml:"name"`
	Users            []string                    `yaml:"users"`
	AddressMappings  []BootstrapAddressMapping    `yaml:"address_mappings"`
	SendingAddresses []BootstrapSendingAddress    `yaml:"sending_addresses"`
}

type BootstrapAddressMapping struct {
	Pattern  string `yaml:"pattern"`
	Priority int    `yaml:"priority"`
}

type BootstrapSendingAddress struct {
	User        string `yaml:"user"`
	Address     string `yaml:"address"`
	DisplayName string `yaml:"display_name"`
}

func LoadBootstrapConfig(path string) (*BootstrapConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bootstrap config: %w", err)
	}

	var cfg BootstrapConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing bootstrap config: %w", err)
	}

	return &cfg, nil
}
