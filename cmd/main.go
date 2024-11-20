package main

import (
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tgbot "github.com/rarebek/supervisor-tg-notifier/pkg/bot"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
)

func main() {
	supervisorClient, err := supervisor.NewClient(config.ServerURL)
	if err != nil {
		log.Fatalf("Error creating supervisor client: %v", err)
	}
	defer supervisorClient.Close()

	bot, err := tgbotapi.NewBotAPI(config.TelegramBotToken)
	if err != nil {
		log.Fatalf("Error creating Telegram bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	handler := tgbot.NewHandler(bot, supervisorClient)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go handler.HandleUpdates()

	for range ticker.C {
		handler.CheckProcessStatuses()
	}
}
