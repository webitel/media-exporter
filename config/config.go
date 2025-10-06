package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/webitel/media-exporter/internal/errors"
)

type AppConfig struct {
	File     string          `json:"-"`
	Consul   *ConsulConfig   `json:"consul,omitempty"`
	Redis    *RedisConfig    `json:"redis,omitempty"`
	Database *DatabaseConfig `json:"database,omitempty"`
	Export   *ExportConfig   `json:"export,omitempty"`
}

type ConsulConfig struct {
	Id            string `json:"id"`
	Address       string `json:"address"`
	PublicAddress string `json:"publicAddress"`
}

type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type DatabaseConfig struct {
	Url string `json:"url"`
}

type ExportConfig struct {
	Workers int `json:"workers"`
}

func LoadConfig() (*AppConfig, error) {
	bindFlagsAndEnv()

	configFile := getConfigFilePath()
	if configFile != "" {
		if err := loadFromFile(configFile); err != nil {
			return nil, err
		}
	}

	cfg := buildAppConfig(configFile)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func bindFlagsAndEnv() {
	pflag.String("config_file", "", "Configuration file in JSON format")

	// database
	pflag.String("data_source", "", "Data source")

	// consul
	pflag.String("id", "", "Service id")
	pflag.String("consul", "", "Host to consul")
	pflag.String("grpc_addr", "", "Public gRPC address with port")

	// redis
	pflag.String("redis_addr", "localhost:6379", "Redis address")
	pflag.String("redis_password", "", "Redis password")
	pflag.Int("redis_db", 0, "Redis DB number")
	// export
	pflag.Int("workers", 5, "Number of concurrent export workers")

	pflag.Parse()

	_ = viper.BindPFlags(pflag.CommandLine)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Explicit mapping
	_ = viper.BindEnv("id", "CONSUL_ID")
	_ = viper.BindEnv("consul", "CONSUL_HOST")
	_ = viper.BindEnv("grpc_addr", "GRPC_ADDR")
	_ = viper.BindEnv("redis_addr", "REDIS_ADDR")
	_ = viper.BindEnv("redis_password", "REDIS_PASSWORD")
	_ = viper.BindEnv("redis_db", "REDIS_DB")
}

func getConfigFilePath() string {
	file := viper.GetString("config_file")
	if file == "" {
		file = os.Getenv("MEDIA_EXPORTER_CONFIG_FILE")
	}
	return file
}

func loadFromFile(path string) error {
	viper.SetConfigFile(path)
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err != nil {
		return errors.New(fmt.Sprintf("could not load config file: %s", err.Error()))
	}
	return nil
}

func buildAppConfig(file string) *AppConfig {
	return &AppConfig{
		File:     file,
		Database: &DatabaseConfig{Url: viper.GetString("data_source")},
		Export:   &ExportConfig{Workers: viper.GetInt("workers")},
		Consul: &ConsulConfig{
			Id:            viper.GetString("id"),
			Address:       viper.GetString("consul"),
			PublicAddress: viper.GetString("grpc_addr"),
		},
		Redis: &RedisConfig{
			Addr:     viper.GetString("redis_addr"),
			Password: viper.GetString("redis_password"),
			DB:       viper.GetInt("redis_db"),
		},
	}
}

func validateConfig(cfg *AppConfig) error {
	if cfg.Database.Url == "" {
		return errors.New("Data source is required")
	}
	if cfg.Consul.Id == "" {
		return errors.New("Service id is required")
	}
	if cfg.Consul.Address == "" {
		return errors.New("Consul address is required")
	}
	if cfg.Consul.PublicAddress == "" {
		return errors.New("gRPC address is required")
	}
	if cfg.Redis.Addr == "" {
		return errors.New("Redis address is required")
	}
	return nil
}
