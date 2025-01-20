package config

import (
	"errors"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Postgres DBConfig
	Redis    RedisConfig
	S3       S3Config
	Logger   Logger
}

type ServerConfig struct {
	AppVersion   string
	Port         string
	Mode         string
	JwtSecretKey string
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	PgDriver string
}

type RedisConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       string
	DB            int
	MinIdleConns  int
	PoolSize      int
	PoolTimeout   int
	JobQueueKey   string
}

type S3Config struct {
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	InputBucket  string
	OutputBucket string
}

type Logger struct {
	Development       bool
	DisableCaller     bool
	DisableStacktrace bool
	Encoding          string
	Level             string
}

func LoadConfig(filename string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(filename)
	v.AddConfigPath(".")
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFound viper.ConfigFileNotFoundError
		if errors.Is(err, configFileNotFound) {
			return nil, errors.New("config file not found")
		}
		return nil, err
	}
	return v, nil
}

func ParseConfig(v *viper.Viper) (*Config, error) {
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
