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

type Handler struct {
	bot                  *tgbotapi.BotAPI
	supervisorClients    map[string]*supervisor.Client
	userStoppedProcesses map[string]struct{}
	previousStatus       map[string]string
}

func NewHandler(bot *tgbotapi.BotAPI, supervisorClients map[string]*supervisor.Client) *Handler {
	return &Handler{
		bot:                  bot,
		supervisorClients:    supervisorClients,
		userStoppedProcesses: make(map[string]struct{}),
		previousStatus:       make(map[string]string),
	}
}

func (h *Handler) HandleUpdates() {
	// Set up update config with 60 second timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get update channel
	updates := h.bot.GetUpdatesChan(u)

	// Process updates in infinite loop
	for update := range updates {
		// Handle callback queries (button clicks)
		if update.CallbackQuery != nil {
			h.handleCallbackQuery(update.CallbackQuery)
			continue
		}

		// Handle text messages
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
			// Last part is always the server ID
			serverID := parts[len(parts)-1]
			// All parts except last one form the process name
			processName := strings.Join(parts[:len(parts)-1], "_")

			// Find server URL from ID
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
		// Remove the prefix (start_ or stop_)
		trimmedData := strings.TrimPrefix(data, strings.Split(data, "_")[0]+"_")
		// Find the last underscore to split process name and server ID
		lastUnderscoreIndex := strings.LastIndex(trimmedData, "_")
		if lastUnderscoreIndex != -1 {
			processName := trimmedData[:lastUnderscoreIndex]
			shortID := trimmedData[lastUnderscoreIndex+1:]

			// Find the full server URL from short ID
			var serverURL string
			for url := range h.supervisorClients {
				if utils.GetShortServerId(url) == shortID {
					serverURL = url
					break
				}
			}
			pp.Println("serverURL in control: ", serverURL)

			if serverURL != "" {
				if client, ok := h.supervisorClients[serverURL]; ok {
					switch {
					case strings.HasPrefix(data, "start_"):
						pp.Println(data)
						if err := client.StartProcess(processName); err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error starting process: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							return
						}
						processes, _ := client.GetAllProcesses()
						for _, process := range processes {
							if process.Name == processName {
								process.ServerURL = serverURL
								message := telegram.FormatProcessDetails(process)
								keyboard := telegram.BuildProcessControlKeyboard(process.Name, serverURL)

								editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, message, keyboard)
								editMsg.ParseMode = "MarkdownV2"
								h.bot.Send(editMsg)
								break
							}
						}

					case strings.HasPrefix(data, "stop_"):
						// Set user-stopped flag before stopping
						processKey := serverURL + ":" + processName
						h.userStoppedProcesses[processKey] = struct{}{}

						if err := client.StopProcess(processName); err != nil {
							telegram.SendToTelegram(h.bot, chatID, fmt.Sprintf("Error stopping process: `%s`", telegram.EscapeMarkdownV2(err.Error())))
							return
						}
						// Get updated process details from only this server
						processes, _ := client.GetAllProcesses()
						for _, process := range processes {
							if process.Name == processName {
								process.ServerURL = serverURL
								message := telegram.FormatProcessDetails(process)
								keyboard := telegram.BuildProcessControlKeyboard(process.Name, serverURL)
								editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, message, keyboard)
								editMsg.ParseMode = "MarkdownV2"
								h.bot.Send(editMsg)
								break
							}
						}
					}
				}
			}
		}
	}

	// Always acknowledge the callback
	callback := tgbotapi.NewCallback(query.ID, "")
	if _, err := h.bot.Request(callback); err != nil {
		log.Printf("Error acknowledging callback: %v", err)
	}
}

func (h *Handler) handleMessage(message *tgbotapi.Message) {
	text := message.Text
	chatID := message.Chat.ID

	switch {
	case strings.HasPrefix(text, "/view_"):
		// Split keeping all parts
		allParts := strings.Split(strings.TrimPrefix(text, "/view_"), "_")
		if len(allParts) >= 2 {
			// Last part is always the shortID
			shortID := allParts[len(allParts)-1]
			// Combine all parts except last one to get full process name
			processName := strings.Join(allParts[:len(allParts)-1], "_")

			// Find the full server URL from short ID
			var serverURL string
			for url := range h.supervisorClients {
				if utils.GetShortServerId(url) == shortID {
					serverURL = url
					break
				}
			}

			if serverURL != "" {
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
							telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
							return
						}
					}
				}
			}
		}

	case text == "/start" || text == "/help":
		h.ShowAllProcesses(chatID)
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
			// Get last part after underscore for comparison
			parts := strings.Split(process.Name, "_")
			normalizedName := parts[len(parts)-1]
			// Compare with the last part of input process name
			inputParts := strings.Split(processName, "_")
			inputNormalized := inputParts[len(inputParts)-1]

			if normalizedName == inputNormalized {
				process.ServerURL = serverURL
				foundProcesses = append(foundProcesses, process)
			}
		}
	}
	if len(foundProcesses) == 0 {
		telegram.SendToTelegram(h.bot, chatID, "Process not found")
		return
	}

	// If only one process found, show details directly
	if len(foundProcesses) == 1 {
		process := foundProcesses[0]
		process.Name = strings.ReplaceAll(process.Name, "_", "")
		message := telegram.FormatProcessDetails(process)
		keyboard := telegram.BuildProcessControlKeyboard(process.Name, process.ServerURL)
		telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
		return
	}

	// If multiple processes found, show selection menu
	var message strings.Builder
	var keyboard [][]tgbotapi.InlineKeyboardButton
	message.WriteString("*Multiple processes found with same name:*\n\n")

	for _, process := range foundProcesses {
		serverName := telegram.EscapeMarkdownV2(process.ServerURL)
		message.WriteString(fmt.Sprintf("• Server: `%s`\n  Status: `%s`\n\n", serverName, process.State))

		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("View on %s", serverName),
				fmt.Sprintf("details_%s_%s", telegram.EscapeMarkdownV2(strings.ReplaceAll(process.Name, "_", "")), url.QueryEscape(process.ServerURL)),
			),
		))
	}

	telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message.String(), tgbotapi.NewInlineKeyboardMarkup(keyboard...))
}

// func (h *Handler) ListProcesses(chatID int64, page int, specificClient *supervisor.Client) {
// 	var processes []models.Process
// 	var err error

// 	if specificClient != nil {
// 		processes, err = specificClient.GetAllProcesses()
// 	} else {
// 		for _, client := range h.supervisorClients {
// 			clientProcesses, clientErr := client.GetAllProcesses()
// 			if clientErr != nil {
// 				log.Printf("Error getting processes: %v", clientErr)
// 				continue
// 			}
// 			processes = append(processes, clientProcesses...)
// 		}
// 	}

// 	if err != nil {
// 		log.Printf("Error getting processes: %v", err)
// 		return
// 	}

// 	totalPages := (len(processes) + config.ProcessesPerPage - 1) / config.ProcessesPerPage
// 	keyboard := telegram.BuildPaginatedKeyboard(processes, page)
// 	message := telegram.FormatProcessList(processes, page, totalPages)

//		err = telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, keyboard)
//		if err != nil {
//			log.Printf("Error sending processes list to Telegram: %v", err)
//		} else {
//			log.Printf("Processes list sent to Telegram")
//		}
//	}
func (h *Handler) StartProcess(chatID int64, processPath string) {
	// Split group:process if present
	parts := strings.Split(processPath, ":")
	processName := processPath
	if len(parts) == 2 {
		// Use group:process format for supervisor
		processName = parts[0] + ":" + parts[1]
	}

	for _, client := range h.supervisorClients {
		err := client.StartProcess(processName)
		if err != nil {
			log.Printf("Error starting process %s: %v", processName, err)
			continue
		}
		err = telegram.SendStatusMessage(h.bot, chatID, processPath, "started")
		if err != nil {
			log.Printf("Error sending start process message to Telegram: %v", err)
		}
	}
}

func (h *Handler) StopProcess(chatID int64, processPath string) {
	// Split group:process if present
	parts := strings.Split(processPath, ":")
	processName := processPath
	if len(parts) == 2 {
		processName = parts[1] // Use just process name for actual stop
	}

	for _, client := range h.supervisorClients {
		err := client.StopProcess(processName)
		if err != nil {
			log.Printf("Error stopping process %s: %v", processName, err)
			continue
		}
		err = telegram.SendStatusMessage(h.bot, chatID, processPath, "stopped")
		if err != nil {
			log.Printf("Error sending stop process message to Telegram: %v", err)
		}
	}
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

			// Store initial status if not exists
			if !exists {
				h.previousStatus[processKey] = process.State
				continue
			}

			// Check for status change from RUNNING to non-RUNNING
			if prev == "RUNNING" && process.State != "RUNNING" {
				// Check if it was stopped by user
				if _, isUserStopped := h.userStoppedProcesses[processKey]; !isUserStopped {
					// Only notify if not user-stopped
					message := "*Processes Status:*\n"
					message += fmt.Sprintf("*Server:* `%s`\n", telegram.EscapeMarkdownV2(clientURL))
					message += telegram.FormatProcessStatusChange(process)
					keyboard := [][]tgbotapi.InlineKeyboardButton{
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData(
								"🚀 Start",
								fmt.Sprintf("start_%s_%s", process.Name, url.QueryEscape(clientURL)),
							),
							tgbotapi.NewInlineKeyboardButtonData(
								"🛑 Stop",
								fmt.Sprintf("stop_%s_%s", process.Name, url.QueryEscape(clientURL)),
							),
						),
					}
					markup := tgbotapi.NewInlineKeyboardMarkup(keyboard...)
					if err := telegram.SendToTelegramWithInlineKeyboard(h.bot, config.TelegramChatID, message, markup); err != nil {
						log.Printf("Error sending notification to Telegram: %v", err)
					}
				} else {
					// Clear user-stopped flag since status changed
					delete(h.userStoppedProcesses, processKey)
				}
			}

			// Update status for next check
			h.previousStatus[processKey] = process.State
		}
	}
}

func (h *Handler) ShowAllProcesses(chatID int64) {
	var summary strings.Builder
	summary.WriteString("*Process Status Summary*\n\n")

	for clientID, client := range h.supervisorClients {
		clientProcesses, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting all processes from %s: %v", clientID, err)
			message := fmt.Sprintf("Error fetching processes from %s: `%s`", clientID, telegram.EscapeMarkdownV2(err.Error()))
			telegram.SendToTelegram(h.bot, chatID, message)
			continue
		}

		// Set ServerURL for each process
		for i := range clientProcesses {
			clientProcesses[i].ServerURL = clientID
		}

		// Group by group name first
		processByGroup := make(map[string][]models.Process)
		for _, p := range clientProcesses {
			if p.Group != "" {
				processByGroup[p.Group] = append(processByGroup[p.Group], p)
			} else {
				// Handle ungrouped processes
				processByGroup[""] = append(processByGroup[""], p)
			}
		}

		summary.WriteString(fmt.Sprintf("*Client: %s*\n", telegram.EscapeMarkdownV2(clientID)))

		// Process each group
		for groupName, groupProcesses := range processByGroup {
			// Skip empty group name for ungrouped processes
			if groupName != "" && len(groupProcesses) > 1 {
				summary.WriteString(fmt.Sprintf("\n*Group: %s*\n", telegram.EscapeMarkdownV2(groupName)))
				// Add group control buttons
				summary.WriteString(fmt.Sprintf("\\[ [Start All](https://t.me/%s?start=g_start_%s_%s) | "+
					"[Stop All](https://t.me/%s?start=g_stop_%s_%s) \\]\n",
					h.bot.Self.UserName, url.QueryEscape(groupName), url.QueryEscape(utils.GetShortServerId(clientID)),
					h.bot.Self.UserName, url.QueryEscape(groupName), url.QueryEscape(utils.GetShortServerId(clientID))))
			}

			// Group by status within each group
			processByStatus := make(map[string][]models.Process)
			for _, p := range groupProcesses {
				processByStatus[p.State] = append(processByStatus[p.State], p)
			}

			// Show RUNNING processes
			if runningProcs, ok := processByStatus["RUNNING"]; ok {
				summary.WriteString("\n*🟢 RUNNING Processes:*\n")
				for _, process := range runningProcs {
					escapedName := telegram.EscapeMarkdownV2(process.Name)
					processPath := process.Name
					if groupName != "" {
						processPath = groupName + ":" + process.Name
					}
					command := fmt.Sprintf("/view\\_%s\\_%s",
						telegram.EscapeMarkdownV2(processPath),
						telegram.EscapeMarkdownV2(utils.GetShortServerId(clientID)))
					summary.WriteString(fmt.Sprintf("• %s \\[ %s \\]\n",
						escapedName,
						command,
					))
				}
			}

			// Show other statuses
			for status, processes := range processByStatus {
				if status == "RUNNING" {
					continue
				}
				summary.WriteString(fmt.Sprintf("\n*🔴 %s Processes:*\n", telegram.EscapeMarkdownV2(status)))
				for _, process := range processes {
					escapedName := telegram.EscapeMarkdownV2(process.Name)
					processPath := process.Name
					if groupName != "" {
						processPath = groupName + ":" + process.Name
					}
					summary.WriteString(fmt.Sprintf("• %s \\[ [View](https://t.me/%s?start=view_%s_%s) \\]\n",
						escapedName,
						h.bot.Self.UserName,
						url.QueryEscape(processPath),
						url.QueryEscape(utils.GetShortServerId(clientID)),
					))
				}
			}
			summary.WriteString("\n")
		}
	}

	message := summary.String()
	err := telegram.SendToTelegram(h.bot, chatID, message)
	if err != nil {
		log.Printf("Error sending all processes list to Telegram: %v", err)
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
		// Set ServerURL for each process
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
