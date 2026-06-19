package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration for gopher-email.
type Config struct {
	StorageRoot     string `mapstructure:"storage_root"`
	DBPath          string `mapstructure:"db_path"`
	CredentialsFile string `mapstructure:"credentials_file"`
	TokenFile       string `mapstructure:"token_file"`
	InboundLabel    string `mapstructure:"inbound_label"`
	ArchiveLabel    string `mapstructure:"archive_label"`
}

// Load reads the YAML config file at the given path and returns a Config.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Sensible defaults so the tool works with a minimal config file.
	v.SetDefault("storage_root", "./storage")
	v.SetDefault("db_path", "./email_archive.db")
	v.SetDefault("credentials_file", "./credentials.json")
	v.SetDefault("token_file", "./token.json")
	v.SetDefault("inbound_label", "gSave")
	v.SetDefault("archive_label", "gArchive")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	return &cfg, nil
}
