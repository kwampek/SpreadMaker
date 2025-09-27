package mexc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kwampek/spreadmaker2/cmd/mydebug"
	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/all_chains"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

var config *Config = Load()

type Mexc struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *Mexc {
	// proxyUrl, err := url.Parse("http://127.0.0.1:12334")
	// if err != nil {
	// 	log.Fatal("Turn On VPN or check port (12334)")
	// }

	return &Mexc{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},        //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(40, 1), //Timeout: 15 * time.Second},
		// 500 IN 10 SEC???
	}
}

func (m *Mexc) Name() string {
	return "MEXC"
}

func (m *Mexc) Id() int {
	return exchanges.RExchangesId["MEXC"]
}

func (b *Mexc) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Mexc) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func (mx *Mexc) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.mexc.com/api/v3/exchangeInfo"

	resp, err := mx.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Symbols []struct {
			Symbol     string `json:"symbol"`
			BaseAsset  string `json:"baseAsset"`
			QuoteAsset string `json:"quoteAsset"`
			Status     string `json:"status"`
		} `json:"symbols"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Symbols {
		if pair.QuoteAsset == "USDT" && pair.Status == "1" {
			all_coins[pair.BaseAsset]++
		}
	}

	return nil
}

func (m *Mexc) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	resp, err := m.Client.Get("https://www.mexc.com/open/api/v2/market/coin/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type mexcChain struct {
		Chain             string  `json:"chain"`
		Precision         int     `json:"precision"`
		Fee               float64 `json:"fee"`
		IsWithdrawEnabled bool    `json:"is_withdraw_enabled"`
		IsDepositEnabled  bool    `json:"is_deposit_enabled"`
		WithdrawMinLimit  float64 `json:"withdraw_limit_min"`
		WithdrawMaxLimit  float64 `json:"withdraw_limit_max"`
	}

	type mexcCoinChains struct {
		Currency string      `json:"currency"`
		Coins    []mexcChain `json:"coins"`
		FullName string      `json:"full_name"`
	}

	type mexcResponse struct {
		Code int              `json:"code"`
		Data []mexcCoinChains `json:"data"`
		Msg  string           `json:"msg"`
	}

	var result mexcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("MEXC API error: %s", result.Msg)
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, coin := range result.Data {
		if _, exists := m.Symbols[coin.Currency]; !exists {
			continue
		}

		if _, exists := chains[coin.Currency]; !exists {
			chains[coin.Currency] = make(map[int]common.ChainInfo, exchanges.ExchangesCount)
			m.Symbols[coin.Currency] = true
		}

		for _, chain := range coin.Coins {
			chainID, exists := all_chains.ChainId[chain.Chain]
			if !exists {
				fmt.Println("Unsupported chain: ", chain.Chain)
				continue
			}

			chains[coin.Currency][chainID] = common.ChainInfo{
				Chain:                   chain.Chain,
				DepositEnabled:          chain.IsDepositEnabled,
				WithdrawEnabled:         chain.IsWithdrawEnabled,
				FeeWithdraw:             chain.Fee,
				FeeDeposit:              0,
				MinFeeWithdraw:          chain.WithdrawMinLimit,
				MaxFeeWithdraw:          chain.WithdrawMaxLimit,
				TransactFeeRateWithdraw: 1.0,
				Fetched:                 fetched,
				Accuracy:                uint8(chain.Precision),
			}
		}
	}

	fmt.Println("MEXC 				Fetched All Chains: ")
	return chains, nil
}

func (m *Mexc) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol = symbol + config.Separator + common.BaseCurrency

	url := fmt.Sprintf("https://api.mexc.com/api/v3/depth?symbol=%s&limit=20", symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("MEXC FETCH ORDERBOOK ERROR ", err)
		return common.OrderBook{}, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Go-http-client)")
	req.Header.Set("Accept", "application/json")

	resp, err := m.OrderBookClient.Do(req)
	if err != nil {
		fmt.Println("MEXC FETCH ORDERBOOK ERROR ", err)
		return common.OrderBook{}, err
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Println("MEXC FETCH ORDERBOOK ERROR ", err, "   :  ", string(bodyBytes))
		return common.OrderBook{}, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	defer resp.Body.Close()

	var data struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Println("Cant decode mesage")
		return common.OrderBook{}, err
	}

	orderBook := common.OrderBook{}
	for _, bid := range data.Bids {
		price, _ := parseFloat(bid[0])
		qty, _ := parseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range data.Asks {
		price, _ := parseFloat(ask[0])
		qty, _ := parseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       MEXC")
	}
	return orderBook, nil
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
