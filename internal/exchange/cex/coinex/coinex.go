package coinex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

type CoinEx struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *CoinEx {
	return &CoinEx{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},           //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(100, 100), //Timeout: 15 * time.Second},
	} //390
}

func (b *CoinEx) GetSymbols() map[string]bool {
	return b.Symbols
}

func (c *CoinEx) SetSymbols(symbols map[string]bool) {
	c.Symbols = symbols
}

func (ce *CoinEx) Name() string {
	return "COINEX"
}

func (ce *CoinEx) Id() int {
	return exchanges.RExchangesId["COINEX"]
}

func (ce *CoinEx) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.coinex.com/v1/market/info"

	resp, err := ce.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Структура для парсинга JSON
	var result struct {
		Code int `json:"code"`
		Data map[string]struct {
			BaseCurrency  string `json:"trading_name"`
			QuoteCurrency string `json:"pricing_name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Data {
		if pair.QuoteCurrency == "USDT" {
			all_coins[pair.BaseCurrency]++
		}
	}

	return nil
}

// подпись запроса CoinEx: HMAC SHA256, X-COINEX-KEY, X-COINEX-SIGN, X-COINEX-TIMESTAMP
func (ce *CoinEx) signedGET(path string, params map[string]string) ([]byte, error) {
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	params["timestamp"] = ts

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	signStr := strings.Join(parts, "&")
	mac := hmac.New(sha256.New, []byte(config.SECRET))
	mac.Write([]byte(signStr))
	sig := hex.EncodeToString(mac.Sum(nil))

	params["sign"] = sig
	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	fullURL := fmt.Sprintf("https://api.coinex.com/v2%s?%s", path, query.Encode())
	req, _ := http.NewRequest("GET", fullURL, nil)
	req.Header.Set("X-COINEX-KEY", config.API)
	req.Header.Set("X-COINEX-SIGN", sig)
	req.Header.Set("X-COINEX-TIMESTAMP", ts)

	resp, err := ce.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (ce *CoinEx) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	body, err := ce.signedGET("/assets/info", map[string]string{})
	if err != nil {
		return nil, err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			ShortName string `json:"short_name"`
			ChainInfo []struct {
				ChainName           string `json:"chain_name"`
				WithdrawEnabled     bool   `json:"withdraw_enabled"`
				DepositEnabled      bool   `json:"deposit_enabled"`
				MinWithdrawAmount   string `json:"min_withdraw_amount"`
				WithdrawalFee       string `json:"withdrawal_fee"`
				WithdrawalPrecision string `json:"withdrawal_precision"`
			} `json:"chain_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("API error: %s", res.Message)
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, entry := range res.Data {
		coin := entry.ShortName
		if _, exists := ce.Symbols[coin]; !exists {
			continue
		}

		if chains[coin] == nil {
			chains[coin] = make(map[int]common.ChainInfo, 10)
			ce.Symbols[coin] = true
		}

		for _, ci := range entry.ChainInfo {
			if chainID, ok := all_chains.ChainId[coin]; ok {
				fee := parseFloat(ci.WithdrawalFee)
				minwd := parseFloat(ci.MinWithdrawAmount)
				acc := parseUint8(ci.WithdrawalPrecision)
				chains[coin][chainID] = common.ChainInfo{
					Chain:           coin,
					DepositEnabled:  ci.DepositEnabled,
					WithdrawEnabled: ci.WithdrawEnabled,
					FeeWithdraw:     fee,
					MinFeeWithdraw:  minwd,
					Fetched:         fetched,
					Accuracy:        acc,
				}
			}
		}
	}

	fmt.Println("COINEX 				Fetched All Chains: ")

	return chains, nil
}

func (ce *CoinEx) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol += config.Separator + common.BaseCurrency

	url := fmt.Sprintf("https://api.coinex.com/v2/spot/depth?market=%s&limit=20&interval=0", strings.ToLower(symbol))
	resp, err := ce.OrderBookClient.Get(url)
	if err != nil {
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return common.OrderBook{}, err
	}
	if res.Code != 0 {
		return common.OrderBook{}, fmt.Errorf("API error: %s", res.Message)
	}

	orderBook := common.OrderBook{}
	for _, bid := range res.Data.Bids {
		price, _ := util.ParseFloat(bid[0])
		qty, _ := util.ParseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range res.Data.Asks {
		price, _ := util.ParseFloat(ask[0])
		qty, _ := util.ParseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       COINEX")
	}
	return orderBook, nil
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseUint8(s string) uint8 {
	v, _ := strconv.Atoi(s)
	return uint8(v)
}
