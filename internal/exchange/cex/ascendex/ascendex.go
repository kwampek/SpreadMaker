package ascendex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kwampek/spreadmaker2/cmd/mydebug"
	"github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/all_chains"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

var config *Config = Load()

type AscendEX struct {
	exchanges.Exchange
	Client          *http.Client
	OrderBookClient *util.Client
}

func New() *AscendEX {
	return &AscendEX{
		Exchange:        exchanges.Exchange{},
		Client:          &http.Client{},          //Timeout: 15 * time.Second},
		OrderBookClient: util.NewClient(20.0, 1), //Timeout: 15 * time.Second},
		//18.0
	}
}

func (ae *AscendEX) GetSymbols() map[string]bool {
	return ae.Symbols
}

func (ae *AscendEX) SetSymbols(symbols map[string]bool) {
	ae.Symbols = symbols
}

func (ae *AscendEX) GetClient() *util.Client {
	return ae.OrderBookClient
}

func (ae *AscendEX) Name() string {
	return "ASCENDEX"
}

func (ae *AscendEX) Id() int {
	return exchanges.RExchangesId["ASCENDEX"]
}

type SpotPair struct {
	Symbol     string `json:"symbol"`     // Полный символ, например: "BTC/USDT"
	QuoteAsset string `json:"domain"`     // Котируемая валюта, например: "USDT"
	Status     string `json:"statusCode"` // Статус пары (например: "Normal")
}

type SpotPairResponse struct {
	Code int        `json:"code"`
	Data []SpotPair `json:"data"`
}

func (ae *AscendEX) FetchAllCoins(all_coins map[string]int) error {
	url := "https://ascendex.com/api/pro/v1/cash/products" // Эндпоинт для спотового рынка

	//resp, err := ae.Client.Get(url)
	resp, err := util.SafeOpenPage(ae.Client, url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result SpotPairResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, pair := range result.Data {
		if pair.QuoteAsset == "USDS" && pair.Status == "Normal" {
			tmp := len(pair.Symbol) - 5
			if tmp < 0 {
				continue
			}
			all_coins[pair.Symbol[:tmp]]++
		}
	}

	return nil
}

func (ae *AscendEX) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	//resp, err := ae.Client.Get("https://ascendex.com/api/pro/v2/assets")
	resp, err := util.SafeOpenPage(ae.Client, "https://ascendex.com/api/pro/v2/assets")

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Code int `json:"code"`
		Data []struct {
			AssetCode string `json:"assetCode"`
			Chains    []struct {
				ChainName       string  `json:"chainName"` // пример: "ETH"
				DepositEnabled  bool    `json:"allowDeposit"`
				WithdrawEnabled bool    `json:"allowWithdraw"`
				WithdrawFee     float64 `json:"withdrawFee,string"`
				MinWithdrawal   float64 `json:"minWithdrawal,string"`
			} `json:"blockChain"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("ascendex API error code: %d", result.Code)
	}

	fetched := time.Now()

	chains := make(map[string]map[int]common.ChainInfo)

	for _, asset := range result.Data {
		coin := asset.AssetCode
		if _, exists := ae.Symbols[coin]; !exists {
			continue
		}

		if chains[coin] == nil {
			chains[coin] = make(map[int]common.ChainInfo)
			ae.Symbols[coin] = true
		}

		for _, ch := range asset.Chains {
			base := strings.ToUpper(ch.ChainName)
			if id, ok := all_chains.ChainId[base]; ok {
				chains[coin][id] = common.ChainInfo{
					Chain:           base,
					DepositEnabled:  ch.DepositEnabled,
					WithdrawEnabled: ch.WithdrawEnabled,
					FeeWithdraw:     ch.WithdrawFee,
					MinFeeWithdraw:  ch.MinWithdrawal,
					Fetched:         fetched,
				}
			} else {
				fmt.Println("ASCENDEX 		UnsupportedChain: ", base)
			}
		}
	}

	fmt.Println("ASCENDEX 				Fetched All Chains: ")
	return chains, nil
}

// 🔹 Получение ордербука через REST
func (ae *AscendEX) FetchOrderBook(symbol string, _ string) (common.OrderBook, error) {
	symbol += config.Separator + common.BaseCurrency
	resp, err := ae.OrderBookClient.Get(fmt.Sprintf("https://ascendex.com/api/pro/v1/depth?symbol=%s", strings.ToUpper(symbol)))
	if err != nil {
		return common.OrderBook{}, err
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
		Data struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	orderBook := common.OrderBook{}
	for _, bid := range result.Data.Bids {
		price, _ := util.ParseFloat(bid[0])
		qty, _ := util.ParseFloat(bid[1])
		orderBook.Bids = append(orderBook.Bids, common.Order{Price: price, Quantity: qty})
	}
	for _, ask := range result.Data.Asks {
		price, _ := util.ParseFloat(ask[0])
		qty, _ := util.ParseFloat(ask[1])
		orderBook.Asks = append(orderBook.Asks, common.Order{Price: price, Quantity: qty})
	}
	if mydebug.DebugOrderBook {
		fmt.Println("SUCC FETCHED ORDERBOOK      (", symbol, ")                       ASCENDEX")
	}
	return orderBook, nil
}

// // Формируем подпись для REST-запроса
// func (ae *AscendEX) signedGET(path string, params map[string]string) ([]byte, error) {
// 	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
// 	params["timestamp"] = ts

// 	keys := make([]string, 0, len(params))
// 	for k := range params {
// 		keys = append(keys, k)
// 	}
// 	sort.Strings(keys)

// 	vals := url.Values{}
// 	var parts []string
// 	for _, k := range keys {
// 		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
// 		vals.Set(k, params[k])
// 	}
// 	prehash := strings.Join(parts, "&")

// 	mac := hmac.New(sha256.New, []byte(config.SECRET))
// 	mac.Write([]byte(prehash))
// 	sig := hex.EncodeToString(mac.Sum(nil))
// 	vals.Set("signature", sig)

// 	fullURL := fmt.Sprintf("https://ascendex.com/api/pro/v1%s?%s", path, vals.Encode())
// 	req, _ := http.NewRequest("GET", fullURL, nil)
// 	req.Header.Set("x-auth-key", config.API)
// 	req.Header.Set("x-auth-signature", sig)
// 	req.Header.Set("x-auth-timestamp", ts)

// 	resp, err := http.DefaultClient.Do(req)
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
