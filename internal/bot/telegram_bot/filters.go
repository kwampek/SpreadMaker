package telegrambot

import (
	"database/sql"
	"strings"
)

type FilterState int

const (
	FilterIdle FilterState = iota
	FilterChoosingType
	FilterCoinMenu
	FilterExchangeMenu
	FilterAddingCoin
	FilterRemovingCoin
	FilterAddingExchange
	FilterRemovingExchange
)

type PendingFilterEdit struct {
	State  FilterState
	Login  string
	ChatID int64
	UserID int64
}

func (tb *TelegramBot) StartFilterEdit(chatID int64, user *User) {
	tb.mu.Lock()
	tb.FilterSessions[chatID] = &PendingFilterEdit{
		State:  FilterChoosingType,
		Login:  user.Login,
		ChatID: chatID,
		UserID: user.UserID,
	}
	tb.mu.Unlock()

	tb.sendMessage(chatID, "Выберите, что хотите отфильтровать:", []string{
		"🚫 Монеты", "🚫 Биржи", "🔙 Назад",
	})
}

func (tb *TelegramBot) HandleFilterInput(chatID int64, input string) {
	tb.mu.RLock()
	session, ok := tb.FilterSessions[chatID]
	tb.mu.RUnlock()

	if !ok {
		tb.sendMessage(chatID, "Введите /filters для редактирования фильтров.", nil)
		return
	}

	switch session.State {
	case FilterChoosingType:
		switch input {
		case "🚫 Монеты":
			session.State = FilterCoinMenu
			tb.sendMessage(chatID, "Что вы хотите сделать с монетами?", []string{
				"➕ Добавить монету", "➖ Удалить монету", "📋 Список", "🔙 Назад",
			})
		case "🚫 Биржи":
			session.State = FilterExchangeMenu
			tb.sendMessage(chatID, "Что вы хотите сделать с биржами?", []string{
				"➕ Добавить биржу", "➖ Удалить биржу", "📋 Список", "🔙 Назад",
			})
		case "🔙 Назад":
			tb.mu.Lock()
			delete(tb.FilterSessions, chatID)
			tb.mu.Unlock()
			tb.sendMessage(chatID, "Вы вышли из меню фильтров.", nil)
		}

	case FilterCoinMenu:
		switch input {
		case "➕ Добавить монету":
			session.State = FilterAddingCoin
			tb.sendMessage(chatID, "Введите название монеты для блокировки:", []string{"🔙 Назад"})
		case "➖ Удалить монету":
			session.State = FilterRemovingCoin
			tb.sendMessage(chatID, "Введите монету, которую хотите разблокировать:", []string{"🔙 Назад"})
		case "📋 Список":
			user := tb.Users[session.Login]
			var coins []string
			for coin := range user.BannedCoins {
				coins = append(coins, coin)
			}
			if len(coins) == 0 {
				tb.sendMessage(chatID, "Список заблокированных монет пуст.", nil)
			} else {
				tb.sendMessage(chatID, "Заблокированные монеты:\n"+strings.Join(coins, ", "), nil)
			}
		case "🔙 Назад":
			session.State = FilterChoosingType
			tb.sendMessage(chatID, "Выберите, что хотите отфильтровать:", []string{"🚫 Монеты", "🚫 Биржи", "🔙 Назад"})
		}

	case FilterAddingCoin:
		if input == "🔙 Назад" {
			session.State = FilterCoinMenu
			tb.sendMessage(chatID, "Что вы хотите сделать с монетами?", []string{
				"➕ Добавить монету", "➖ Удалить монету", "📋 Список", "🔙 Назад",
			})
			return
		}
		tb.AddBannedCoin(session.Login, input)
		tb.sendMessage(chatID, "✅ Монета '"+input+"' добавлена в фильтр.", nil)
		session.State = FilterCoinMenu

	case FilterRemovingCoin:
		if input == "🔙 Назад" {
			session.State = FilterCoinMenu
			tb.sendMessage(chatID, "Что вы хотите сделать с монетами?", []string{
				"➕ Добавить монету", "➖ Удалить монету", "📋 Список", "🔙 Назад",
			})
			return
		}
		tb.RemoveBannedCoin(session.Login, input)
		tb.sendMessage(chatID, "✅ Монета '"+input+"' удалена из фильтра.", nil)
		session.State = FilterCoinMenu

		// Аналогично реализуется FilterExchangeMenu, FilterAddingExchange, FilterRemovingExchange
	}
}

func (tb *TelegramBot) AddBannedCoin(login, coin string) {
	db, _ := sql.Open("sqlite3", DBPath)
	defer db.Close()
	db.Exec(`INSERT OR IGNORE INTO banned_coins (login, coin) VALUES (?, ?)`, login, coin)

	tb.mu.Lock()
	if user, ok := tb.Users[login]; ok {
		user.BannedCoins[coin] = struct{}{}
	}
	tb.mu.Unlock()
}

func (tb *TelegramBot) RemoveBannedCoin(login, coin string) {
	db, _ := sql.Open("sqlite3", DBPath)
	defer db.Close()
	db.Exec(`DELETE FROM banned_coins WHERE login = ? AND coin = ?`, login, coin)

	tb.mu.Lock()
	if user, ok := tb.Users[login]; ok {
		delete(user.BannedCoins, coin)
	}
	tb.mu.Unlock()
}
