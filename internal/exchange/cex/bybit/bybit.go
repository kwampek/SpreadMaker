package bybit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

type Bybit struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *Bybit {
	return &Bybit{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},           //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(100.0, 1), //Timeout: 15 * time.Second},
	}
}

func (b *Bybit) SetSymbols(symbols map[string]bool) {
	b.Symbols = symbols
}

func (b *Bybit) FetchAllCoins(all_coins map[string]int) error {
	url := "https://api.bybit.com/v5/market/instruments-info?category=spot"

	resp, err := b.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Структура для парсинга ответа
	var result struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			List []struct {
				Symbol    string `json:"symbol"`
				BaseCoin  string `json:"baseCoin"`
				QuoteCoin string `json:"quoteCoin"`
				Status    string `json:"status"`
			} `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Result.List {
		if pair.QuoteCoin == "USDT" && pair.Status == "Trading" {
			all_coins[pair.BaseCoin]++
		}
	}

	return nil
}

func (b *Bybit) GetSymbols() map[string]bool {
	return b.Symbols
}

func (b *Bybit) Name() string {
	return "BYBIT"
}

func (b *Bybit) Id() int {
	return exchanges.RExchangesId["BYBIT"]
}

func (b *Bybit) FetchOrderBook(symbol, limit string) (common.OrderBook, error) {
	symbol = symbol + config.Separator + common.BaseCurrency

	fmt.Println("Called Fetch Order Book ", symbol, " ON BYBIT")

	endpoint := fmt.Sprintf("%s/v5/market/orderbook?category=spot&symbol=%s&limit=%s", config.BaseUrl, symbol, limit)

	resp, err := b.OrderBookClient.Get(endpoint)
	if err != nil {
		fmt.Println("Get problem ORDERBOOK (", symbol, ") ON BYBIT")
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Println("Status problem (", symbol, ") ON BYBIT")
		return common.OrderBook{}, fmt.Errorf("bybit: failed to fetch order book, status %s", resp.Status)
	}

	var res bybitOrderBookResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Println("Decode problem (", symbol, ") ON BYBIT")
		return common.OrderBook{}, err
	}

	orderBook := common.OrderBook{}
	for _, bid := range res.Result.Bids {
		price, _ := util.ParseFloat(bid[0])
		qty, _ := util.ParseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range res.Result.Asks {
		price, _ := util.ParseFloat(ask[0])
		qty, _ := util.ParseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       BYBIT")
	}
	return orderBook, nil
}

func (b *Bybit) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	timestamp := strconv.FormatInt(time.Now().UnixNano()/1000000, 10)
	body := ""

	// Строка для подписи: timestamp + apiKey + recvWindow + body
	signPayload := timestamp + config.API + config.RecvWindow + body
	h := hmac.New(sha256.New, []byte(config.SECRET))
	h.Write([]byte(signPayload))
	sign := hex.EncodeToString(h.Sum(nil))

	req, err := http.NewRequest("GET", config.BaseUrl+config.ChainEndpoint, nil)
	if err != nil {
		return nil, err
	}

	// Добавляем заголовки авторизации
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BAPI-API-KEY", config.API)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", config.RecvWindow)
	req.Header.Set("X-BAPI-SIGN", sign)
	req.Header.Set("X-BAPI-SIGN-TYPE", "2")

	resp, err := b.Client.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Разбор JSON
	var apiResp struct {
		Result struct {
			Rows []struct {
				Coin   string `json:"coin"`
				Chains []struct {
					Chain          string `json:"chain"`
					WithdrawFee    string `json:"withdrawFee"`
					DepositMin     string `json:"depositMin"`
					WithdrawMin    string `json:"withdrawMin"`
					WithdrawMax    string `json:"withdrawMax"`
					DepositEnable  string `json:"chainDeposit"`
					WithdrawEnable string `json:"chainWithdraw"`
					Accuracy       string `json:"minAccuracy"`
				} `json:"chains"`
			} `json:"rows"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		fmt.Println("Error while decoding chain message bybit: ", err)
		return nil, err
	}

	fetched := time.Now()
	chains := make(map[string]map[int]common.ChainInfo)
	fmt.Println("LEN: ", len(apiResp.Result.Rows))
	for _, row := range apiResp.Result.Rows {
		if _, exists := b.Symbols[row.Coin]; !exists {
			fmt.Println("NO COIN: ", row.Coin)
			continue
		}

		if chains[row.Coin] == nil {
			chains[row.Coin] = make(map[int]common.ChainInfo, exchanges.RExchangesCount)
			b.Symbols[row.Coin] = true
		}

		for _, ch := range row.Chains {

			chain_id, exists := all_chains.ChainId[ch.Chain]
			if !exists {
				fmt.Println("Unsupported chain: ", ch.Chain)
				continue
			}

			withdrawFee, _ := util.ParseFloat(ch.WithdrawFee)
			withdrawMin, _ := util.ParseFloat(ch.WithdrawMin)
			withdrawMax, _ := util.ParseFloat(ch.WithdrawMax)
			accuracy, _ := strconv.ParseUint(ch.Accuracy, 10, 8)

			chains[row.Coin][chain_id] = common.ChainInfo{
				Chain:                   ch.Chain,
				DepositEnabled:          ch.DepositEnable == "1",
				WithdrawEnabled:         ch.WithdrawEnable == "1",
				FeeWithdraw:             withdrawFee,
				MinFeeWithdraw:          withdrawMin,
				MaxFeeWithdraw:          withdrawMax,
				TransactFeeRateWithdraw: 1.0,
				Fetched:                 fetched,
				Accuracy:                uint8(accuracy),
			}
		}
	}

	fmt.Println("BYBIT 				Fetched All Chains: ")
	return chains, nil
}

// --------------------
// BYBIT API RESPONSES
// --------------------

type bybitOrderBookResponse struct {
	RetCode int `json:"retCode"`
	Result  struct {
		Bids [][]string `json:"b"` // [price, quantity]
		Asks [][]string `json:"a"`
	} `json:"result"`
}
