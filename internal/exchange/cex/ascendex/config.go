package ascendex

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API       string `envconfig:"ASCENDEX_API_KEY"`
	SECRET    string `envconfig:"ASCENDEX_SECRET_KEY"`
	BaseUrl   string `envconfig:"ASCENDEX_BASEURL"`
	Separator string `envconfig:"ASCENDEX_SEP"`
}

func Load() *Config {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal("Failed to load ASCENDEX config: %m", err)
	}
	//fmt.Println(cfg)
	return &cfg
}
