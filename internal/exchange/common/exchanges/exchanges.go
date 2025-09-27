package exchanges

import (
	"fmt"

	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"golang.org/x/time/rate"
)

const (
	ExchangesCount  = 6 // TODO
	RExchangesCount = ExchangesCount
	ButchCount      = 5

	UNCONFIRMED_COIN = false
	CONFIRMED_COIN   = true

	StopExchange = 666
)

var (
	RExchanges = []string{
		"BINANCE",
		"COINEX",
		"ASCENDEX",
		"MEXC",
		"KUCOIN",
		"BITGET"}
	RExchangesId = GetIdRExchanges()
)

/*
const (
	ExchangesCount  = 6 // TODO
	RExchangesCount = 6
)

var (
	RExchanges = []string{
		"BYBIT",
		"KUCOIN",
		"MEXC",
		"BINANCE",
		"OKX",
		"HUOBI"}
	RExchangesId = GetIdRExchanges()
)
*/

func GetIdRExchanges() map[string]int {
	mp := make(map[string]int)
	for i, w := range RExchanges {
		mp[w] = i
	}
	return mp
}

type Exchange struct {
	Symbols map[string]bool
}

func (e *Exchange) Name() string {
	return "NULL"
}

func (e *Exchange) Id() int {
	return 0
}

func (e *Exchange) FetchAllCoins(map[string]int) error {
	return fmt.Errorf("uninitialized exchange")
}

func (e *Exchange) FetchOrderBook(string, string) (common.OrderBook, error) {
	return common.OrderBook{
		Bids: []common.Order{},
		Asks: []common.Order{},
	}, fmt.Errorf("Z")
}

func (e *Exchange) FetchAllChains() (map[string]map[int]common.ChainInfo, error) {
	return nil, nil
}

func (e *Exchange) GetSymbols() map[string]bool {
	return nil
}

func (e *Exchange) SetSymbols(map[string]bool) {
}

func (e *Exchange) GetLimiter() *rate.Limiter {
	return nil
}

func (e *Exchange) GetLink(string) string {
	return ""
}

type IExchange interface {
	FetchAllChains() (map[string]map[int]common.ChainInfo, error)
	FetchAllCoins(map[string]int) error
	FetchOrderBook(string, string) (common.OrderBook, error)
	GetSymbols() map[string]bool
	SetSymbols(map[string]bool)
	Name() string
	Id() int
}

/*
	RExchanges = []string{
		"PANCAKE",
		"UNISWAP"}

	WSExchanges = []string{
		"BYBIT",
		"KUCOIN",
		"MEXC",
		"BINANCE",
		"OKX",
		"HUOBI"}
*/
