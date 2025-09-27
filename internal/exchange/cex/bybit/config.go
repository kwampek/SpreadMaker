package bybit

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	API           string `envconfig:"X_BYBIT_API_KEY"`
	SECRET        string `envconfig:"X_BYBIT_SECRET_KEY"`
	BaseUrl       string `envconfig:"BYBIT_BASEURL"`
	ChainEndpoint string `envconfig:"CHAINS_END"`
	RecvWindow    string `envconfig:"RECV"`
	Separator     string `envconfig:"BYBIT_SEP"`
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
