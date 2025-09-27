package telegrambot

import (
	"database/sql"
	"fmt"
)

type AuthState int

const (
	AuthIdle AuthState = iota
	AuthWaitingLogin
	AuthWaitingPassword
)

type PendingAuth struct {
	Login      string
	State      AuthState
	ChatID     int64
	TelegramID int64
}

func (tb *TelegramBot) StartAuthorization(chatID, telegramID int64) error {
	tb.mu.Lock()
	tb.AuthSessions[chatID] = &PendingAuth{
		State:      AuthWaitingLogin,
		ChatID:     chatID,
		TelegramID: telegramID,
	}
	tb.mu.Unlock()

	return tb.sendMessage(chatID, "🔐 Введите ваш *логин* для авторизации:", []string{"🔙 Назад"})
}

func (tb *TelegramBot) HandleAuthStep(chatID int64, input string) error {
	tb.mu.RLock()
	session, ok := tb.AuthSessions[chatID]
	tb.mu.RUnlock()

	if !ok {
		return tb.sendMessage(chatID, "Сначала введите /login для начала авторизации.", nil)
	}

	switch session.State {
	case AuthWaitingLogin:
		if input == "🔙 Назад" {
			return tb.cancelAuth(chatID)
		}
		session.Login = input
		session.State = AuthWaitingPassword
		tb.sendMessage(chatID, fmt.Sprintf("🔑 Введите пароль для логина *%s*:", input), []string{"🔙 Назад"})

	case AuthWaitingPassword:
		if input == "🔙 Назад" {
			session.State = AuthWaitingLogin
			return tb.sendMessage(chatID, "🔐 Введите ваш *логин* для авторизации:", []string{"🔙 Назад"})
		}
		login := session.Login
		password := input

		user, ok := tb.Users[login]
		if !ok || user.Password != password {
			return tb.sendMessage(chatID, "❌ Неверный логин или пароль. Попробуйте снова:", []string{"🔙 Назад"})
		}

		// Обновим user_id в БД
		err := updateUserIDInDB("internal/bot/telegram_bot/base.sql", login, session.TelegramID)
		if err != nil {
			return tb.sendMessage(chatID, "⚠️ Ошибка при сохранении user_id. Попробуйте позже.", nil)
		}

		tb.mu.Lock()
		user.UserID = session.TelegramID
		delete(tb.AuthSessions, chatID)
		tb.mu.Unlock()

		return tb.sendMessage(chatID, fmt.Sprintf("✅ Добро пожаловать, *%s*! Вы успешно авторизованы.", login), nil)
	}

	return nil
}

func (tb *TelegramBot) cancelAuth(chatID int64) error {
	//tb.mu.Lock()
	delete(tb.AuthSessions, chatID)
	//tb.mu.Unlock()

	return tb.sendMessage(chatID, "🚪 Вы вышли из авторизации.", nil)
}

func updateUserIDInDB(dbPath, login string, userID int64) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`UPDATE users SET user_id = ? WHERE login = ?`, userID, login)
	return err
}
