package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var (
	ServerURLs         string
	TelegramBotToken   string
	ProcessesPerPage   int
	AllowedChatIDs     []int64
	SupervisorUsername string
	SupervisorPassword string
	DBUser             string
	DBPassword         string
	DBName             string
	DBHost             string
	DBPort             string
)

func init() {
	err := godotenv.Load(".env.dev")
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
	}

	ServerURLs = getEnv("SERVER_URLS", "")
	TelegramBotToken = getEnv("TELEGRAM_BOT_TOKEN", "")
	ProcessesPerPage = getEnvAsInt("PROCESSES_PER_PAGE", 5)

	allowedChatIDsStr := getEnv("ALLOWED_CHAT_IDS", "")
	if allowedChatIDsStr != "" {
		idStrings := strings.Split(allowedChatIDsStr, ",")
		for _, idStr := range idStrings {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil {
				AllowedChatIDs = append(AllowedChatIDs, id)
			} else {
				log.Printf("Warning: Invalid chat ID in ALLOWED_CHAT_IDS: %s", idStr)
			}
		}
	}
	if len(AllowedChatIDs) == 0 {
		log.Println("Warning: No valid chat IDs configured in ALLOWED_CHAT_IDS")
	}

	SupervisorUsername = getEnv("SUPERVISOR_USERNAME", "")
	SupervisorPassword = getEnv("SUPERVISOR_PASSWORD", "")
	DBUser = getEnv("DB_USER", "youruser")
	DBPassword = getEnv("DB_PASSWORD", "yourpassword")
	DBName = getEnv("DB_NAME", "yourdb")
	DBHost = getEnv("DB_HOST", "localhost")
	DBPort = getEnv("DB_PORT", "5432")
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

func IsAllowedChatID(chatID int64) bool {
	for _, id := range AllowedChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}
