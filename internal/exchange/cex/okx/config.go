package okx

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API       string `envconfig:"OKX_API"`
	SECRET    string `envconfig:"OKX_SECRET"`
	BaseUrl   string `envconfig:"OKX_BASEURL"`
	Separator string `envconfig:"OKX_SEP"`
}

func Load() *Config {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal("Failed to load OKX config: %m", err)
	}
	//fmt.Println(cfg)
	return &cfg
}
