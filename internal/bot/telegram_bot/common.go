package telegrambot

import (
	"sort"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type User struct {
	Id              int64
	Login           string
	Password        string
	UserID          int64
	ChatID          int64
	Balance         float64
	BannedCoins     map[string]struct{}
	BannedExchanges map[string]struct{}
	LastMessages    map[string]Message
}

type Message struct {
	Symbol    string
	Chain     string
	MessageID int
	UserID    int64
	ChatID    int64
	Timestamp time.Time
}

type UserBalance struct {
	Balance float64
	UserId  int
}

type ByBalance []UserBalance

func (a ByBalance) Len() int           { return len(a) }
func (a ByBalance) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByBalance) Less(i, j int) bool { return a[i].Balance < a[j].Balance }

type TelegramBot struct {
	mu             sync.RWMutex
	CurrentUsers   []UserBalance
	NewUsers       []UserBalance
	Users          map[string]*User
	AuthSessions   map[int64]*PendingAuth
	Bot            *tgbotapi.BotAPI
	FilterSessions map[int64]*PendingFilterEdit // по chat_id
}

func (tb *TelegramBot) AddNewUserBalance(balance float64, userid int) {
	// No Lock Need
	tb.CurrentUsers = append(tb.CurrentUsers, UserBalance{Balance: balance, UserId: userid})
}

func (tb *TelegramBot) GetAllUsers() []UserBalance {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.CurrentUsers = append(tb.CurrentUsers, tb.NewUsers...)
	sort.Sort(ByBalance(tb.CurrentUsers)) // OPRIMIZE ME
	tb.NewUsers = nil

	return tb.CurrentUsers
}
