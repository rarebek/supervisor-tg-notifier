package main

import (
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tgbot "github.com/rarebek/supervisor-tg-notifier/pkg/bot"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
)

func main() {
	serverURLs := strings.Split(config.ServerURLs, ",")
	supervisorClients := make(map[string]*supervisor.Client)

	for _, url := range serverURLs {
		auth := &supervisor.BasicAuth{
			Username: config.SupervisorUsername,
			Password: config.SupervisorPassword,
		}

		client, err := supervisor.NewClient(url, auth)
		if err != nil {
			log.Fatalf("Error creating supervisor client for %s: %v", url, err)
		}
		defer client.Close()
		supervisorClients[url] = client
	}

	bot, err := tgbotapi.NewBotAPI(config.TelegramBotToken)
	if err != nil {
		log.Fatalf("Error creating Telegram bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	handler := tgbot.NewHandler(bot, supervisorClients)

	// handler.ShowAllProcesses(config.TelegramChatID)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go handler.HandleUpdates()

	for range ticker.C {
		handler.CheckProcessStatuses()
	}
}
