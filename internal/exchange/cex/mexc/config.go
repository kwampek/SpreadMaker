package mexc

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	BaseUrl   string `envconfig:"MEXC_BASEURL"`
	Separator string `envconfig:"MEXC_SEP"`
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
