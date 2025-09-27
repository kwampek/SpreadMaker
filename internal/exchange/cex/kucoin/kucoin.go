package kucoin

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
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

type Kucoin struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *Kucoin {
	return &Kucoin{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},          //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(35.0, 1), //Timeout: 15 * time.Second},
	}
	//39
}

func (b *Kucoin) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Kucoin) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func (k *Kucoin) Name() string {
	return "KUCOIN"
}

func (k *Kucoin) Id() int {
	return exchanges.RExchangesId["KUCOIN"]
}

func (k *Kucoin) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.kucoin.com/api/v1/symbols"

	resp, err := k.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code string `json:"code"`
		Data []struct {
			Symbol        string `json:"symbol"`
			BaseCurrency  string `json:"baseCurrency"`
			QuoteCurrency string `json:"quoteCurrency"`
			EnableTrading bool   `json:"enableTrading"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Data {
		if pair.QuoteCurrency == "USDT" && pair.EnableTrading {
			all_coins[pair.BaseCurrency]++
		}
	}

	return nil
}

func (k *Kucoin) FetchOrderBook(symbol, _ string) (common.OrderBook, error) {
	symbol = symbol + config.Separator + common.BaseCurrency

	endpoint := fmt.Sprintf("%s/api/v1/market/orderbook/level2_20?symbol=%s", config.BaseUrl, symbol)

	resp, err := k.OrderBookClient.Get(endpoint)
	if err != nil {
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(body))
		return common.OrderBook{}, fmt.Errorf("kucoin: failed to fetch order book, status %s", resp.Status)
	}

	var res kucoinOrderBookResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return common.OrderBook{}, err
	}

	orderBook := common.OrderBook{}
	for _, bid := range res.Data.Bids {
		price, _ := parseFloat(bid[0])
		qty, _ := parseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range res.Data.Asks {
		price, _ := parseFloat(ask[0])
		qty, _ := parseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       KUCOIN")
	}
	return orderBook, nil
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func (k *Kucoin) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	url := config.BaseUrl + "/api/v3/currencies"

	resp, err := k.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code string `json:"code"`
		Data []struct {
			Currency string `json:"currency"`
			Chains   []struct {
				ChainName          string `json:"chainName"`
				DepositStatus      bool   `json:"isDepositEnabled"`
				WithdrawStatus     bool   `json:"isWithdrawEnabled"`
				WithdrawFeeType    string `json:"withdrawFeeType"` // fixed / ratio / hybrid
				WithdrawFee        string `json:"withdrawFee"`
				DepositFee         string `json:"depositFee"`
				MinWithdrawFee     string `json:"minWithdrawFee"`
				MaxWithdrawFee     string `json:"maxWithdrawFee"`
				WithdrawMinFeeRate string `json:"withdrawMinFeeRate"`
				Precision          uint8  `json:"precision"`
			} `json:"chains"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if result.Code != "200000" {
		return nil, fmt.Errorf("kucoin API returned error code: %s", result.Code)
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)

	for _, currency := range result.Data {
		if _, exists := k.Symbols[currency.Currency]; !exists {
			continue
		}

		if chains[currency.Currency] == nil {
			chains[currency.Currency] = make(map[int]common.ChainInfo)
			k.Symbols[currency.Currency] = true
		}

		for _, c := range currency.Chains {
			chain_id, exists := all_chains.ChainId[c.ChainName]
			if !exists {
				fmt.Println("Unsupported chain: ", c.ChainName)
				continue
			}

			// Преобразуем строки в float64
			parse := func(s string) float64 {
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					return f
				}
				return 0
			}

			chains[currency.Currency][chain_id] = common.ChainInfo{
				Chain:                   c.ChainName,
				DepositEnabled:          c.DepositStatus,
				WithdrawEnabled:         c.WithdrawStatus,
				FeeWithdraw:             parse(c.WithdrawFee),
				FeeDeposit:              parse(c.DepositFee),
				MinFeeWithdraw:          parse(c.MinWithdrawFee),
				MaxFeeWithdraw:          parse(c.MaxWithdrawFee),
				TransactFeeRateWithdraw: 1 - parse(c.WithdrawMinFeeRate),
				Fetched:                 fetched,
				Accuracy:                c.Precision,
			}
		}
	}

	fmt.Println("KUCOIN 				Fetched All Chains: ")
	return chains, nil
}

type kucoinOrderBookResponse struct {
	Code string `json:"code"`
	Data struct {
		Bids [][]string `json:"bids"` // [price, quantity]
		Asks [][]string `json:"asks"`
	} `json:"data"`
}
