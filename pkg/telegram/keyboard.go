package telegram

import (
	"fmt"
	"net/url"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/k0kubun/pp"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
	"github.com/rarebek/supervisor-tg-notifier/pkg/utils"
)

type PaginatedKeyboard struct {
	CurrentPage int
	TotalPages  int
	Keyboard    [][]tgbotapi.InlineKeyboardButton
}

func BuildProcessControlKeyboard(processName, serverURL string) tgbotapi.InlineKeyboardMarkup {
	// Get short ID for server URL
	shortID := utils.GetShortServerId(serverURL)
	pp.Println("keyboard text is: ", "start_"+processName+"_"+shortID)
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"🚀 Start",
				fmt.Sprintf("start_%s_%s", processName, shortID),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				"🛑 Stop",
				fmt.Sprintf("stop_%s_%s", processName, shortID),
			),
		),
	)
}

func BuildPaginatedKeyboard(processes []models.Process, page int) tgbotapi.InlineKeyboardMarkup {
	totalProcesses := len(processes)
	totalPages := (totalProcesses + config.ProcessesPerPage - 1) / config.ProcessesPerPage

	start := (page - 1) * config.ProcessesPerPage
	end := start + config.ProcessesPerPage
	if end > totalProcesses {
		end = totalProcesses
	}

	keyboard := [][]tgbotapi.InlineKeyboardButton{}

	for i := start; i < end; i++ {
		process := processes[i]
		buttonLabel := fmt.Sprintf("🔍 %s", EscapeMarkdownV2(process.Name))
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonLabel, fmt.Sprintf("details_%s_%s", process.Name, url.QueryEscape(process.ServerURL))),
		))
	}

	navRow := []tgbotapi.InlineKeyboardButton{}
	if page > 1 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Previous Page", fmt.Sprintf("page_%d", page-1)))
	}
	if page < totalPages {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Next Page ➡️", fmt.Sprintf("page_%d", page+1)))
	}
	if len(navRow) > 0 {
		keyboard = append(keyboard, navRow)
	}

	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

func BuildAllProcessesKeyboard(processes []models.Process) tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton

	for i := 0; i < len(processes); i += 2 {
		var row []tgbotapi.InlineKeyboardButton

		process := processes[i]
		row = append(row,
			tgbotapi.NewInlineKeyboardButtonData(
				"🔍 "+EscapeMarkdownV2(process.Name),
				fmt.Sprintf("details_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				"🚀",
				fmt.Sprintf("start_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				"🛑",
				fmt.Sprintf("stop_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
			),
		)

		if i+1 < len(processes) {
			process = processes[i+1]
			row = append(row,
				tgbotapi.NewInlineKeyboardButtonData(
					"🔍 "+EscapeMarkdownV2(process.Name),
					fmt.Sprintf("details_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
				),
				tgbotapi.NewInlineKeyboardButtonData(
					"🚀",
					fmt.Sprintf("start_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
				),
				tgbotapi.NewInlineKeyboardButtonData(
					"🛑",
					fmt.Sprintf("stop_%s_%s", process.Name, url.QueryEscape(process.ServerURL)),
				),
			)
		}

		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Paginated View", "page_1"),
		tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "show_all"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

func ShowAllProcessKeyboard(processes []models.Process) tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton

	for _, process := range processes {
		// Get short ID for server URL
		shortID := utils.GetShortServerId(process.ServerURL)

		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🚀 %s", EscapeMarkdownV2(process.Name)),
				fmt.Sprintf("start_%s_%s", process.Name, shortID), // Add server shortID
			),
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🛑 %s", EscapeMarkdownV2(process.Name)),
				fmt.Sprintf("stop_%s_%s", process.Name, shortID), // Add server shortID
			),
		))
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Paginated View", "page_1"),
		tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "show_all"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

func ChooseProcessesKeyboard(processes []models.Process) tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton

	for _, process := range processes {
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(process.Name, process.Name),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}
