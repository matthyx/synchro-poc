package config

import (
	"time"

	"github.com/matthyx/synchro-poc/domain"
	"github.com/spf13/viper"
)

type Config struct {
	Cluster   string     `mapstructure:"cluster"`
	Nats      NatsConfig `mapstructure:"nats"`
	Resources []Resource `mapstructure:"resources"`
}

type NatsConfig struct {
	Subject string        `mapstructure:"subject"`
	Timeout time.Duration `mapstructure:"timeout"`
	Urls    string        `mapstructure:"urls"`
}

type Resource struct {
	Group    string          `mapstructure:"group"`
	Version  string          `mapstructure:"version"`
	Resource string          `mapstructure:"resource"`
	Strategy domain.Strategy `mapstructure:"strategy"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (Config, error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	viper.SetDefault("nats.subject", "sync")
	viper.SetDefault("nats.timeout", 2*time.Second)

	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		return Config{}, err
	}

	var config Config
	err = viper.Unmarshal(&config)
	return config, err
}
