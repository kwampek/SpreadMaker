package telegrambot

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

const (
	AdminLogin    = "admin"
	AdminPassword = "securepassword"
	DBPath        = "../internal/bot/telegram_bot/base.sql"
	BotToken      = "" // ADD
)

func InitDb(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	defer db.Close()

	schema := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			user_id INTEGER DEFAULT 0,
			chat_id INTEGER DEFAULT 0,
			login TEXT UNIQUE,
			password TEXT,
			balance REAL DEFAULT 100.0
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

	for _, stmt := range schema {
		_, err := db.Exec(stmt)
		if err != nil {
			return fmt.Errorf("failed to execute statement: %v\nstatement: %s", err, stmt)
		}
	}

	return nil
}

func (tb *TelegramBot) LoadAllUsersFromDb() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	db, err := sql.Open("sqlite3", DBPath)
	if err != nil {
		return fmt.Errorf("failed to open DB: %w", err)
	}
	defer db.Close()

	users, err := tb.loadUsers(db)
	if err != nil {
		return err
	}

	tb.Users = make(map[string]*User)
	tb.CurrentUsers = make([]UserBalance, 0, len(users)) // сбрасываем и подготавливаем

	for _, user := range users {
		if err := tb.loadBannedCoins(db, user); err != nil {
			log.Printf("failed to load banned coins for %s: %v", user.Login, err)
		}
		if err := tb.loadBannedExchanges(db, user); err != nil {
			log.Printf("failed to load banned exchanges for %s: %v", user.Login, err)
		}
		tb.Users[user.Login] = user

		// Добавляем в CurrentUsers
		tb.CurrentUsers = append(tb.CurrentUsers, UserBalance{
			Balance: user.Balance,
			UserId:  int(user.UserID),
		})
	}

	return nil
}

func (tb *TelegramBot) loadUsers(db *sql.DB) ([]*User, error) {
	rows, err := db.Query(`SELECT id, user_id, chat_id, login, password, balance FROM users`)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*User

	for rows.Next() {
		var user User
		err := rows.Scan(&user.Id, &user.UserID, &user.ChatID, &user.Login, &user.Password, &user.Balance)
		if err != nil {
			log.Printf("failed to scan user: %v", err)
			continue
		}

		user.BannedCoins = make(map[string]struct{})
		user.BannedExchanges = make(map[string]struct{})
		user.LastMessages = make(map[string]Message)

		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error while iterating user rows: %w", err)
	}

	return users, nil
}

func (tb *TelegramBot) loadBannedCoins(db *sql.DB, user *User) error {
	rows, err := db.Query(`SELECT coin FROM banned_coins WHERE login = ?`, user.Login)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var coin string
		if err := rows.Scan(&coin); err == nil {
			user.BannedCoins[coin] = struct{}{}
		}
	}
	return rows.Err()
}

func (tb *TelegramBot) loadBannedExchanges(db *sql.DB, user *User) error {
	rows, err := db.Query(`SELECT exchange FROM banned_exchanges WHERE login = ?`, user.Login)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var exchange string
		if err := rows.Scan(&exchange); err == nil {
			user.BannedExchanges[exchange] = struct{}{}
		}
	}
	return rows.Err()
}

/*
func StartTgBot() {
	// Инициализация базы данных
	db, err := initDB()
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	// Загрузка данных из БД
	if err := loadInitialData(db); err != nil {
		log.Fatalf("Error loading initial data: %v", err)
	}

	// Инициализация бота
	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		log.Panicf("Error creating bot: %v", err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)

	// Обработка входящих сообщений
	for update := range updates {
		if update.Message == nil {
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
		userID := update.Message.From.ID
		chatID := update.Message.Chat.ID

		switch {
		case update.Message.IsCommand():
			switch update.Message.Command() {
			case "start":
				handleStart(db, bot, update.Message)
			case "login":
				msg.Text = "Введите логин и пароль в формате: /auth логин пароль"
			case "auth":
				handleAuth(db, bot, update.Message)
			case "add_coin":
				handleAddCoin(db, bot, update.Message)
			case "remove_coin":
				handleRemoveCoin(db, bot, update.Message)
			case "add_exchange":
				handleAddExchange(db, bot, update.Message)
			case "remove_exchange":
				handleRemoveExchange(db, bot, update.Message)
			case "list_banned":
				handleListBanned(bot, update.Message)
			case "last_messages":
				handleLastMessages(bot, update.Message)
			default:
				msg.Text = "Неизвестная команда"
			}
		default:
			// Сохранение обычного сообщения
			handleMessage(db, bot, update.Message)
		}

		if msg.Text != "" {
			bot.Send(msg)
		}
	}
}

// Инициализация базы данных
func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", DBPath)
	if err != nil {
		return nil, err
	}

	// Создание таблиц
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY
			user_id INTEGER,
			chat_id INTEGER NOT NULL,
			login TEXT UNIQUE,
			password TEXT,
			balance REAL DEFAULT 100.0
		)`,
		`CREATE TABLE IF NOT EXISTS banned_coins (
			login TEXT
			coin TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS banned_exchanges (
			login TEXT
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

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return nil, fmt.Errorf("error creating table: %v, query: %s", err, query)
		}
	}

	return db, nil
}

// Загрузка начальных данных
func loadInitialData(db *sql.DB) error {
	// Загрузка пользователей
	rows, err := db.Query("SELECT user_id, chat_id, login, password, balance FROM users")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UserID, &u.ChatID, &u.Login, &u.Password, &u.Balance); err != nil {
			return err
		}
		storage.users[u.UserID] = &u
	}
	storage.sortUsersByBalance()

	// Загрузка запрещенных монет
	rows, err = db.Query("SELECT coin_symbol FROM banned_coins")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var coin string
		if err := rows.Scan(&coin); err != nil {
			return err
		}
		storage.bannedCoins[coin] = true
	}

	// Загрузка запрещенных бирж
	rows, err = db.Query("SELECT exchange_name FROM banned_exchanges")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var exchange string
		if err := rows.Scan(&exchange); err != nil {
			return err
		}
		storage.bannedExchanges[exchange] = true
	}

	// Загрузка последних сообщений
	rows, err = db.Query(`
		SELECT symbol, message_id, user_id, chat_id, timestamp
		FROM messages
		ORDER BY timestamp DESC
		LIMIT 10
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	storage.lastMessages = make([]Message, 0, 10)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Symbol, &m.MessageID, &m.UserID, &m.ChatID, &m.Timestamp); err != nil {
			return err
		}
		storage.lastMessages = append(storage.lastMessages, m)
	}

	return nil
}

// Обработка команды /start
func handleStart(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	storage.mu.Lock()
	defer storage.mu.Unlock()

	// Проверка существующего пользователя
	if _, exists := storage.users[userID]; exists {
		reply := tgbotapi.NewMessage(chatID, "Вы уже зарегистрированы!")
		bot.Send(reply)
		return
	}

	// Создание нового пользователя
	newUser := &User{
		UserID:  userID,
		ChatID:  chatID,
		Balance: 0.0,
	}

	// Сохранение в БД
	_, err := db.Exec(`
		INSERT INTO users (user_id, chat_id, balance)
		VALUES (?, ?, ?)`,
		userID, chatID, 0.0)

	if err != nil {
		log.Printf("Error creating user: %v", err)
		reply := tgbotapi.NewMessage(chatID, "Ошибка регистрации")
		bot.Send(reply)
		return
	}

	// Сохранение в памяти
	storage.users[userID] = newUser
	storage.sortUsersByBalance()

	reply := tgbotapi.NewMessage(chatID,
		"Добро пожаловать! Используйте /auth логин пароль для авторизации")
	bot.Send(reply)
}

// Обработка авторизации
func handleAuth(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) < 2 {
		reply := tgbotapi.NewMessage(msg.Chat.ID,
			"Неверный формат. Используйте: /auth логин пароль")
		bot.Send(reply)
		return
	}

	login := args[0]
	password := args[1]
	userID := msg.From.ID
	chatID := msg.Chat.ID

	storage.mu.Lock()
	defer storage.mu.Unlock()

	user, exists := storage.users[userID]
	if !exists {
		reply := tgbotapi.NewMessage(chatID, "Сначала зарегистрируйтесь с помощью /start")
		bot.Send(reply)
		return
	}

	// Проверка учетных данных
	if login == AdminLogin && password == AdminPassword {
		user.Login = login
		user.Password = password

		// Обновление в БД
		_, err := db.Exec(`
			UPDATE users
			SET login = ?, password = ?
			WHERE user_id = ?`,
			login, password, userID)

		if err != nil {
			log.Printf("Error updating user: %v", err)
			reply := tgbotapi.NewMessage(chatID, "Ошибка авторизации")
			bot.Send(reply)
			return
		}

		reply := tgbotapi.NewMessage(chatID, "Авторизация успешна!")
		bot.Send(reply)
	} else {
		reply := tgbotapi.NewMessage(chatID, "Неверные учетные данные")
		bot.Send(reply)
	}
}

// Обработчик добавления монеты
func handleAddCoin(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	// Реализация аналогична handleAddExchange
}

// Обработчик добавления биржи
func handleAddExchange(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	userID := msg.From.ID
	exchange := strings.TrimSpace(msg.CommandArguments())

	if exchange == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Укажите название биржи")
		bot.Send(reply)
		return
	}

	storage.mu.Lock()
	defer storage.mu.Unlock()

	// Проверка прав администратора
	user, exists := storage.users[userID]
	if !exists || user.Login != AdminLogin {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Доступ запрещен")
		bot.Send(reply)
		return
	}

	// Добавление в базу данных
	_, err := db.Exec(`
		INSERT OR IGNORE INTO banned_exchanges (exchange_name)
		VALUES (?)`, exchange)

	if err != nil {
		log.Printf("Error adding exchange: %v", err)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Ошибка добавления биржи")
		bot.Send(reply)
		return
	}

	// Обновление в памяти
	storage.bannedExchanges[exchange] = true

	reply := tgbotapi.NewMessage(msg.Chat.ID,
		fmt.Sprintf("Биржа %s добавлена в запрещенные", exchange))
	bot.Send(reply)
}

// Вывод списка запрещенных элементов
func handleListBanned(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()

	response := "🚫 Запрещенные монеты:\n"
	for coin := range storage.bannedCoins {
		response += fmt.Sprintf("- %s\n", coin)
	}

	response += "\n🚫 Запрещенные биржи:\n"
	for exchange := range storage.bannedExchanges {
		response += fmt.Sprintf("- %s\n", exchange)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	bot.Send(reply)
}

// Обработка обычных сообщений
func handleMessage(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	// Сохранение сообщения
	newMsg := Message{
		Symbol:    "N/A", // Здесь можно добавить парсинг символа
		MessageID: msg.MessageID,
		UserID:    msg.From.ID,
		ChatID:    msg.Chat.ID,
		Timestamp: time.Now(),
	}

	storage.mu.Lock()
	defer storage.mu.Unlock()

	// Сохранение в БД
	_, err := db.Exec(`
		INSERT INTO messages (symbol, message_id, user_id, chat_id)
		VALUES (?, ?, ?, ?)`,
		newMsg.Symbol, newMsg.MessageID, newMsg.UserID, newMsg.ChatID)

	if err != nil {
		log.Printf("Error saving message: %v", err)
	}

	// Обновление в памяти
	if len(storage.lastMessages) >= 10 {
		storage.lastMessages = storage.lastMessages[1:]
	}
	storage.lastMessages = append(storage.lastMessages, newMsg)

	// Здесь можно добавить обработку сообщения (проверка запрещенных монет/бирж и т.д.)
}

// Сортировка пользователей по балансу
func (s *Storage) sortUsersByBalance() {
	s.usersByBalance = make([]*User, 0, len(s.users))
	for _, user := range s.users {
		s.usersByBalance = append(s.usersByBalance, user)
	}

	sort.Slice(s.usersByBalance, func(i, j int) bool {
		return s.usersByBalance[i].Balance > s.usersByBalance[j].Balance
	})
}

// Вывод последних сообщений
func handleLastMessages(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()

	response := "📝 Последние 10 сообщений:\n\n"
	for i, m := range storage.lastMessages {
		response += fmt.Sprintf("%d. [%s] Пользователь: %d, Чат: %d\n   Время: %s\n",
			i+1,
			m.Symbol,
			m.UserID,
			m.ChatID,
			m.Timestamp.Format("2006-01-02 15:04:05"))
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	bot.Send(reply)
}

// Обновление баланса (пример)
func updateUserBalance(db *sql.DB, userID int64, newBalance float64) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	user, exists := storage.users[userID]
	if !exists {
		return fmt.Errorf("user not found")
	}

	// Обновление в БД
	_, err := db.Exec("UPDATE users SET balance = ? WHERE user_id = ?", newBalance, userID)
	if err != nil {
		return err
	}

	// Обновление в памяти
	user.Balance = newBalance
	storage.sortUsersByBalance()
	return nil
}

// Пример использования обновления баланса
func updateBalanceHandler(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) < 1 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Укажите новый баланс")
		bot.Send(reply)
		return
	}

	newBalance, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Неверный формат баланса")
		bot.Send(reply)
		return
	}

	if err := updateUserBalance(db, msg.From.ID, newBalance); err != nil {
		log.Printf("Error updating balance: %v", err)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Ошибка обновления баланса")
		bot.Send(reply)
		return
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Баланс обновлен: %.2f", newBalance))
	bot.Send(reply)
}
*/
