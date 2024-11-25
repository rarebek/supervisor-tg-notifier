package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	tgbot "github.com/rarebek/supervisor-tg-notifier/pkg/bot"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
)

func main() {
	dbInfo := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		config.DBUser, config.DBPassword, config.DBName, config.DBHost, config.DBPort)
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	defer db.Close()

	if err := runMigrations(db); err != nil {
		log.Fatalf("Error running migrations: %v", err)
	}

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

	handler := tgbot.NewHandler(bot, supervisorClients, db)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go handler.HandleUpdates()

	for range ticker.C {
		handler.CheckProcessStatuses()
	}
}

func runMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("could not run up migrations: %w", err)
	}

	return nil
}
