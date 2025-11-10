package telegram

import (
	"fmt"
	"log"
	"sync"
	"time"
	timed "IoTT/internal/time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	bot             *tgbotapi.BotAPI
	lastMessageTime time.Time
	messageMutex    sync.Mutex
)

const messageInterval = 2 * time.Second // Hanya izinkan 1 pesan setiap 2 detik

func InitBot() error {
	if BotToken == "" || ChatID == 0 {
		return nil
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		return fmt.Errorf("gagal membuat instance bot Telegram: %w", err)
	}

	lastMessageTime = timed.Now().Add(-messageInterval) // Inisialisasi agar pesan pertama bisa langsung dikirim
	return nil
}

func SendAlert(messageText string) {
	if bot == nil || ChatID == 0 {
		return
	}

	go func(msgTxt string) {
		messageMutex.Lock()
		defer messageMutex.Unlock()

		if time.Since(lastMessageTime) < messageInterval {
			time.Sleep(messageInterval - time.Since(lastMessageTime))
		}

		msg := tgbotapi.NewMessage(ChatID, msgTxt)
		msg.ParseMode = tgbotapi.ModeMarkdown
		msg.DisableWebPagePreview = true

		if _, err := bot.Send(msg); err != nil {
			log.Printf("Error mengirim pesan Telegram: %v", err)
		}
		lastMessageTime = timed.Now()
	}(messageText)
}
