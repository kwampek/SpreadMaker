package bitget

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kwampek/spreadmaker2/cmd/mydebug"
	"github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/all_chains"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

var config *Config = Load()

type Bitget struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func (b *Bitget) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Bitget) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func (bg *Bitget) Name() string {
	return "BITGET"
}

func (bd *Bitget) Id() int {
	return exchanges.RExchangesId["BITGET"]
}

func New() *Bitget {
	return &Bitget{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},          //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(16.0, 1), //Timeout: 15 * time.Second},
	}
}

func (bg *Bitget) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.bitget.com/api/v2/spot/public/symbols"

	//resp, err := bg.Client.Get(url)
	resp, err := util.SafeOpenPage(bg.Client, url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Временная структура для парсинга JSON
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Symbol    string `json:"symbol"`
			BaseCoin  string `json:"baseCoin"`
			QuoteCoin string `json:"quoteCoin"`
			Status    string `json:"status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Data {
		if pair.QuoteCoin == "USDT" && pair.Status == "online" {
			all_coins[pair.BaseCoin]++
		}
	}

	return nil
}

func (bg *Bitget) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	// Публичный coins + chains
	//resp, err := http.Get("https://api.bitget.com/api/v2/spot/public/coins")
	resp, err := util.SafeOpenPage(bg.Client, "https://api.bitget.com/api/v2/spot/public/coins")

	if err != nil {
		fmt.Println("Error occurred while sending req BITGET: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	var pub struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Coin   string `json:"coin"`
			Chains []struct {
				Chain        string `json:"chain"`
				Withdrawable string `json:"withdrawable"`
				Rechargeable string `json:"rechargeable"`
				WithdrawFee  string `json:"withdrawFee"`
				MinWithdraw  string `json:"minWithdrawAmount"`
			} `json:"chains"`
		} `json:"data"`
	}

	json.NewDecoder(resp.Body).Decode(&pub)
	if pub.Code != "00000" {
		fmt.Println("Error occured while decoding resp BITGET: ", pub.Msg)
		return nil, fmt.Errorf("error: %s", pub.Msg)
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, entry := range pub.Data {
		coin := entry.Coin
		if _, exists := bg.Symbols[coin]; !exists {
			continue
		}

		if chains[coin] == nil {
			chains[coin] = make(map[int]common.ChainInfo)
			bg.Symbols[coin] = true
		}

		for _, ch := range entry.Chains {
			base := strings.ToUpper(ch.Chain)
			if id, ok := all_chains.ChainId[base]; ok {
				fee := parseFloat(ch.WithdrawFee)
				minw := parseFloat(ch.MinWithdraw)
				chains[coin][id] = common.ChainInfo{
					Chain:                   base,
					DepositEnabled:          ch.Rechargeable == "true",
					WithdrawEnabled:         ch.Withdrawable == "true",
					FeeWithdraw:             fee,
					TransactFeeRateWithdraw: 1.0,
					MinFeeWithdraw:          minw,
					Fetched:                 fetched,
					Accuracy:                8,
				}
			}
		}
	}

	fmt.Println("BITGET 				Fetched All Chains: ")
	return chains, nil
}

func (bg *Bitget) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol = symbol + config.Separator + common.BaseCurrency

	url := fmt.Sprintf("https://api.bitget.com/api/v2/spot/market/orderbook?symbol=%s&limit=20", strings.ToUpper(symbol))
	resp, err := bg.OrderBookClient.Get(url)
	if err != nil {
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()
	var data struct {
		Code string                          `json:"code"`
		Data struct{ Bids, Asks [][]string } `json:"data"`
		Msg  string                          `json:"msg"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	if data.Code != "00000" {
		return common.OrderBook{}, fmt.Errorf("API error: %s", data.Msg)
	}

	orderBook := common.OrderBook{}
	for _, bid := range data.Data.Bids {
		price, _ := util.ParseFloat(bid[0])
		qty, _ := util.ParseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range data.Data.Asks {
		price, _ := util.ParseFloat(ask[0])
		qty, _ := util.ParseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       BITGET")
	}
	return orderBook, nil
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// func (bg Bitget) signedGET(path string, params map[string]string) ([]byte, error) {
// 	// Время
// 	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
// 	params["timestamp"] = ts

// 	// Сортировка
// 	keys := make([]string, 0, len(params))
// 	for k := range params {
// 		keys = append(keys, k)
// 	}
// 	sort.Strings(keys)
// 	query := url.Values{}
// 	var parts []string
// 	for _, k := range keys {
// 		v := params[k]
// 		query.Set(k, v)
// 		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
// 	}
// 	data := strings.Join(parts, "&")

// 	// Подпись
// 	mac := hmac.New(sha256.New, []byte(config.API))
// 	mac.Write([]byte(data))
// 	sig := hex.EncodeToString(mac.Sum(nil))
// 	query.Set("signature", sig)

// 	req, _ := http.NewRequest("GET", "https://api.bitget.com"+path+"?"+query.Encode(), nil)
// 	req.Header.Set("ACCESS-KEY", config.API)
// 	req.Header.Set("ACCESS-PASSPHRASE", config.SECRET)
// 	req.Header.Set("ACCESS-TIMESTAMP", ts)

// 	resp, err := bg.Client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	body, _ := io.ReadAll(resp.Body)
// 	if resp.StatusCode != 200 {
// 		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
// 	}
// 	return body, nil
// }
