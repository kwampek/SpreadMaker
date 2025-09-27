package coins

// import (
// 	"database/sql"
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"time"

// 	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
// )

// // // DO AFTER
// // const (
// // 	dbPath       = "./coins.db"
// // 	coinsTable   = "coins"
// // 	timeTable    = "update_time"
// // 	CoinsCount   = 10
// // 	CoinsInButch = 10
// // 	ButchesCount = (CoinsCount + CoinsInButch - 1) / CoinsInButch
// // )

// type Coins struct {
// 	CoinsList   []string
// 	LastFetched time.Time
// 	db          *sql.DB
// }

// func (c *Coins) New() *Coins {
// 	db, err := sql.Open("sqlite3", dbPath)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	var lastFetched time.Time
// 	query := `SELECT max(updated_at) FROM update_time`
// 	row := db.QueryRow(query)
// 	err = row.Scan(&lastFetched)

// 	now := time.Now()

// 	if err != nil || now.Sub(lastFetched) > time.Hour {
// 		coinsList, err := FetchCoinsList()
// 		if err == nil {
// 			c.StoreCoinsList()
// 			return &Coins{
// 				CoinsList:   coinsList,
// 				LastFetched: time.Now(),
// 				db:          db,
// 			}
// 		}
// 		log.Fatalf("Cannot fetch coins: %s", err)
// 	}

// 	coinsList, err := LoadCoinsList()
// 	if err != nil {
// 		log.Fatalf("Cannot load coins due to unresolved error: %s", err)
// 	}

// 	return &Coins{
// 		CoinsList:   coinsList,
// 		LastFetched: lastFetched,
// 		db:          db,
// 	}
// }

// func FetchCoinsList() ([]string, error) {
// 	var rexchanges [exchanges.RExchangesCount]exchanges.RExchange
// 	client := http.Client{
// 		Timeout: 10 * time.Second,
// 	}
// 	for j := 0; j < exchanges.RExchangesCount; j++ {
// 		rexchanges[j] = NewR(exchanges.RExchanges[j], client)
// 	}
// 	return nil, fmt.Errorf("TODO")
// }

// func LoadCoinsList() ([]string, error) {
// 	return nil, fmt.Errorf("TODO")
// }

// func (c *Coins) StoreCoinsList() {

// }

// var (
// 	LCoins = []string{
// 		"BTC",
// 		"ETH",
// 		"USDT",
// 		"XRP",
// 		"BNB",
// 		"SOL",
// 		"USDC",
// 		"TRX",
// 		"DOGE",
// 		"ADA"}
// )
