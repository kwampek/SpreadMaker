package bot

import (
	"fmt"

	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
)

func (b *Bot) PollCurrency(exchange_id int, symbol string) {
	//fmt.Println("Called PollCurrency ON ", exchanges.RExchanges[exchange_id])

	info_link, exists := b.OrderBooks[symbol]
	if !exists {
		fmt.Println("Error occured while call PollCurrency: ", symbol)
		return
	}

	chains_link, exists2 := b.Chains[symbol]
	if !exists2 {
		fmt.Println("No Chains Info for symbol: ", symbol)
		return
	}

	var info [exchanges.ExchangesCount]common.OrderBook
	var chains_info [exchanges.ExchangesCount]map[int]common.ChainInfo

	for i := 0; i < exchanges.ExchangesCount; i++ {
		if info_link[i].Load() == nil || chains_link[i].Load() == nil {
			continue
		}
		info[i] = *info_link[i].Load()
		chains_info[i] = *chains_link[i].Load()
	}

	users := *b.TgBot.CurrentUsers.Load()
	if len(users) == 0 {
		fmt.Println("No users")
	}

first_exchange_loop:
	for first_exchange := 0; first_exchange < exchanges.ExchangesCount; first_exchange++ {
		if len(info[first_exchange].Asks) == 0 {
			//fmt.Println("											no saved data asks (", symbol, ") for ", exchanges.RExchanges[first_exchange])
			continue
		}
		asks := info[first_exchange].Asks

	second_exchange_loop:
		for second_exchange := 0; second_exchange < exchanges.ExchangesCount; second_exchange++ {
			if (first_exchange != exchange_id && second_exchange != exchange_id) || second_exchange == first_exchange || len(info[second_exchange].Bids) == 0 { // Asks = empty <=> Bids = empty
				// if len(info[second_exchange].Bids) == 0 {
				// 	fmt.Println("											no saved data bids (", symbol, ") for ", exchanges.RExchanges[second_exchange])
				// }

				continue
			}

			bids := info[second_exchange].Bids

			for chaind_id, chain1 := range chains_info[first_exchange] {
				chain2, exists := chains_info[second_exchange][chaind_id]
				if !exists || !chain1.WithdrawEnabled || !chain2.DepositEnabled {
					continue
				}

				//fmt.Println("START MAIN PART")

				if chain1.WithdrawEnabled && chain2.WithdrawEnabled {
					current_buy_usdt := 0.0
					current_buy_coin := 0.0

					user_index := 0 // size must be >= 1
					if len(users) == 0 {
						fmt.Println("dermo")
					}
					next_user := users[user_index]
					next_user_amount := next_user.Balance

					ask_index := 0
					bid_index := 0

					current_sell_coin := 0.0
					current_sell_usdt := 0.0

					ask_bucket := asks[ask_index]
					bid_bucket := bids[bid_index]

				main_cycle:
					for current_buy_usdt < next_user_amount {
						ask_cost := ask_bucket.Price * ask_bucket.Quantity

						if current_buy_usdt+ask_cost < next_user_amount {
							current_buy_usdt += ask_cost
							current_buy_coin += ask_bucket.Quantity
							ask_index++
							if ask_index == len(asks) {
								// TODO warn another users
								continue first_exchange_loop
							}
							ask_bucket = asks[ask_index]
							continue
						}

						delta := (next_user_amount - current_buy_usdt) / ask_bucket.Price

						ask_bucket.Quantity -= delta

						current_buy_usdt = next_user_amount
						current_buy_coin += delta

						after_chain_current_buy_coin := (current_buy_coin-chain1.FeeWithdraw)*chain1.TransactFeeRateWithdraw - chain2.FeeDeposit

						// MAX MIN  manipulation

						for current_sell_coin < after_chain_current_buy_coin {
							bid_cost := bid_bucket.Price * bid_bucket.Quantity

							if current_sell_coin+bid_bucket.Quantity < after_chain_current_buy_coin {
								current_sell_coin += bid_bucket.Quantity
								current_sell_usdt += bid_cost
								bid_index++
								if bid_index == len(bids) {
									continue second_exchange_loop
								}
								bid_bucket = bids[bid_index]
								continue
							}

							d := (after_chain_current_buy_coin - current_sell_coin)
							bid_bucket.Quantity -= d

							current_sell_coin = after_chain_current_buy_coin
							current_sell_usdt += d * bid_bucket.Price

						}

						if current_sell_usdt >= current_buy_usdt {

							spread := common.Spread{
								UserID:    int64(next_user.UserId),
								Coin:      symbol,
								Exchange1: exchanges.RExchanges[first_exchange],
								Exchange2: exchanges.RExchanges[second_exchange],
								From:      current_buy_usdt,
								To:        current_sell_usdt,
								Chain:     chain1.Chain,
								Percent:   current_sell_usdt / current_buy_usdt,
								Spread:    current_sell_usdt - current_buy_usdt}

							fmt.Print("SPREAD: ", spread, "\n\n")
							b.TgBot.PublishSpread(spread)
						} else {
							fmt.Println(current_buy_usdt, current_sell_usdt, symbol)
						}

						for {
							user_index++
							if user_index == len(users) {
								break main_cycle
							}
							next_user = users[user_index]
							if next_user.Balance >= next_user_amount {
								break
							}
						}
						next_user_amount = next_user.Balance

					}

				}

			}

		}
	}

}

// func (b *Butch) StartPoll(stop <-chan bool, wg *sync.WaitGroup) {
// 	for _, symbol := range b.CoinsList {
// 		go func(symbol string) {
// 			defer wg.Done()
// 			for {
// 				select {
// 				default:
// 					//fmt.Println("loop cycle")
// 					startTime := time.Now()
// 					b.PollCurrency(symbol)
// 					remainingTime := 30*time.Second - time.Since(startTime)
// 					<-time.After(remainingTime)
// 				case <-stop:
// 					fmt.Println("Остановка")
// 					return
// 				}
// 			}
// 		}(symbol)
// 	}
// }

// func (b *Butch) CollectInfo(id int, symbol string) common.OrderBook {
// 	info, err := b.RExchanges[id].FetchOrderBook(symbol, "10")
// 	if err == nil {
// 		return info
// 	}
// 	return common.OrderBook{
// 		Bids: []common.Order{},
// 		Asks: []common.Order{},
// 	}
// }

// func equal_chains(chain1, chain2 string) bool {
// 	value1, ok1 := all_chains.ChainId[chain1]
// 	value2, ok2 := all_chains.ChainId[chain2]
// 	if ok1 && ok2 {
// 		return value1 == value2
// 	}
// 	chain1 = strings.ToLower(chain1)
// 	chain2 = strings.ToLower(chain2)
// 	if chain1 == chain2 {
// 		return true
// 	}
// 	if len(chain1) < len(chain2) {
// 		return strings.Contains(chain2, chain1)
// 	}
// 	return strings.Contains(chain1, chain2)
// }
