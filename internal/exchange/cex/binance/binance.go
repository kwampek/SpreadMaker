package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kwampek/spreadmaker2/cmd/mydebug"
	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/all_chains"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

var config *Config = Load()

type Binance struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *Binance {
	return &Binance{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},           //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(200.0, 1), //Timeout: 15 * time.Second},
	}
}

func (b *Binance) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Binance) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func (b *Binance) Name() string {
	return "BINANCE"
}

func (b *Binance) Id() int {
	return exchanges.RExchangesId["BINANCE"]
}

type binanceAsset struct {
	Coin        string               `json:"coin"`
	NetworkList []binanceNetworkInfo `json:"networkList"`
}

type binanceNetworkInfo struct {
	Network                 string  `json:"network"`
	WithdrawEnable          bool    `json:"withdrawEnable"`
	DepositEnable           bool    `json:"depositEnable"`
	WithdrawFee             float64 `json:"withdrawFee,string"`
	MinWithdrawAmount       float64 `json:"minWithdrawAmount,string"`
	WithdrawIntegerMultiple float64 `json:"withdrawIntegerMultiple,string"`
	DepositDesc             string  `json:"depositDesc"`
	WithdrawDesc            string  `json:"withdrawDesc"`
	Coin                    string  `json:"coin"`
}

func (b *Binance) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.binance.com/api/v3/exchangeInfo"

	//resp, err := b.Client.Get(url)
	resp, err := util.SafeOpenPage(b.Client, url)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var result struct {
		Symbols []struct {
			Status     string `json:"status"`
			BaseAsset  string `json:"baseAsset"`
			QuoteAsset string `json:"quoteAsset"`
		} `json:"symbols"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Symbols {
		if pair.QuoteAsset == "USDT" && pair.Status == "TRADING" {
			all_coins[pair.BaseAsset]++
		}
	}

	return nil
}

func (b *Binance) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	endpoint := "https://api.binance.com/sapi/v1/capital/config/getall"

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	query := url.Values{}
	query.Set("timestamp", timestamp)

	mac := hmac.New(sha256.New, []byte(config.SECRET))
	mac.Write([]byte(query.Encode()))
	signature := hex.EncodeToString(mac.Sum(nil))
	query.Set("signature", signature)

	req, err := http.NewRequest("GET", endpoint+"?"+query.Encode(), nil)
	if err != nil {
		fmt.Println("BINANCE Error while creating req: ", err)
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", config.API)

	//resp, err := b.Client.Do(req)
	resp, err := util.SafeOpenReq(b.Client, req)

	if err != nil {
		fmt.Println("BINANCE Error while sending req: ", err)
		return nil, err
	}

	defer resp.Body.Close()

	var data []binanceAsset
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Println("BINANCE Error while decoding req: ", err)
		return nil, err
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, asset := range data {
		if _, exists := b.Symbols[asset.Coin]; !exists {
			continue
		}

		if _, exists := chains[asset.Coin]; !exists {
			chains[asset.Coin] = make(map[int]common.ChainInfo, 10)
			b.Symbols[asset.Coin] = true
		}

		for _, net := range asset.NetworkList {
			chainID, ok := all_chains.ChainId[net.Network]

			if !ok {
				fmt.Println("Unprocessed Chain: ", net.Network)
				continue
			}

			chains[asset.Coin][chainID] = common.ChainInfo{
				Chain:                   net.Network,
				DepositEnabled:          net.DepositEnable,
				WithdrawEnabled:         net.WithdrawEnable,
				FeeWithdraw:             net.WithdrawFee,
				FeeDeposit:              0,
				MinFeeWithdraw:          net.MinWithdrawAmount,
				MaxFeeWithdraw:          0,
				TransactFeeRateWithdraw: 1.0,
				Fetched:                 fetched,
				Accuracy:                18, // Binance не указывает, можно задать дефолт
			}
		}
	}

	fmt.Println("BINANCE 				Fetched All Chains: ")

	return chains, nil
}

func (b *Binance) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol += config.Separator + common.BaseCurrency

	url := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=20", strings.ToUpper(symbol))
	resp, err := b.OrderBookClient.Get(url)
	if err != nil {
		fmt.Println("Error while sending req ", symbol, " ON BINANCE")
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()

	var data struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Println("Error while decoding orderbook ", symbol, " ON BINANCE")
		return common.OrderBook{}, err
	}

	orderBook := common.OrderBook{}
	for _, bid := range data.Bids {
		price, _ := util.ParseFloat(bid[0])
		qty, _ := util.ParseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range data.Asks {
		price, _ := util.ParseFloat(ask[0])
		qty, _ := util.ParseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       BINANCE")
	}
	return orderBook, nil
}

// func signedRequest(apiKey, apiSecret string) ([]byte, error) {
// 	endpoint := "https://api.binance.com/sapi/v1/capital/config/getall"

// 	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
// 	query := url.Values{}
// 	query.Set("timestamp", timestamp)

// 	// Подпись
// 	mac := hmac.New(sha256.New, []byte(apiSecret))
// 	mac.Write([]byte(query.Encode()))
// 	signature := hex.EncodeToString(mac.Sum(nil))
// 	query.Set("signature", signature)

// 	req, err := http.NewRequest("GET", endpoint+"?"+query.Encode(), nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header.Set("X-MBX-APIKEY", apiKey)

// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	return io.ReadAll(resp.Body)
// }
