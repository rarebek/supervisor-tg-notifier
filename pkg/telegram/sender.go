package telegram

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendToTelegram(bot *tgbotapi.BotAPI, chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "MarkdownV2"
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message to Telegram: %v", err)
	} else {
		log.Printf("Message sent to Telegram: %s", message)
	}
	return err
}

func SendStatusMessage(bot *tgbotapi.BotAPI, chatID int64, processName, action string) error {
	escapedName := EscapeMarkdownV2(processName)
	message := fmt.Sprintf("*Process* `%s` *%s successfully\\.*", escapedName, action)
	return SendToTelegram(bot, chatID, message)
}

func SendToTelegramWithInlineKeyboard(bot *tgbotapi.BotAPI, chatID int64, message string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = keyboard
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message with inline keyboard to Telegram: %v", err)
	} else {
		log.Printf("Message with inline keyboard sent to Telegram: %s", message)
	}
	return err
}
