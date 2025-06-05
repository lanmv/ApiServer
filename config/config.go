package config

import (
	"github.com/spf13/viper"
)

// Config stores all configuration of the application.
// The values are read by viper from a config file or environment variable.
type Config struct {
	DatabaseType            string `mapstructure:"database_type"`
	DatabaseConnectionString string `mapstructure:"database_connection_string"`
	ServicePort             string `mapstructure:"service_port"`
	APISecretKey            string `mapstructure:"api_secret_key"`
	LogLevel                string `mapstructure:"log_level"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}
