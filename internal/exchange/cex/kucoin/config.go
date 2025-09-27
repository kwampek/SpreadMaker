package kucoin

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API       string `envconfig:"KUCOIN_API_KEY"`
	SECRET    string `envconfig:"KUCOIN_SECRET_KEY"`
	BaseUrl   string `envconfig:"KUCOIN_BASEURL"`
	Separator string `envconfig:"KUCOIN_SEP"`
}

func Load() *Config {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal("Failed to load bybit config: %m", err)
	}
	//fmt.Println(cfg)
	return &cfg
}
