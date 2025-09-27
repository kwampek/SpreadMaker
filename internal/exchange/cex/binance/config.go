package binance

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API       string `envconfig:"BINANCE_API"`
	SECRET    string `envconfig:"BINANCE_SECRET"`
	BaseUrl   string `envconfig:"BINANCE_BASEURL"`
	Separator string `envconfig:"BINANCE_SEP"`
}

func Load() *Config {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal("Failed to load BINANCE config: %m", err)
	}
	//fmt.Println(cfg)
	return &cfg
}
