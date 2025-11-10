package telegram

import (
	"log"
)

var (
	BotToken string
	ChatID   int64
)

func LoadConfig() {
	BotToken = "8017784237:AAE7SQI1nyNAdiUCmAP86ERUJTIiXAOs3Os" // Ganti dengan token bot Anda
	ChatID = 7412135090                                         // Ganti dengan chat ID Anda

	if BotToken == "YOUR_TELEGRAM_BOT_TOKEN" {
		log.Println("Peringatan: Harap ganti 'YOUR_TELEGRAM_BOT_TOKEN' dengan token bot Telegram Anda yang sebenarnya di internal/telegram/config.go")
	}
	if ChatID == 123456789 {
		log.Println("Peringatan: Harap ganti '123456789' dengan chat ID Telegram Anda yang sebenarnya di internal/telegram/config.go")
	}
}
