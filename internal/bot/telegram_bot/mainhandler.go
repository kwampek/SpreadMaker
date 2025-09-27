package telegrambot

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (tb *TelegramBot) findUserByTelegramID(telegramID int64) *User {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	for _, user := range tb.Users {
		if user.UserID == telegramID {
			return user
		}
	}
	return nil
}

func (tb *TelegramBot) sendMessage(chatID int64, text string, buttons []string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"

	if len(buttons) > 0 {
		// Создаём клавиатуру с кнопками в одной строке
		var keyboardRow []tgbotapi.KeyboardButton
		for _, b := range buttons {
			keyboardRow = append(keyboardRow, tgbotapi.NewKeyboardButton(b))
		}
		keyboard := tgbotapi.NewReplyKeyboard(keyboardRow)
		keyboard.OneTimeKeyboard = true
		keyboard.ResizeKeyboard = true
		msg.ReplyMarkup = keyboard
	} else {
		// Убираем клавиатуру
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	}

	_, err := tb.Bot.Send(msg)
	return err
}

func NewTelegramBot(token string) *TelegramBot {
	// Подключаемся к Telegram API
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("❌ Ошибка подключения к Telegram API: %v", err)
	}

	bot.Debug = false // Включить при отладке

	log.Printf("🤖 Бот авторизован как %s", bot.Self.UserName)

	tb := &TelegramBot{
		Bot:          bot,
		Users:        make(map[string]*User),
		AuthSessions: make(map[int64]*PendingAuth),
	}

	// Инициализация базы данных
	if err := InitDb(DBPath); err != nil {
		log.Fatalf("❌ Ошибка инициализации БД: %v", err)
	}

	// Загрузка пользователей из базы
	if err := tb.LoadAllUsersFromDb(); err != nil {
		log.Fatalf("❌ Ошибка загрузки пользователей: %v", err)
	}

	// Запускаем обработку сообщений

	go tb.listenUpdates()

	return tb
}

func StartTelegramBot() {
	NewTelegramBot(BotToken)
	select {} // блокировка main, чтобы бот не завершался
}

func (tb *TelegramBot) listenUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := tb.Bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		msg := update.Message
		chatID := msg.Chat.ID
		userID := msg.From.ID
		text := msg.Text

		if text == "/start" {
			tb.sendMessage(chatID, "👋 Добро пожаловать! Введите /login для входа.", nil)
			continue
		}

		// Обработка авторизации и команд
		if _, ok := tb.AuthSessions[chatID]; ok {
			fmt.Println("NO AUTH SESSEION CHATID: ", chatID)
			tb.HandleAuthStep(chatID, text)
			continue
		}

		user := tb.findUserByTelegramID(userID)

		if user == nil {
			fmt.Println("MESSAGE FROM UNREG USER", userID, chatID)

			if text != "/login" {
				fmt.Println("LOGIN: ", userID, chatID)
				tb.sendMessage(chatID, "❌ Вы не авторизованы. Введите /login", nil)
				continue
			}
			tb.StartAuthorization(chatID, userID)
			fmt.Println("start authorization ", userID, chatID)
			continue
		}

		switch text {
		case "/filters":
			tb.StartFilterEdit(chatID, user)
		default:
			tb.sendMessage(chatID, "❓ Неизвестная команда. Введите /login для авторизации.", nil)
		}
	}

	fmt.Println("Stop listen updates")
}
