package bot

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kwampek/spreadmaker2/internal/bot/tbot"
	telegrambot "github.com/kwampek/spreadmaker2/internal/bot/telegram_bot"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/ascendex"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/binance"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/bitget"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/bybit"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/coinex"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/kucoin"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/mexc"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/okx"
	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/cmd"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

const (
	AdminLogin    = "admin"
	AdminPassword = "admin"

	DBPath   = "../internal/bot/tbot/base.db"
	BotToken = telegrambot.BotToken
)

type Bot struct {
	Exchanges  []exchanges.IExchange
	OrderBooks map[string][]atomic.Pointer[common.OrderBook]
	Chains     map[string][]atomic.Pointer[map[int]common.ChainInfo]
	Sync       cmd.BotSync

	TgBot *tbot.TelegramBot
}

func NewExchange(name string) exchanges.IExchange {
	switch name {
	case "BYBIT":
		return bybit.New()
	case "KUCOIN":
		return kucoin.New()
	case "MEXC":
		return mexc.New()
	case "BINANCE":
		return binance.New()
	case "OKX":
		return okx.New()
	case "BITGET":
		return bitget.New()
	case "COINEX":
		return coinex.New()
	case "ASCENDEX":
		return ascendex.New()
	default:
		return &exchanges.Exchange{}
	}
}

func NewBot() *Bot {
	var bot Bot

	bt, err := tbot.NewTelegramBot(BotToken, DBPath)
	if err != nil {
		fmt.Println("Error: ", err)
	}

	bot.TgBot = bt

	bot.Exchanges = make([]exchanges.IExchange, exchanges.ExchangesCount)
	bot.OrderBooks = make(map[string][]atomic.Pointer[common.OrderBook])
	bot.Chains = make(map[string][]atomic.Pointer[map[int]common.ChainInfo])

	for i := range exchanges.ExchangesCount {
		fmt.Println("New exchange ", exchanges.RExchanges[i])
		bot.Exchanges[i] = NewExchange(exchanges.RExchanges[i])
	}
	return &bot
}

func (b *Bot) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ловим SIGINT (Ctrl+C) и SIGTERM (docker stop / kill)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("Shutdown signal received...")
		cancel()
	}()

	// запускаем телеграм-бота
	go b.TgBot.StartTelegramBot(ctx)

	// собираем инфу о цепочках
	b.CollectChainsInfo()

	// канал обновлений
	ch := make(chan *common.OrderBookUpdate, 10000)

	// консюмер
	go b.ConsumeUpdates(ch)

	// фиды бирж
	for _, ex := range b.Exchanges {
		go b.StartOrderBookFeed(ex, ch)
	}

	// ждём завершения контекста
	<-ctx.Done()

	b.Sync.Status.Store(cmd.StatusPaused)
	log.Println("Bot stopped gracefully")
}

/*
func (b *Bot) Start() {
	go b.TgBot.StartTelegramBot()
	b.Sync.StartRoutine()
	b.CollectChainsInfo()

	ch := make(chan *common.OrderBookUpdate, 10000)

	go b.ConsumeUpdates(ch)

	for _, ex := range b.Exchanges {
		go b.StartOrderBookFeed(ex, ch)
	}

	for b.Sync.Status.Load() == cmd.StatusOk {
	}
	for b.Sync.LoadLevel.Load() != 0 {
	}

}
*/

func (b *Bot) Stop() {
	b.Sync.Status.Store(cmd.StatusPaused)
	for b.Sync.LoadLevel.Load() != 0 {
	}
}

func (b *Bot) clearInfo() {
	b.OrderBooks = make(map[string][]atomic.Pointer[common.OrderBook])
	b.Chains = make(map[string][]atomic.Pointer[map[int]common.ChainInfo])
}

func (b *Bot) CollectChainsInfo() {
	all_chains := make([]map[string]map[int]common.ChainInfo, exchanges.ExchangesCount)
	all_coins := make(map[string]int, 1000)

	succ := make([]bool, exchanges.ExchangesCount)

	wg := sync.WaitGroup{}
	wg.Add(len(b.Exchanges))
	for id, ex := range b.Exchanges {
		go func() {
			defer wg.Done()
			fmt.Println(id, ex.Name())
			err := ex.FetchAllCoins(all_coins)
			if err == nil {
				succ[id] = true
			}
		}()
	}
	wg.Wait()
	b.InitSymbolsForAllExchanges(all_coins)

	for _, ex := range b.Exchanges {
		fmt.Println(ex.Name(), ex.GetSymbols())
	}

	wg.Add(len(b.Exchanges))
	for id, ex := range b.Exchanges {
		go func() {
			defer wg.Done()
			chains, err := ex.FetchAllChains()

			if err != nil {
				fmt.Println("Error occured while fetching chains: ", err)
				return
			}
			all_chains[id] = chains
		}()
	}

	wg.Wait()

	for id := range exchanges.ExchangesCount {
		for coin, chs := range all_chains[id] {
			if b.Chains[coin] == nil {
				b.Chains[coin] = make([]atomic.Pointer[map[int]common.ChainInfo], exchanges.ExchangesCount)
				b.OrderBooks[coin] = make([]atomic.Pointer[common.OrderBook], exchanges.ExchangesCount)
			}
			b.Chains[coin][id].Store(&chs)
		}
	}
	fmt.Println("That ALL")
}

func (b *Bot) CollectCoinsInfo() {

	all_coins := make(map[string]int)
	wg := sync.WaitGroup{}
	wg.Add(len(b.Exchanges))
	for _, ex := range b.Exchanges {
		go func() {
			defer wg.Done()
			err := ex.FetchAllCoins(all_coins)
			if err != nil {
				fmt.Println("Error occured while fetching coins: ", err, " ON ", ex.Name())
				return
			}
		}()
	}
	wg.Wait()

	for _, ex := range b.Exchanges {
		fmt.Println(ex.Name(), ": ", util.GetMapKeys(ex.GetSymbols())[:20])
	}
}

func (b *Bot) ConsumeUpdates(ch <-chan *common.OrderBookUpdate) {
	for update := range ch {
		if update == nil {
			continue
		}

		if update.Exchange == exchanges.StopExchange {
			fmt.Println("HARAM						  Received Stop message")
			return
		}

		b.OrderBooks[update.Symbol][update.Exchange].Store(update.OrderBook)
		go b.PollCurrency(update.Exchange, update.Symbol)
	}
}

func (b *Bot) InitSymbolsForAllExchanges(allCoins map[string]int) {
	all_coins := make(map[string]bool, len(allCoins))
	for coin, supported := range allCoins {
		if supported > 1 {
			all_coins[coin] = exchanges.UNCONFIRMED_COIN
		}
		//                                                                                 CORRECT HERE
	}

	for _, exchange := range b.Exchanges {
		exchange.SetSymbols(maps.Clone(all_coins))
	}
}

func (b *Bot) StartOrderBookFeed(e exchanges.IExchange, updates chan<- *common.OrderBookUpdate) {
	fmt.Println("STARTORDERBOOKFEED: ", e.Name(), len(e.GetSymbols()))

	var all_active_symbols []string
	for s, active := range e.GetSymbols() {
		if active {
			all_active_symbols = append(all_active_symbols, s)
		}
	}

	for {
		for _, s := range all_active_symbols {
			// Запрос к API
			ob, err := e.FetchOrderBook(s, "20")
			if err != nil {
				fmt.Println("Failed to fetch order book:", err, ", ON: ", e.Name(), " SYMBOL: ", s)
				time.Sleep(time.Second)
				continue
			}

			if b.Sync.Status.Load() == cmd.StatusPaused {
				updates <- &common.OrderBookUpdate{
					Exchange: exchanges.StopExchange,
				}
				fmt.Println("HARAM 										StopExchangeSended for ", e.Name())
				return
			}

			// Отправка обновления
			updates <- &common.OrderBookUpdate{
				Symbol:    s,
				Exchange:  e.Id(),
				OrderBook: &ob,
			}
		}
	}
}

/*
import (
	"net/http"
	"sync"
	"time"

	"github.com/kwampek/spreadmaker2/internal/exchange/cex/ascendex"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/binance"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/bitget"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/bybit"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/coinex"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/kucoin"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/mexc"
	"github.com/kwampek/spreadmaker2/internal/exchange/cex/okx"
	common "github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	"github.com/kwampek/spreadmaker2/internal/util"
)

type Bot struct {
	Butches     []Butch
	ChainsTable Chains
	TgBot       TelegramBot
	// IsActive atomic.Bool
}

type Butch struct {
	RExchanges []exchanges.RExchange
	HttpClient http.Client
	Info       [exchanges.ExchangesCount]common.OrderBook
	CoinsList  []string
	Host       *Bot
	// IsActivePtr *atomic.Bool
}

func NewBot() *Bot {
	var bot Bot

	bot.ChainsTable = New()

	butches_count := (len(bot.ChainsTable.AllSymbols) + CoinsInButch - 1) / CoinsInButch
	lcoins := util.GetMapKeys(bot.ChainsTable.AllSymbols)

	var butches = make([]Butch, butches_count)

	lboard := 0
	for i := range butches_count {
		var rexchanges = make([]exchanges.RExchange, exchanges.RExchangesCount)

		butches[i] = Butch{
			RExchanges: rexchanges,
			HttpClient: http.Client{
				Timeout: 10 * time.Second,
			},
			CoinsList: lcoins[lboard:min(lboard+CoinsInButch, len(lcoins))],
			Host:      &bot,
		}
		client := &butches[i].HttpClient

		lboard += CoinsInButch

		for j := range exchanges.RExchangesCount {
			rexchanges[j] = NewR(exchanges.RExchanges[j], client)
		}
	}
	bot.Butches = butches
	bot.TgBot = *NewTelegramBot()
	return &bot
}

func (b *Bot) Start(stop <-chan bool) error {
	wg := sync.WaitGroup{}
	wg.Add(len(b.Butches))
	for id, butch := range b.Butches {
		butch.StartPoll(stop, &wg)
		if (id+1)%exchanges.ButchCount == 0 {
			<-time.After(2 * time.Second)
		}
	}
	wg.Wait()
	return nil
}

func NewR(name string, client *http.Client) exchanges.RExchange {
	switch name {
	case "BYBIT":
		return bybit.New(client)
	case "KUCOIN":
		return kucoin.New(client)
	case "MEXC":
		return mexc.New(client)
	case "BINANCE":
		return binance.New(client)
	case "OKX":
		return okx.New(client)
	case "BITGET":
		return bitget.New(client)
	case "COINEX":
		return coinex.New(client)
	case "ASCENDEX":
		return ascendex.New(client)
	default:
		return exchanges.DefaultRExchange{}
	}
}

// type WSExchange interface {
// 	Name() string
// 	Connect()
// }
*/
