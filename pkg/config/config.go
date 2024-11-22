package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

var (
	ServerURLs         string
	TelegramBotToken   string
	ProcessesPerPage   int
	TelegramChatID     int64
	SupervisorUsername string
	SupervisorPassword string
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
	}

	ServerURLs = getEnv("SERVER_URLS", "")
	TelegramBotToken = getEnv("TELEGRAM_BOT_TOKEN", "")
	ProcessesPerPage = getEnvAsInt("PROCESSES_PER_PAGE", 5)
	TelegramChatID = getEnvAsInt64("TELEGRAM_CHAT_ID", 0)
	SupervisorUsername = getEnv("SUPERVISOR_USERNAME", "")
	SupervisorPassword = getEnv("SUPERVISOR_PASSWORD", "")
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(name string, defaultValue int) int {
	valueStr := getEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsInt64(name string, defaultValue int64) int64 {
	valueStr := getEnv(name, "")
	if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return value
	}
	return defaultValue
}
