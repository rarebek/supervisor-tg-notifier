package bot

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/k0kubun/pp"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/logger"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
	"github.com/rarebek/supervisor-tg-notifier/pkg/telegram"
	"github.com/rarebek/supervisor-tg-notifier/pkg/utils"
)

var processPath string

type Handler struct {
	bot                  *tgbotapi.BotAPI
	supervisorClients    map[string]*supervisor.Client
	userStoppedProcesses map[string]bool
	previousStatus       map[string]string
}

func NewHandler(bot *tgbotapi.BotAPI, supervisorClients map[string]*supervisor.Client) *Handler {
	return &Handler{
		bot:                  bot,
		supervisorClients:    supervisorClients,
		userStoppedProcesses: make(map[string]bool),
		previousStatus:       make(map[string]string),
	}
}

func (h *Handler) HandleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := h.bot.GetUpdatesChan(u)

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
	case strings.HasPrefix(data, "details_"):
		trimmedData := strings.TrimPrefix(data, "details_")
		parts := strings.Split(trimmedData, "_")
		if len(parts) >= 2 {
			serverID := parts[len(parts)-1]
			processName := strings.Join(parts[:len(parts)-1], "_")

			serverURL, _ := url.QueryUnescape(serverID)
			if client, ok := h.supervisorClients[serverURL]; ok {
				processes, err := client.GetAllProcesses()
				if err != nil {
					log.Printf("Error getting process info: %v", err)
					return
				}

				for _, process := range processes {
					if process.Name == processName {
						process.ServerURL = serverURL
						message := telegram.FormatProcessDetails(process)
						keyboard := telegram.BuildProcessControlKeyboard(process.Name, serverURL)

						editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, message, keyboard)
						editMsg.ParseMode = "MarkdownV2"

						if _, err := h.bot.Send(editMsg); err != nil {
							log.Printf("Error updating message: %v", err)
						}

						break
					}
				}
			}
		}

	case strings.HasPrefix(data, "start_"), strings.HasPrefix(data, "stop_"):
		trimmedData := strings.TrimPrefix(data, strings.Split(data, "_")[0]+"_")
		lastUnderscoreIndex := strings.LastIndex(trimmedData, "_")
		if lastUnderscoreIndex != -1 {
			processName := trimmedData[:lastUnderscoreIndex]
			shortID := trimmedData[lastUnderscoreIndex+1:]

			var serverURL string
			for url := range h.supervisorClients {
				if utils.GetShortServerId(url) == shortID {
					serverURL = url
					break
				}
			}

			if serverURL != "" {
				log.Printf("Found server URL: %s for short ID: %s", serverURL, shortID)
				if client, ok := h.supervisorClients[serverURL]; ok {
					processes, err := client.GetAllProcesses()
					if err != nil {
						telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error getting process info: `%s`", telegram.EscapeMarkdownV2(err.Error())))
						log.Printf("Error getting process info from server %s: %v", serverURL, err)
						return
					}
					for _, process := range processes {
						if process.Name == processName {
							break
						}
					}

					if processName == "" {
						telegram.SendToTelegram(h.bot, chatID, "Process not found")
						log.Printf("Process name is empty after processing data: %s", data)
						return
					}

					switch {
					case strings.HasPrefix(data, "start_"):
						if err := client.StartProcess(processName); err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error starting process: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							log.Printf("Error starting process %s on server %s: %v", processName, serverURL, err)
							return
						}

						processInfo, err := client.GetProcessInfo(processName)
						if err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error retrieving process info: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							log.Printf("Error retrieving process info for %s on server %s: %v", processName, serverURL, err)
							return
						}

						processInfo.ServerURL = serverURL
						message := telegram.FormatProcessDetails(processInfo)
						keyboard := telegram.BuildProcessControlKeyboard(processName, serverURL)
						editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, message, keyboard)
						editMsg.ParseMode = "MarkdownV2"
						h.bot.Send(editMsg)
						log.Printf("Started process %s on server %s and updated message", processName, serverURL)

					case strings.HasPrefix(data, "stop_"):
						if err := client.StopProcess(processName); err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error stopping process: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							log.Printf("Error stopping process %s on server %s: %v", processName, serverURL, err)
							return
						}

						processInfo, err := client.GetProcessInfo(processName)
						if err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error retrieving process info: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							log.Printf("Error retrieving process info for %s on server %s: %v", processName, serverURL, err)
							return
						}

						processInfo.ServerURL = serverURL
						message := telegram.FormatProcessDetails(processInfo)
						keyboard := telegram.BuildProcessControlKeyboard(processName, serverURL)

						editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, message, keyboard)
						editMsg.ParseMode = "MarkdownV2"
						h.bot.Send(editMsg)
						h.userStoppedProcesses[serverURL+":"+processInfo.Name] = true
					}
				}
			} else {
				pp.Println("server url not found in given process")
			}
		}
	}

	callback := tgbotapi.NewCallback(query.ID, "")
	if _, err := h.bot.Request(callback); err != nil {
		log.Printf("Error acknowledging callback: %v", err)
	}
}

func (h *Handler) handleMessage(message *tgbotapi.Message) {
	text := message.Text
	chatID := message.Chat.ID

	switch {
	case strings.HasPrefix(text, "View "):
		processName := strings.TrimPrefix(text, "View ")
		processName = strings.ReplaceAll(processName, "\\_", "_")
		h.ShowProcessDetails(chatID, processName)

	case text == "/start" || text == "/help":
		h.ShowAllProcesses(chatID)

	default:
		telegram.SendToTelegram(h.bot, chatID, "Unknown command. Use /start or /help to see available commands.")
	}
}

func (h *Handler) ShowProcessDetails(chatID int64, processName string) {
	var foundProcesses []models.Process

	for serverURL, client := range h.supervisorClients {
		processes, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting process info from %s: %v", serverURL, err)
			continue
		}

		for _, process := range processes {
			if process.Group+":"+process.Name == processName {
				process.ServerURL = serverURL
				foundProcesses = append(foundProcesses, process)
			}
		}
	}
	if len(foundProcesses) == 0 {
		telegram.SendToTelegram(h.bot, chatID, "Process not found")
		return
	}

	if len(foundProcesses) == 1 {
		process := foundProcesses[0]
		message := telegram.FormatProcessDetails(process)
		message += fmt.Sprintf("\n*Server:* `%s`", telegram.EscapeMarkdownV2(process.ServerURL))
		keyboard := telegram.BuildProcessControlKeyboard(process.Name, process.ServerURL)
		telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
		return
	}

	var message strings.Builder
	var keyboard [][]tgbotapi.InlineKeyboardButton
	message.WriteString("*Multiple processes found with same name:*\n\n")

	for _, process := range foundProcesses {
		serverName := telegram.EscapeMarkdownV2(process.ServerURL)
		message.WriteString(fmt.Sprintf("• Server: `%s`\n  Status: `%s`\n\n", serverName, process.State))

		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("View on %s", serverName),
				fmt.Sprintf("details_%s_%s", telegram.EscapeMarkdownV2(process.Name), url.QueryEscape(process.ServerURL)),
			),
		))
	}

	telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message.String(), tgbotapi.NewInlineKeyboardMarkup(keyboard...))
}

func (h *Handler) CheckProcessStatuses() {
	for clientURL, client := range h.supervisorClients {
		processes, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting processes from %s: %v", clientURL, err)
			continue
		}

		for _, process := range processes {
			logger.Log("debug", "Process", process.Name, "on", clientURL, ":", process.State)
			processKey := clientURL + ":" + process.Name
			prev, exists := h.previousStatus[processKey]
			if !exists {
				h.previousStatus[processKey] = process.State
				continue
			}

			if prev == "RUNNING" && process.State != "RUNNING" {
				_, isUserStopped := h.userStoppedProcesses[processKey]
				if !isUserStopped {
					message := "*Processes Status:*\n"
					message += fmt.Sprintf("*Server:* `%s`\n", telegram.EscapeMarkdownV2(clientURL))
					message += telegram.FormatProcessStatusChange(process)

					shortID := utils.GetShortServerId(clientURL)

					keyboard := [][]tgbotapi.InlineKeyboardButton{
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData(
								"🚀 Start",
								fmt.Sprintf("start_%s_%s", process.Group+":"+process.Name, shortID),
							),
							tgbotapi.NewInlineKeyboardButtonData(
								"🛑 Stop",
								fmt.Sprintf("stop_%s_%s", process.Group+":"+process.Name, shortID),
							),
						),
					}
					markup := tgbotapi.NewInlineKeyboardMarkup(keyboard...)
					if err := telegram.SendToTelegramWithInlineKeyboard(h.bot, config.TelegramChatID, message, markup); err != nil {
						log.Printf("Error sending notification to Telegram: %v", err)
					}
				} else {
					delete(h.userStoppedProcesses, processKey)
				}
			}

			h.previousStatus[processKey] = process.State
		}
	}
}

func (h *Handler) ShowAllProcesses(chatID int64) {
	var summary strings.Builder
	var keyboard [][]tgbotapi.KeyboardButton

	summary.WriteString("*📊 Process Status Summary*\n\n")

	for clientID, client := range h.supervisorClients {
		clientProcesses, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting all processes from %s: %v", clientID, err)
			message := fmt.Sprintf("⚠️ Error fetching processes from %s: `%s`", clientID, telegram.EscapeMarkdownV2(err.Error()))
			telegram.SendToTelegram(h.bot, chatID, message)
			continue
		}

		for i := range clientProcesses {
			clientProcesses[i].ServerURL = clientID
		}

		processByGroup := make(map[string][]models.Process)
		for _, p := range clientProcesses {
			if p.Group != "" {
				processByGroup[p.Group] = append(processByGroup[p.Group], p)
			} else {
				processByGroup[""] = append(processByGroup[""], p)
			}
		}

		summary.WriteString(fmt.Sprintf("🖥️ *Server: %s*\n", telegram.EscapeMarkdownV2(clientID)))
		summary.WriteString("━━━━━━━━━━━━━━\n")

		for groupName, groupProcesses := range processByGroup {
			if groupName != "" {
				summary.WriteString(fmt.Sprintf("\n📦 *Group: %s*\n", telegram.EscapeMarkdownV2(groupName)))
			}

			processByStatus := make(map[string][]models.Process)
			for _, p := range groupProcesses {
				processByStatus[p.State] = append(processByStatus[p.State], p)
			}

			for status, processes := range processByStatus {
				statusIcon := getStatusIcon(status)
				summary.WriteString(fmt.Sprintf("\n%s *%s*\n", statusIcon, telegram.EscapeMarkdownV2(status)))

				for _, process := range processes {
					fullProcessName := process.Name
					if groupName != "" {
						fullProcessName = groupName + ":" + process.Name
					}

					escapedName := telegram.EscapeMarkdownV2(fullProcessName)
					summary.WriteString(fmt.Sprintf("  • %s\n", escapedName))

					keyboard = append(keyboard, tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton(fmt.Sprintf("View %s", escapedName)),
					))
				}
			}
			summary.WriteString("\n")
		}
		summary.WriteString("\n")
	}

	summary.WriteString("\n💡 *Tips:*\n")
	summary.WriteString("• Click on process name to view details\n")
	summary.WriteString("• Use /help to see all commands\n")

	message := summary.String()
	replyKeyboard := tgbotapi.NewReplyKeyboard(keyboard...)
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = replyKeyboard

	err := telegram.SendToTelegramWithReplyKeyboard(h.bot, chatID, msg)
	if err != nil {
		log.Printf("Error sending all processes list to Telegram: %v", err)
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "RUNNING":
		return "🟢"
	case "STOPPED":
		return "🔴"
	case "STARTING":
		return "🟡"
	case "STOPPING":
		return "🟠"
	case "FATAL":
		return "⚠️"
	default:
		return "❓"
	}
}

func (h *Handler) RefreshAllProcesses(chatID int64, messageID int) {
	var processes []models.Process

	for serverURL, client := range h.supervisorClients {
		clientProcesses, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting all processes: %v", err)
			message := fmt.Sprintf("Error fetching processes: `%s`", telegram.EscapeMarkdownV2(err.Error()))
			telegram.SendToTelegram(h.bot, chatID, message)
			continue
		}
		for i := range clientProcesses {
			clientProcesses[i].ServerURL = serverURL
		}
		processes = append(processes, clientProcesses...)
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

	_, err := h.bot.Send(editMsg)
	if err != nil {
		log.Printf("Error editing message with all processes list: %v", err)
	} else {
		log.Printf("All processes list edited in Telegram")
	}
}
