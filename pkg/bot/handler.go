package bot

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
	"github.com/rarebek/supervisor-tg-notifier/pkg/telegram"
)

type Handler struct {
	bot                  *tgbotapi.BotAPI
	supervisorClient     *supervisor.Client
	userStoppedProcesses map[string]struct{}
	previousStatus       map[string]string
}

func NewHandler(bot *tgbotapi.BotAPI, supervisorClient *supervisor.Client) *Handler {
	return &Handler{
		bot:                  bot,
		supervisorClient:     supervisorClient,
		userStoppedProcesses: make(map[string]struct{}),
		previousStatus:       make(map[string]string),
	}
}

func (h *Handler) HandleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := h.bot.GetUpdatesChan(u)

	h.ListProcesses(config.TelegramChatID, 1)

	for update := range updates {
		if update.CallbackQuery != nil {
			h.handleCallbackQuery(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		h.handleMessage(update.Message)
	}
}

func (h *Handler) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	data := query.Data
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	switch {
	case strings.HasPrefix(data, "start_"):
		processName := strings.TrimPrefix(data, "start_")
		h.StartProcess(chatID, processName)
		h.ShowProcessDetails(chatID, processName)

	case strings.HasPrefix(data, "stop_"):
		processName := strings.TrimPrefix(data, "stop_")
		h.StopProcess(chatID, processName)
		h.userStoppedProcesses[processName] = struct{}{}
		h.ShowProcessDetails(chatID, processName)

	case data == "show_all":
		h.RefreshAllProcesses(chatID, messageID)

	case strings.HasPrefix(data, "details_"):
		processName := strings.TrimPrefix(data, "details_")
		h.ShowProcessDetails(chatID, processName)

	case strings.HasPrefix(data, "page_"):
		if page, err := strconv.Atoi(strings.TrimPrefix(data, "page_")); err == nil {
			h.ListProcesses(chatID, page)
		}
	}

	callback := tgbotapi.NewCallback(query.ID, "")
	h.bot.Send(callback)
}

func (h *Handler) handleMessage(message *tgbotapi.Message) {
	text := message.Text
	chatID := message.Chat.ID

	log.Printf("Received message from Telegram: %s", text)

	switch {
	case text == "List Processes":
		h.ListProcesses(chatID, 1)

	case strings.HasPrefix(text, "Start "):
		processName := strings.TrimSpace(strings.TrimPrefix(text, "Start "))
		h.StartProcess(chatID, processName)

	case text == "Show All" || text == "/all":
		h.ShowAllProcesses(chatID)

	case strings.HasPrefix(text, "Stop "):
		processName := strings.TrimSpace(strings.TrimPrefix(text, "Stop "))
		h.StopProcess(chatID, processName)
	}
}

func (h *Handler) ShowProcessDetails(chatID int64, processName string) {
	processes, err := h.supervisorClient.GetAllProcesses()
	if err != nil {
		log.Printf("Error getting process info: %v", err)
		return
	}

	for _, process := range processes {
		if process.Name == processName {
			message := telegram.FormatProcessDetails(process)
			keyboard := telegram.BuildProcessControlKeyboard(processName)

			showAllButton := tgbotapi.NewInlineKeyboardButtonData("Show All", "show_all")
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{showAllButton})

			err = telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
			if err != nil {
				log.Printf("Error sending process details: %v", err)
			}
			return
		}
	}
}

func (h *Handler) ListProcesses(chatID int64, page int) {
	processes, err := h.supervisorClient.GetAllProcesses()
	if err != nil {
		log.Printf("Error getting processes: %v", err)
		return
	}

	totalPages := (len(processes) + config.ProcessesPerPage - 1) / config.ProcessesPerPage
	keyboard := telegram.BuildPaginatedKeyboard(processes, page)
	message := telegram.FormatProcessList(processes, page, totalPages)

	err = telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
	if err != nil {
		log.Printf("Error sending processes list to Telegram: %v", err)
	} else {
		log.Printf("Processes list sent to Telegram")
	}
}

func (h *Handler) StartProcess(chatID int64, processName string) {
	err := h.supervisorClient.StartProcess(processName)
	if err != nil {
		log.Printf("Error starting process %s: %v", processName, err)
		return
	}

	err = telegram.SendStatusMessage(h.bot, chatID, processName, "started")
	if err != nil {
		log.Printf("Error sending start process message to Telegram: %v", err)
	}
}

func (h *Handler) StopProcess(chatID int64, processName string) {
	err := h.supervisorClient.StopProcess(processName)
	if err != nil {
		log.Printf("Error stopping process %s: %v", processName, err)
		return
	}

	err = telegram.SendStatusMessage(h.bot, chatID, processName, "stopped")
	if err != nil {
		log.Printf("Error sending stop process message to Telegram: %v", err)
	}
}

func (h *Handler) CheckProcessStatuses() {
	processes, err := h.supervisorClient.GetAllProcesses()
	if err != nil {
		log.Printf("Error getting processes: %v", err)
		return
	}

	message := "*Processes Status:*\n"
	notify := false
	var keyboard [][]tgbotapi.InlineKeyboardButton

	for _, process := range processes {
		prev, exists := h.previousStatus[process.Name]

		if exists && prev == "RUNNING" && process.State != "RUNNING" {
			if _, isUserStopped := h.userStoppedProcesses[process.Name]; isUserStopped {
				delete(h.userStoppedProcesses, process.Name)
				h.previousStatus[process.Name] = process.State
				continue
			}

			notify = true
			message += telegram.FormatProcessStatusChange(process)
			keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Start "+process.Name, "start_"+process.Name),
				tgbotapi.NewInlineKeyboardButtonData("Stop "+process.Name, "stop_"+process.Name),
			))
		}

		h.previousStatus[process.Name] = process.State
	}

	if notify {
		markup := tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		err = telegram.SendToTelegramWithInlineKeyboard(h.bot, config.TelegramChatID, message, markup)
		if err != nil {
			log.Printf("Error sending notification to Telegram: %v", err)
		} else {
			log.Printf("Notification sent to Telegram")
		}
	}
}

func (h *Handler) ShowAllProcesses(chatID int64) {
	processes, err := h.supervisorClient.GetAllProcesses()
	if err != nil {
		log.Printf("Error getting all processes: %v", err)
		message := fmt.Sprintf("Error fetching processes: `%s`", telegram.EscapeMarkdownV2(err.Error()))
		telegram.SendToTelegram(h.bot, chatID, message)
		return
	}

	sort.Slice(processes, func(i, j int) bool {
		if processes[i].State == processes[j].State {
			return processes[i].Name < processes[j].Name
		}
		if processes[i].State == "RUNNING" {
			return true
		}
		if processes[j].State == "RUNNING" {
			return false
		}
		return processes[i].State < processes[j].State
	})

	stateGroups := make(map[string][]models.Process)
	for _, process := range processes {
		stateGroups[process.State] = append(stateGroups[process.State], process)
	}

	var summary strings.Builder
	summary.WriteString("*Process Status Summary*\n")
	for state, procs := range stateGroups {
		escapedState := telegram.EscapeMarkdownV2(state)
		summary.WriteString(fmt.Sprintf("• %s: `%d`\n", escapedState, len(procs)))
	}
	summary.WriteString("\n")

	message := summary.String() + telegram.FormatAllProcessesList(processes)
	keyboard := telegram.BuildAllProcessesKeyboard(processes)

	err = telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
	if err != nil {
		log.Printf("Error sending all processes list to Telegram: %v", err)
	} else {
		log.Printf("All processes list sent to Telegram")
	}
}

func (h *Handler) RefreshAllProcesses(chatID int64, messageID int) {
	processes, err := h.supervisorClient.GetAllProcesses()
	if err != nil {
		log.Printf("Error getting all processes: %v", err)
		message := fmt.Sprintf("Error fetching processes: `%s`", telegram.EscapeMarkdownV2(err.Error()))
		telegram.SendToTelegram(h.bot, chatID, message)
		return
	}

	sort.Slice(processes, func(i, j int) bool {
		if processes[i].State == processes[j].State {
			return processes[i].Name < processes[j].Name
		}
		if processes[i].State == "RUNNING" {
			return true
		}
		if processes[j].State == "RUNNING" {
			return false
		}
		return processes[i].State < processes[j].State
	})

	stateGroups := make(map[string][]models.Process)
	for _, process := range processes {
		stateGroups[process.State] = append(stateGroups[process.State], process)
	}

	var summary strings.Builder
	summary.WriteString("*Process Status Summary*\n")
	for state, procs := range stateGroups {
		escapedState := telegram.EscapeMarkdownV2(state)
		summary.WriteString(fmt.Sprintf("• %s: `%d`\n", escapedState, len(procs)))
	}
	summary.WriteString("\n")

	message := summary.String() + telegram.FormatAllProcessesList(processes)
	keyboard := telegram.BuildAllProcessesKeyboard(processes)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, message)
	editMsg.ParseMode = "MarkdownV2"
	editMsg.ReplyMarkup = &keyboard

	_, err = h.bot.Send(editMsg)
	if err != nil {
		log.Printf("Error editing message with all processes list: %v", err)
	} else {
		log.Printf("All processes list edited in Telegram")
	}
}
