package common

import "time"

const BaseCurrency = "USDT"

type Order struct {
	Price    float64
	Quantity float64
}

type OrderBook struct {
	Bids []Order
	Asks []Order
}

type OrderBookUpdate struct {
	Symbol    string
	OrderBook *OrderBook
	Exchange  int
}

type ChainInfo struct {
	Chain                   string
	FeeWithdraw             float64
	FeeDeposit              float64
	MinFeeWithdraw          float64
	MaxFeeWithdraw          float64
	TransactFeeRateWithdraw float64
	Fetched                 time.Time
	Accuracy                uint8
	DepositEnabled          bool
	WithdrawEnabled         bool
}

type Spread struct {
	UserID    int64
	Coin      string
	Exchange1 string
	Exchange2 string
	From      float64
	To        float64
	Percent   float64
	Chain     string
	Spread    float64
}
