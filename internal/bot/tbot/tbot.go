package tbot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kwampek/spreadmaker2/internal/exchange/common"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
	_ "github.com/mattn/go-sqlite3"
)

type UserBalance struct {
	Balance float64
	UserId  int64
}

const (
	stateNone = iota
	stateRegLogin
	stateRegPassword
	stateLoginLogin
	stateLoginPassword
	stateCoinBan
	statePercentFilter
	stateBalanceInput
)

type TelegramBot struct {
	bot        *tgbotapi.BotAPI
	db         *sql.DB
	sessions   map[int64]string // chatID -> login
	userStates map[int64]int
	tempData   map[int64]string

	// сохраняем id последнего отправленного/редактируемого сообщения фильтров для каждого чата
	lastFilterMessage map[int64]int // chatID -> messageID

	CurrentUsers atomic.Pointer[[]UserBalance]
	NewUsers     []UserBalance

	SpreadChan chan common.Spread
}

func NewTelegramBot(token, dbPath string) (*TelegramBot, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе: %w", err)
	}
	if err := initDB(db); err != nil {
		db.Close()
		return nil, err
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}
	tb := &TelegramBot{
		bot:               bot,
		db:                db,
		sessions:          make(map[int64]string),
		userStates:        make(map[int64]int),
		tempData:          make(map[int64]string),
		lastFilterMessage: make(map[int64]int),
		NewUsers:          make([]UserBalance, 0),
		SpreadChan:        make(chan common.Spread, 1000),
	}
	users := make([]UserBalance, 0)
	tb.CurrentUsers.Store(&users)

	return tb, nil
}

func initDB(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			user_id INTEGER DEFAULT 0,
			chat_id INTEGER DEFAULT 0,
			login TEXT UNIQUE,
			password TEXT,
			balance REAL DEFAULT 100.0,
			plan TEXT,
			min_percent REAL DEFAULT 1.0,
			paused INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS banned_coins (
			login TEXT,
			coin TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS banned_exchanges (
			login TEXT,
			exchange TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS messages_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			coin TEXT,
			balance REAL,
			spread REAL,
			message_id INTEGER,
			user_id INTEGER,
			chat_id INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("ошибка инициализации БД: %w", err)
		}
	}
	return nil
}

func (tb *TelegramBot) LoadCurrentUsers() error {
	rows, err := tb.db.Query(`SELECT user_id, balance FROM users`)
	if err != nil {
		return fmt.Errorf("не удалось прочитать пользователей из БД: %w", err)
	}
	defer rows.Close()

	users := make([]UserBalance, 0)
	for rows.Next() {
		var userID int64
		var balance float64
		if err := rows.Scan(&userID, &balance); err != nil {
			return fmt.Errorf("ошибка чтения строки пользователей: %w", err)
		}
		users = append(users, UserBalance{
			UserId:  userID,
			Balance: balance,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ошибка итерации по строкам пользователей: %w", err)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Balance < users[j].Balance
	})
	tb.CurrentUsers.Store(&users) // Here

	return nil
}

func (tb *TelegramBot) AddNewUser(ub UserBalance) {
	tb.NewUsers = append(tb.NewUsers, ub)
}

func (tb *TelegramBot) UpdateUsers() {
	users := *tb.CurrentUsers.Load()
	users = append(users, tb.NewUsers...)
	sort.Slice(users, func(i, j int) bool {
		return users[i].Balance < users[j].Balance
	})
	tb.NewUsers = tb.NewUsers[:0]
	tb.CurrentUsers.Store(&users)
}

func (tb *TelegramBot) NextCurrentUser(index *int) (UserBalance, bool) {
	users := *tb.CurrentUsers.Load()
	if *index >= len(users) {
		return UserBalance{}, false
	}
	ub := users[*index]
	*index++
	return ub, true
}

func (tb *TelegramBot) StartTelegramBot(ctx context.Context) {
	log.Println("Start Telegram Bot")
	if err := tb.LoadCurrentUsers(); err != nil {
		log.Printf("LoadCurrentUsers warning: %v", err)
	}

	// start background goroutine with context
	go tb.handleSpreadMessages(ctx)

	updates := tb.bot.GetUpdatesChan(tgbotapi.NewUpdate(0))

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping Telegram Bot loop")
			return
		case upd, ok := <-updates:
			if !ok {
				log.Println("Updates channel closed")
				return
			}
			if upd.CallbackQuery != nil {
				tb.handleCallback(upd)
			} else if upd.Message != nil {
				tb.handleMessage(upd)
			}
		}
	}
}

func (tb *TelegramBot) handleSpreadMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// context canceled, exit goroutine
			log.Println("handleSpreadMessages stopped")
			return
		case sp, ok := <-tb.SpreadChan:
			if !ok {
				// channel closed, exit goroutine
				log.Println("SpreadChan closed, stopping handleSpreadMessages")
				return
			}
			tb.sendSpreadMessage(sp)
		}
	}
}

func (tb *TelegramBot) PublishSpread(sp common.Spread) {
	go func() {
		tb.SpreadChan <- sp
	}()
}

func (tb *TelegramBot) sendSpreadMessage(sp common.Spread) {
	// предполагаем sp.UserID типа int64
	chatID := sp.UserID
	coin := strings.ToUpper(sp.Coin)

	// Проверка на паузу
	var paused int
	_ = tb.db.QueryRow("SELECT paused FROM users WHERE user_id = ?", sp.UserID).Scan(&paused)
	if paused == 1 {
		return // пользователь поставил бота на паузу — ничего не отправляем
	}

	// получить min_percent из БД (по user_id)
	var minPercent float64 = 1.0
	_ = tb.db.QueryRow("SELECT min_percent FROM users WHERE user_id = ?", sp.UserID).Scan(&minPercent)
	if (sp.Percent-1)*100 < minPercent {
		return // ниже порога — не уведомляем
	}

	// проверим, есть ли недавнее сообщение по этой монете
	var prevMsgID int
	var prevDateStr string
	err := tb.db.QueryRow(`
		SELECT message_id, timestamp
		  FROM messages_history
		 WHERE user_id = ? AND coin = ?
		 ORDER BY timestamp DESC
		 LIMIT 1
	`, sp.UserID, coin).Scan(&prevMsgID, &prevDateStr)

	shouldEdit := false
	if err == nil && prevMsgID != 0 {
		if prevTime, e := time.Parse("2006-01-02 15:04:05", prevDateStr); e == nil {
			if time.Since(prevTime) <= 10*time.Minute {
				shouldEdit = true
			}
		}
	}

	text := fmt.Sprintf(
		"📣 <b>Найден спред — %s</b>\n\n"+
			"🏷️ <b><a href=\"%s\">%s</a> → <a href=\"%s\">%s</a></b>\n\n"+
			"💵 <b>Покупка:</b> %.8f\n"+
			"💰 <b>Продажа:</b> %.8f\n"+
			"📈 <b>Процент:</b> %.2f%%\n"+
			"💸 <b>Профит (оценка):</b> %.8f\n"+
			"🔗 <b>Сеть:</b> %s",
		coin,
		GetLink(sp.Exchange1, coin), sp.Exchange1,
		GetLink(sp.Exchange2, coin), sp.Exchange2,
		sp.From, sp.To,
		100*sp.Percent,
		sp.Spread,
		sp.Chain,
	)

	// стикер можно заменить
	stickerID := "CAACAgIAAxkBAAEBHqpg5-xyz_sticker_id_here"

	if shouldEdit {
		edit := tgbotapi.NewEditMessageText(chatID, prevMsgID, text)

		edit.DisableWebPagePreview = true // DISABLED
		edit.ParseMode = "HTML"
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit failed: %v; отправляем новое сообщение", err)
			_, _ = tb.bot.Send(tgbotapi.NewSticker(chatID, tgbotapi.FileID(stickerID)))
			msg := tgbotapi.NewMessage(chatID, text)
			msg.ParseMode = "HTML"
			msg.DisableWebPagePreview = true
			if sent, err := tb.bot.Send(msg); err == nil {
				_, _ = tb.db.Exec(`
					INSERT INTO messages_history (coin, balance, spread, message_id, user_id, chat_id)
					VALUES (?, ?, ?, ?, ?, ?)`, coin, 0.0, sp.Spread, sent.MessageID, sp.UserID, chatID)
			}
		} else {
			_, _ = tb.db.Exec(`UPDATE messages_history SET timestamp = CURRENT_TIMESTAMP WHERE message_id = ?`, prevMsgID)
		}
	} else {
		// новый спред -> отправляем стикер и сообщение
		if _, err := tb.bot.Send(tgbotapi.NewSticker(chatID, tgbotapi.FileID(stickerID))); err != nil {
			// не критично
			log.Printf("sticker send error: %v", err)
		}
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "HTML"
		msg.DisableWebPagePreview = true // DISABLED PREVIEW
		sent, err := tb.bot.Send(msg)
		if err != nil {
			log.Printf("send message error: %v", err)
			return
		}
		_, _ = tb.db.Exec(`
			INSERT INTO messages_history (coin, balance, spread, message_id, user_id, chat_id)
			VALUES (?, ?, ?, ?, ?, ?)`, coin, 0.0, sp.Spread, sent.MessageID, sp.UserID, chatID)
	}
}

func (tb *TelegramBot) handleMessage(update tgbotapi.Update) {
	msg := update.Message
	chatID := msg.Chat.ID
	userID := msg.From.ID

	switch tb.userStates[chatID] {
	case stateRegLogin:
		tb.tempData[chatID] = msg.Text
		tb.userStates[chatID] = stateRegPassword
		tb.send(chatID, "🔐 Введите пароль:")
		return
	case stateRegPassword:
		login := tb.tempData[chatID]
		password := msg.Text
		tb.registerUser(userID, chatID, login, password)
		tb.userStates[chatID] = stateNone
		return
	case stateLoginLogin:
		tb.tempData[chatID] = msg.Text
		tb.userStates[chatID] = stateLoginPassword
		tb.send(chatID, "🔐 Введите пароль:")
		return
	case stateLoginPassword:
		login := tb.tempData[chatID]
		password := msg.Text
		// обновляем user_id и chat_id в БД при логине
		tb.loginUser(userID, chatID, login, password)
		tb.userStates[chatID] = stateNone
		return
	case stateCoinBan:
		// введён символ монеты для блокировки/разблокировки
		tb.handleCoinBanText(chatID, msg.Text)
		tb.userStates[chatID] = stateNone
		// после текстового ввода редактируем сохранённое filters-сообщение (если есть)
		if mid, ok := tb.lastFilterMessage[chatID]; ok && mid != 0 {
			tb.editFiltersToCoins(chatID, mid)
		}
		return
	case statePercentFilter:
		tb.handlePercentInputAndEdit(chatID, msg.Text)
		tb.userStates[chatID] = stateNone
		return
	case stateBalanceInput:
		tb.handleBalanceInputAndEdit(chatID, msg.Text)
		tb.userStates[chatID] = stateNone
		return
	default:
		// команды
		switch strings.TrimSpace(msg.Text) {
		case "/start":
			tb.send(chatID, "👋 Добро пожаловать в SpreadMakerBot!\n\n🔐 Используйте /reg для регистрации или /login для входа.")
		case "/reg":
			tb.userStates[chatID] = stateRegLogin
			tb.send(chatID, "📝 Введите желаемый логин:")
		case "/login":
			tb.userStates[chatID] = stateLoginLogin
			tb.send(chatID, "👤 Введите ваш логин:")
		case "/get_stat":
			tb.send(chatID, "📊 Статистика будет доступна скоро...")
		case "/filters":
			tb.showFilters(chatID)
		case "/status":
			tb.showStatusMenu(chatID)
		case "/menu":
			tb.showMenu(chatID)
		default:
			// игнорируем прочие сообщения
		}
	}
}

func (tb *TelegramBot) handleCallback(update tgbotapi.Update) {
	query := update.CallbackQuery
	chatID := query.Message.Chat.ID
	data := query.Data

	// убираем часы
	tb.answerCallback(query.ID, "")

	// --- menu callbacks (menu_...) ---
	if strings.HasPrefix(data, "menu_") || data == "menu_back" {
		switch data {
		case "menu_start":
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "👋 Добро пожаловать в SpreadMakerBot!\n\n🔐 Используйте /reg для регистрации или /login для входа.")
			if _, err := tb.bot.Send(edit); err != nil {
				// fallback: просто отправим
				tb.send(chatID, "👋 Добро пожаловать в SpreadMakerBot!\n\n🔐 Используйте /reg для регистрации или /login для входа.")
			}
			return
		case "menu_reg":
			// переводим в режим ввода логина
			tb.userStates[chatID] = stateRegLogin
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📝 Введите желаемый логин:")
			// добавим кнопку "Назад" чтобы вернуть меню
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
					{tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "menu_back")},
				},
			}
			if _, err := tb.bot.Send(edit); err != nil {
				tb.send(chatID, "📝 Введите желаемый логин:")
			}
			return
		case "menu_login":
			tb.userStates[chatID] = stateLoginLogin
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "👤 Введите ваш логин:")
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
					{tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "menu_back")},
				},
			}
			if _, err := tb.bot.Send(edit); err != nil {
				tb.send(chatID, "👤 Введите ваш логин:")
			}
			return
		case "menu_get_stat":
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📊 Статистика будет доступна скоро...")
			if _, err := tb.bot.Send(edit); err != nil {
				tb.send(chatID, "📊 Статистика будет доступна скоро...")
			}
			return
		case "menu_filters":
			// отредактируем сообщение и откроем фильтры
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "Открываю настройки фильтров...")
			if _, err := tb.bot.Send(edit); err != nil {
				// ignore
			}
			tb.showFilters(chatID)
			return
		case "menu_status":
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "Открываю меню статуса...")
			if _, err := tb.bot.Send(edit); err != nil {
				// ignore
			}
			tb.showStatusMenu(chatID)
			return
		case "menu_back":
			// вернём меню
			edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📋 Главное меню — выберите команду:")
			kb := tb.buildMenuKeyboard()
			edit.ReplyMarkup = &kb
			if _, err := tb.bot.Send(edit); err != nil {
				tb.send(chatID, "📋 Главное меню — выберите команду:")
			}
			return
		}
	}

	// --- filters navigation (existing) ---
	switch data {
	case "filters_exchanges":
		login, ok := tb.sessions[chatID]
		if !ok {
			tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
			return
		}
		kb := tb.buildExchangesKeyboard(login)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "🏦 Настройка бирж:")
		edit.ReplyMarkup = &kb
		edit.ParseMode = "HTML"
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit filters_exchanges failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return

	case "filters_coins":
		_, ok := tb.sessions[chatID]
		if !ok {
			tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
			return
		}
		tb.editFiltersToCoins(chatID, query.Message.MessageID)
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return

	case "filters_percent":
		login, ok := tb.sessions[chatID]
		if !ok {
			tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
			return
		}
		current := tb.getUserMinPercentByLogin(login)
		kb := tb.buildPercentKeyboard(login)
		text := fmt.Sprintf("📊 <b>Минимальный процент спреда</b>\n\nТекущий порог: <b>%.2f%%</b>\n\nНажмите «Изменить», чтобы ввести новый порог.", current)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit filters_percent failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return

	case "filters_balance":
		login, ok := tb.sessions[chatID]
		if !ok {
			tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
			return
		}
		current := tb.getUserBalanceByLogin(login)
		kb := tb.buildBalanceKeyboard(login)
		text := fmt.Sprintf("💰 <b>Баланс</b>\n\nТекущий баланс: <b>%.8f</b>\n\nНажмите «Изменить», чтобы ввести новый баланс.", current)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit filters_balance failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return

	case "filters_back":
		tb.editFiltersToMain(chatID, query.Message.MessageID)
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return

	case "filters_status":
		// открыть статус из меню фильтров
		tb.showStatusMenu(chatID)
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// переключение биржи
	if strings.HasPrefix(data, "ex_") {
		ex := strings.TrimPrefix(data, "ex_")
		tb.toggleExchange(chatID, ex)
		// сразу обновим клавиатуру в том же сообщении
		login := tb.sessions[chatID]
		if login == "" {
			tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
			return
		}
		kb := tb.buildExchangesKeyboard(login)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "🏦 Настройка бирж:")
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit after toggleExchange failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// управление монетами: нажали на одну из заблокированных монет -> разблокируем
	if strings.HasPrefix(data, "coin_") {
		coin := strings.TrimPrefix(data, "coin_")
		login := tb.sessions[chatID]
		if login == "" {
			tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
			return
		}
		if _, err := tb.db.Exec("DELETE FROM banned_coins WHERE login = ? AND coin = ?", login, coin); err != nil {
			log.Printf("coin unban error: %v", err)
			tb.send(chatID, "❌ Ошибка при разблокировке монеты.")
			return
		}
		// отредактируем то же сообщение — обновится список заблокированных монет
		tb.editFiltersToCoins(chatID, query.Message.MessageID)
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// добавить монету — переводим в режим ввода и редактируем сообщение
	if data == "coins_add" {
		tb.userStates[chatID] = stateCoinBan
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "🪙 Введите символ монеты для блокировки (например: BTC). Нажмите «Назад», чтобы вернуться.")
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
				{tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_coins_back")},
			},
		}
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit coins_add failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	if data == "filters_coins_back" {
		tb.editFiltersToCoins(chatID, query.Message.MessageID)
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// процент: перейти к вводу
	if data == "percent_edit" {
		tb.userStates[chatID] = statePercentFilter
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📊 Введите минимальный процент спреда (например: 1.5). Нажмите «Назад», чтобы отменить.")
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
				{tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_percent_back")},
			},
		}
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit percent_edit failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	if data == "filters_percent_back" {
		// показать меню процента с текущим порогом
		login := tb.sessions[chatID]
		if login == "" {
			tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
			return
		}
		current := tb.getUserMinPercentByLogin(login)
		kb := tb.buildPercentKeyboard(login)
		text := fmt.Sprintf("📊 <b>Минимальный процент спреда</b>\n\nТекущий порог: <b>%.2f%%</b>\n\nНажмите «Изменить», чтобы ввести новый порог.", current)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit filters_percent_back failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// баланс: перейти к вводу
	if data == "balance_edit" {
		tb.userStates[chatID] = stateBalanceInput
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "💰 Введите новый баланс (например: 123.45). Нажмите «Назад», чтобы отменить.")
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
				{tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_balance_back")},
			},
		}
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit balance_edit failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	if data == "filters_balance_back" {
		// показать меню баланса заново
		login, ok := tb.sessions[chatID]
		if !ok {
			tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
			return
		}
		current := tb.getUserBalanceByLogin(login)
		kb := tb.buildBalanceKeyboard(login)
		text := fmt.Sprintf("💰 <b>Баланс</b>\n\nТекущий баланс: <b>%.8f</b>\n\nНажмите «Изменить», чтобы ввести новый баланс.", current)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit filters_balance_back failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// переключение статуса (пауза/возобновление)
	if data == "status_toggle" {
		login, ok := tb.sessions[chatID]
		if !ok || login == "" {
			tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
			return
		}
		// прочитаем текущий paused
		var paused int
		if err := tb.db.QueryRow("SELECT paused FROM users WHERE login = ?", login).Scan(&paused); err != nil {
			log.Printf("status_toggle read paused error: %v", err)
			tb.send(chatID, "❌ Не удалось прочитать статус. Попробуйте позже.")
			return
		}
		newVal := 1
		if paused == 1 {
			newVal = 0
		}
		if _, err := tb.db.Exec("UPDATE users SET paused = ? WHERE login = ?", newVal, login); err != nil {
			log.Printf("status_toggle update paused error: %v", err)
			tb.send(chatID, "❌ Не удалось изменить статус. Попробуйте позже.")
			return
		}
		// обновим текст и клавиатуру в том же сообщении
		statusText := "▶️ <b>Активны</b>"
		if newVal == 1 {
			statusText = "⏸️ <b>Приостановлены</b>"
		}
		text := fmt.Sprintf("🔔 Статус уведомлений для <b>%s</b>:\n\nТекущее состояние: %s\n\nНажмите кнопку, чтобы переключить.", login, statusText)
		kb := tb.buildStatusKeyboard(login)
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("status_toggle edit send failed: %v", err)
		}
		tb.lastFilterMessage[chatID] = query.Message.MessageID
		return
	}

	// noop and fallback handled silently
}

func (tb *TelegramBot) registerUser(userID, chatID int64, login, password string) {
	_, err := tb.db.Exec("INSERT INTO users (login, password, plan, user_id, chat_id, balance, min_percent) VALUES (?, ?, ?, ?, ?, ?, ?)",
		login, password, "default", userID, chatID, 100.0, 1.0)
	if err != nil {
		log.Printf("registerUser error: %v", err)
		tb.send(chatID, "❌ Логин уже существует или ошибка. Попробуйте снова с /reg.")
		return
	}
	tb.send(chatID, "✅ Регистрация прошла успешно! Используйте /login для входа.")
}

// loginUser обновляет user_id и chat_id в БД и создаёт сессию
func (tb *TelegramBot) loginUser(userID, chatID int64, login, password string) {
	row := tb.db.QueryRow("SELECT 1 FROM users WHERE login = ? AND password = ?", login, password)
	var placeholder int
	if err := row.Scan(&placeholder); err != nil {
		tb.send(chatID, "❌ Неверные данные. Используйте /login, чтобы попробовать снова.")
		return
	}
	if _, err := tb.db.Exec("UPDATE users SET user_id = ?, chat_id = ? WHERE login = ?", userID, chatID, login); err != nil {
		log.Printf("loginUser update ids error: %v", err)
	}
	tb.sessions[chatID] = login

	// Обновим ин-мемори пользователей сразу после логина
	if err := tb.LoadCurrentUsers(); err != nil {
		log.Printf("LoadCurrentUsers after login warning: %v", err)
	}

	tb.send(chatID, fmt.Sprintf("✅ Вы вошли как %s", login))
}

func (tb *TelegramBot) showFilters(chatID int64) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
		return
	}
	msgText := fmt.Sprintf("⚙️ Настройки фильтров для <b>%s</b>:", login)
	msg := tgbotapi.NewMessage(chatID, msgText)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tb.buildMainFiltersKeyboard()
	sent, err := tb.bot.Send(msg)
	if err != nil {
		log.Printf("showFilters send error: %v", err)
		return
	}
	tb.lastFilterMessage[chatID] = sent.MessageID
}

func (tb *TelegramBot) showStatusMenu(chatID int64) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Пожалуйста, сначала выполните вход с помощью /login.")
		return
	}
	var paused int
	_ = tb.db.QueryRow("SELECT paused FROM users WHERE login = ?", login).Scan(&paused)
	statusText := "▶️ <b>Активны</b>"
	if paused == 1 {
		statusText = "⏸️ <b>Приостановлены</b>"
	}
	text := fmt.Sprintf("🔔 Статус уведомлений для <b>%s</b>:\n\nТекущее состояние: %s\n\nНажмите кнопку, чтобы переключить.", login, statusText)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	kb := tb.buildStatusKeyboard(login)
	msg.ReplyMarkup = &kb
	sent, err := tb.bot.Send(msg)
	if err != nil {
		log.Printf("showStatusMenu send error: %v", err)
		return
	}
	tb.lastFilterMessage[chatID] = sent.MessageID
}

func (tb *TelegramBot) editFiltersToMain(chatID int64, messageID int) {
	login := tb.sessions[chatID]
	if login == "" {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("⚙️ Настройки фильтров для <b>%s</b>:", login))
	edit.ParseMode = "HTML"
	kb := tb.buildMainFiltersKeyboard()
	edit.ReplyMarkup = &kb
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("editFiltersToMain failed: %v", err)
	}
}

func (tb *TelegramBot) editFiltersToCoins(chatID int64, messageID int) {
	login := tb.sessions[chatID]
	if login == "" {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, messageID, "🪙 Управление монетами:")
	kb := tb.buildCoinsKeyboard(login)
	edit.ReplyMarkup = &kb
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("editFiltersToCoins failed: %v", err)
	}
}

// -- Построение клавиатур (единичная точка правки) --

// buildMenuKeyboard — клавиатура с основными командами (смайлики + описание)
func (tb *TelegramBot) buildMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 Старт", "menu_start"),
			tgbotapi.NewInlineKeyboardButtonData("📝 Регистрация", "menu_reg"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔐 Вход", "menu_login"),
			tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "menu_get_stat"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚙️ Фильтры", "menu_filters"),
			tgbotapi.NewInlineKeyboardButtonData("🔔 Статус", "menu_status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ Помощь", "menu_start"),
		),
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (tb *TelegramBot) buildMainFiltersKeyboard() tgbotapi.InlineKeyboardMarkup {
	row1 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🏦 Биржи", "filters_exchanges"),
		tgbotapi.NewInlineKeyboardButtonData("🪙 Монеты", "filters_coins"),
	)
	row2 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📊 Процент", "filters_percent"),
		tgbotapi.NewInlineKeyboardButtonData("💰 Баланс", "filters_balance"),
	)
	row3 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔔 Статус", "filters_status"),
	)
	return tgbotapi.NewInlineKeyboardMarkup(row1, row2, row3)
}

func (tb *TelegramBot) buildExchangesKeyboard(login string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	line := []tgbotapi.InlineKeyboardButton{}
	for i, ex := range exchanges.RExchanges {
		var exists int
		err := tb.db.QueryRow("SELECT 1 FROM banned_exchanges WHERE login = ? AND exchange = ?", login, ex).Scan(&exists)
		banned := err == nil
		icon := "✅"
		if banned {
			icon = "🚫"
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s %s", icon, ex), "ex_"+ex)
		line = append(line, btn)
		if len(line) == 2 || i == len(exchanges.RExchanges)-1 {
			rows = append(rows, line)
			line = []tgbotapi.InlineKeyboardButton{}
		}
	}
	// назад
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_back")))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (tb *TelegramBot) buildCoinsKeyboard(login string) tgbotapi.InlineKeyboardMarkup {
	coins := []string{}
	rows := [][]tgbotapi.InlineKeyboardButton{}

	r, err := tb.db.Query("SELECT coin FROM banned_coins WHERE login = ? ORDER BY coin", login)
	if err == nil {
		defer r.Close()
		for r.Next() {
			var coin string
			if err := r.Scan(&coin); err == nil {
				coins = append(coins, coin)
			}
		}
	}

	// если есть заблокированные монеты — показываем их кнопками (по 2 в ряд)
	if len(coins) > 0 {
		row := []tgbotapi.InlineKeyboardButton{}
		for i, c := range coins {
			btn := tgbotapi.NewInlineKeyboardButtonData("🔒 "+c, "coin_"+c)
			row = append(row, btn)
			if len(row) == 2 || i == len(coins)-1 {
				rows = append(rows, row)
				row = []tgbotapi.InlineKeyboardButton{}
			}
		}
	} else {
		// подсказка, если нет заблокированных монет
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Нет заблокированных монет", "noop")))
	}

	// кнопки "Добавить" и "Назад"
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("➕ Добавить монету", "coins_add")))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_back")))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (tb *TelegramBot) buildPercentKeyboard(login string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("📈 Изменить", "percent_edit")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_back")),
	)
}

func (tb *TelegramBot) buildBalanceKeyboard(login string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("💰 Изменить", "balance_edit")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_balance_back")),
	)
}

// Клавиатура статуса — показывает кнопку с текущим статусом (и кнопку Назад)
func (tb *TelegramBot) buildStatusKeyboard(login string) tgbotapi.InlineKeyboardMarkup {
	var paused int
	_ = tb.db.QueryRow("SELECT paused FROM users WHERE login = ?", login).Scan(&paused)
	label := "▶️ Активны"
	if paused == 1 {
		label = "⏸️ Приостановлены"
	}
	row1 := tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(label, "status_toggle"))
	row2 := tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "filters_back"))
	return tgbotapi.NewInlineKeyboardMarkup(row1, row2)
}

// -- операции с DB --

func (tb *TelegramBot) toggleExchange(chatID int64, exchange string) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	var exists int
	if err := tb.db.QueryRow("SELECT 1 FROM banned_exchanges WHERE login = ? AND exchange = ?", login, exchange).Scan(&exists); err == nil {
		if _, err := tb.db.Exec("DELETE FROM banned_exchanges WHERE login = ? AND exchange = ?", login, exchange); err != nil {
			log.Printf("toggleExchange delete error: %v", err)
		}
	} else {
		if _, err := tb.db.Exec("INSERT INTO banned_exchanges (login, exchange) VALUES (?, ?)", login, exchange); err != nil {
			log.Printf("toggleExchange insert error: %v", err)
		}
	}
}

func (tb *TelegramBot) handleCoinBanText(chatID int64, text string) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	coin := strings.ToUpper(strings.TrimSpace(text))
	if coin == "" {
		tb.send(chatID, "❗ Неверный символ монеты.")
		return
	}
	var exists int
	if err := tb.db.QueryRow("SELECT 1 FROM banned_coins WHERE login = ? AND coin = ?", login, coin).Scan(&exists); err == nil {
		// есть — удаляем
		if _, err := tb.db.Exec("DELETE FROM banned_coins WHERE login = ? AND coin = ?", login, coin); err != nil {
			log.Printf("handleCoinBan delete error: %v", err)
			tb.send(chatID, "❌ Ошибка при разблокировке монеты.")
			return
		}
		tb.send(chatID, fmt.Sprintf("✅ Монета %s разблокирована", coin))
	} else {
		// нет — добавляем
		if _, err := tb.db.Exec("INSERT INTO banned_coins (login, coin) VALUES (?, ?)", login, coin); err != nil {
			log.Printf("handleCoinBan insert error: %v", err)
			tb.send(chatID, "❌ Ошибка при блокировке монеты.")
			return
		}
		tb.send(chatID, fmt.Sprintf("🚫 Монета %s заблокирована", coin))
	}
}

func (tb *TelegramBot) handlePercentInputAndEdit(chatID int64, text string) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	val := strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	percent, err := strconv.ParseFloat(val, 64)
	if err != nil || percent < 0 {
		tb.send(chatID, "❌ Неверный формат. Введите положительное число, например: 1.5")
		return
	}
	if _, err := tb.db.Exec("UPDATE users SET min_percent = ? WHERE login = ?", percent, login); err != nil {
		log.Printf("handlePercentInput update error: %v", err)
		tb.send(chatID, "❌ Ошибка при сохранении значения.")
		return
	}

	// обновим список пользователей в памяти (если нужно)
	if err := tb.LoadCurrentUsers(); err != nil {
		log.Printf("LoadCurrentUsers after percent update warning: %v", err)
	}

	// редактируем последнее filters-сообщение, если есть
	if mid, ok := tb.lastFilterMessage[chatID]; ok && mid != 0 {
		kb := tb.buildPercentKeyboard(login)
		text := fmt.Sprintf("📊 <b>Минимальный процент спреда</b>\n\nТекущий порог: <b>%.2f%%</b>\n\nНажмите «Изменить», чтобы ввести новый порог.", percent)
		edit := tgbotapi.NewEditMessageText(chatID, mid, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit after percent update failed: %v", err)
		}
	} else {
		tb.send(chatID, fmt.Sprintf("✅ Минимальный процент установлен: %.2f%%", percent))
	}
}

func (tb *TelegramBot) handleBalanceInputAndEdit(chatID int64, text string) {
	login, ok := tb.sessions[chatID]
	if !ok {
		tb.send(chatID, "❗ Сначала выполните вход с помощью /login.")
		return
	}
	val := strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	balance, err := strconv.ParseFloat(val, 64)
	if err != nil || balance < 0 {
		tb.send(chatID, "❌ Неверный формат. Введите положительное число, например: 123.45")
		return
	}
	if _, err := tb.db.Exec("UPDATE users SET balance = ? WHERE login = ?", balance, login); err != nil {
		log.Printf("handleBalanceInput update error: %v", err)
		tb.send(chatID, "❌ Ошибка при сохранении значения.")
		return
	}

	// обновим список пользователей в памяти (чтобы баланс отражался в CurrentUsers)
	if err := tb.LoadCurrentUsers(); err != nil {
		log.Printf("LoadCurrentUsers after balance update warning: %v", err)
	}

	// редактируем последнее filters-сообщение, если есть
	if mid, ok := tb.lastFilterMessage[chatID]; ok && mid != 0 {
		kb := tb.buildBalanceKeyboard(login)
		text := fmt.Sprintf("💰 <b>Баланс</b>\n\nТекущий баланс: <b>%.8f</b>\n\nНажмите «Изменить», чтобы ввести новый баланс.", balance)
		edit := tgbotapi.NewEditMessageText(chatID, mid, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		if _, err := tb.bot.Send(edit); err != nil {
			log.Printf("edit after balance update failed: %v", err)
		}
	} else {
		tb.send(chatID, fmt.Sprintf("✅ Баланс установлен: %.8f", balance))
	}
}

func (tb *TelegramBot) getUserMinPercentByLogin(login string) float64 {
	var p float64 = 1.0
	_ = tb.db.QueryRow("SELECT min_percent FROM users WHERE login = ?", login).Scan(&p)
	if p == 0 {
		p = 1.0
	}
	return p
}

func (tb *TelegramBot) getUserBalanceByLogin(login string) float64 {
	var b float64 = 100.0
	_ = tb.db.QueryRow("SELECT balance FROM users WHERE login = ?", login).Scan(&b)
	return b
}

func (tb *TelegramBot) answerCallback(callbackID, text string) {
	if _, err := tb.bot.Request(tgbotapi.CallbackConfig{
		CallbackQueryID: callbackID,
		Text:            text,
	}); err != nil {
		log.Printf("answerCallback error: %v", err)
	}
}

func (tb *TelegramBot) send(chatID int64, text string) {
	if _, err := tb.bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("send error: %v", err)
	}
}

// showMenu отправляет/редактирует меню команд
func (tb *TelegramBot) showMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "📋 Главное меню — выберите команду:")
	kb := tb.buildMenuKeyboard()
	msg.ReplyMarkup = &kb
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("showMenu send error: %v", err)
	}
}

func GetLink(exchange, coin string) string {
	switch exchange {
	case "ASCENDEX":
		return fmt.Sprintf("https://ascendex.com/en/cashtrade-spottrading/%s/%s", common.BaseCurrency, coin)
	case "BINANCE":
		return fmt.Sprintf("https://www.binance.com/en/trade/%s_%s", coin, common.BaseCurrency)
	case "BITGET":
		return fmt.Sprintf("https://www.bitget.com/spot/%s%s", coin, common.BaseCurrency)
	case "BYBIT":
		return fmt.Sprintf("https://www.bybit.com/en/trade/spot/%s/%s", coin, common.BaseCurrency)
	case "COINEX":
		return fmt.Sprintf("https://www.coinex.com/en/exchange/%s-%s", coin, common.BaseCurrency)
	case "KUCOIN":
		return fmt.Sprintf("https://www.kucoin.com/trade/%s-%s", coin, common.BaseCurrency)
	case "MEXC":
		return fmt.Sprintf("https://www.mexc.com/exchange/%s_%s", coin, common.BaseCurrency)
	case "OKX":
		return fmt.Sprintf("https://www.okx.com/ru/trade-spot/%s-%s", coin, common.BaseCurrency)
	default:
		return ""
	}
}
