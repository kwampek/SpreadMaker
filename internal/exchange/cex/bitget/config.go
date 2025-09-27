package bitget

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API       string `envconfig:"BITGET_API"`
	SECRET    string `envconfig:"BITGET_SECRET"`
	BaseUrl   string `envconfig:"BITGET_BASEURL"`
	Separator string `envconfig:"BITGET_SEP"`
}

func Load() *Config {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal("Failed to load BITGET config: %m", err)
	}
	//fmt.Println(cfg)
	return &cfg
}
