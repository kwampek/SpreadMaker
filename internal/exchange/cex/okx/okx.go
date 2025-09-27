package okx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

type Okx struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func (o *Okx) Name() string {
	return "OKX"
}

func (o *Okx) Id() int {
	return exchanges.RExchangesId["OKX"]
}

func (b *Okx) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Okx) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func New() *Okx {
	return &Okx{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},           //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(100.0, 1), //Timeout: 15 * time.Second},
	} // 130
}

func (okx *Okx) FetchAllCoins(all_coins map[string]int) error {
	url := "https://www.okx.com/api/v5/public/instruments?instType=SPOT"

	resp, err := okx.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstId   string `json:"instId"`
			BaseCcy  string `json:"baseCcy"`
			QuoteCcy string `json:"quoteCcy"`
			State    string `json:"state"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Data {
		if pair.QuoteCcy == "USDT" && pair.State == "live" {
			all_coins[pair.BaseCcy]++
		}
	}

	return nil
}

func (o *Okx) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	const url = "https://www.okx.com/api/v5/asset/currencies"
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Create prehash string for signing
	method := "GET"
	requestPath := "/api/v5/asset/currencies"
	body := "" // GET request has empty body
	prehash := timestamp + method + requestPath + body

	// Create signature
	h := hmac.New(sha256.New, []byte(config.SECRET))
	h.Write([]byte(prehash))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Create HTTP request
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// Set required OKX headers
	req.Header.Set("OK-ACCESS-KEY", config.API)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", config.SECRET)
	req.Header.Set("Content-Type", "application/json")

	// Perform request
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Ccy       string `json:"ccy"`
			Chain     string `json:"chain"`
			CanDep    bool   `json:"canDep"`
			CanWd     bool   `json:"canWd"`
			MinWd     string `json:"minWd"`
			MinFee    string `json:"minFee"`
			Fee       string `json:"fee"`
			Precision string `json:"wdTickSz"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != "0" {
		return nil, fmt.Errorf("OKX error: %s", result.Msg)
	}

	// Build result
	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, item := range result.Data {
		coin := item.Ccy
		if _, exists := o.Symbols[coin]; !exists {
			continue
		}

		chain := strings.ToUpper(strings.Split(item.Chain, "-")[0])

		if _, ok := all_chains.ChainId[chain]; !ok {
			continue
		}
		chainId := all_chains.ChainId[chain]

		if chains[coin] == nil {
			chains[coin] = make(map[int]common.ChainInfo)
			o.Symbols[coin] = true
		}

		fee := parseFloat(item.Fee)
		min := parseFloat(item.MinWd)
		acc := parseUint8(item.Precision)

		chains[coin][chainId] = common.ChainInfo{
			Chain:           chain,
			DepositEnabled:  item.CanDep,
			WithdrawEnabled: item.CanWd,
			FeeWithdraw:     fee,
			MinFeeWithdraw:  min,
			Fetched:         fetched,
			Accuracy:        acc,
		}
	}

	fmt.Println("OKX 				Fetched All Chains: ")
	return chains, nil
}

func (o *Okx) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol += config.Separator + common.BaseCurrency
	fmt.Println(symbol)
	url := fmt.Sprintf("https://www.okx.com/api/v5/market/books?instId=%s&sz=20", symbol)
	resp, err := o.OrderBookClient.Get(url)
	if err != nil {
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Asks [][]string `json:"asks"`
			Bids [][]string `json:"bids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return common.OrderBook{}, err
	}
	if result.Code != "0" {
		return common.OrderBook{}, fmt.Errorf("API error: %s", result.Msg)
	}

	ob := common.OrderBook{}
	for _, ask := range result.Data[0].Asks {
		p, _ := util.ParseFloat(ask[0])
		q, _ := util.ParseFloat(ask[1])
		ob.Asks = append(ob.Asks, common.Order{Price: p, Quantity: q})
	}
	for _, bid := range result.Data[0].Bids {
		p, _ := util.ParseFloat(bid[0])
		q, _ := util.ParseFloat(bid[1])
		ob.Bids = append(ob.Bids, common.Order{Price: p, Quantity: q})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       OKX")
	}
	return ob, nil
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseUint8(s string) uint8 {
	v, _ := strconv.Atoi(s)
	return uint8(v)
}
