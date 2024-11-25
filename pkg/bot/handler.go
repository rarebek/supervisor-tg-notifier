package bot

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/k0kubun/pp"
	"github.com/lib/pq"
	"github.com/rarebek/supervisor-tg-notifier/pkg/config"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
	"github.com/rarebek/supervisor-tg-notifier/pkg/supervisor"
	"github.com/rarebek/supervisor-tg-notifier/pkg/telegram"
	"github.com/rarebek/supervisor-tg-notifier/pkg/utils"
)

type Handler struct {
	bot                  *tgbotapi.BotAPI
	supervisorClients    map[string]*supervisor.Client
	userStoppedProcesses map[string]bool
	previousStatus       map[string]string
	db                   *sql.DB
}

func NewHandler(bot *tgbotapi.BotAPI, supervisorClients map[string]*supervisor.Client, db *sql.DB) *Handler {
	return &Handler{
		bot:                  bot,
		supervisorClients:    supervisorClients,
		userStoppedProcesses: make(map[string]bool),
		previousStatus:       make(map[string]string),
		db:                   db,
	}
}

func (h *Handler) HandleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := h.bot.GetUpdatesChan(u)

	for update := range updates {
		var chatID int64

		if update.CallbackQuery != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		} else if update.Message != nil {
			chatID = update.Message.Chat.ID
		} else {
			continue
		}

		// Check if user is authorized
		if !h.isAuthorizedUser(chatID) {
			if update.Message != nil {
				telegram.SendToTelegram(h.bot, chatID, "⚠️ You are not authorized to use this bot.")
			}
			continue
		}

		// Process authorized updates
		if update.CallbackQuery != nil {
			h.handleCallbackQuery(update.CallbackQuery)
			continue
		}

		if update.Message != nil {
			h.handleMessage(update.Message)
		}
	}
}

func (h *Handler) isAuthorizedUser(chatID int64) bool {
	if !config.IsAllowedChatID(chatID) {
		log.Printf("Unauthorized access attempt from chat ID: %d", chatID)
		return false
	}
	return true
}

func (h *Handler) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	data := query.Data
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	switch {
	case strings.HasPrefix(data, "choose_"):
		trimmedData := strings.TrimPrefix(data, "choose_")
		parts := strings.Split(trimmedData, "_")
		if len(parts) >= 2 {
			if err := h.handleChooseCallback(query, parts[0], parts[1]); err != nil {
				log.Printf("Error handling choose callback: %v", err)
				telegram.SendToTelegram(h.bot, query.Message.Chat.ID,
					fmt.Sprintf("Error updating preferences: `%s`",
						telegram.EscapeMarkdownV2(err.Error())))
			}
		}

	case data == "done_choosing":
		telegram.SendToTelegram(h.bot, chatID, "You have finished choosing processes. Your preferences have been saved.")
		// Show the user their selected preferences
		preferences, err := h.GetUserPreferences(chatID)
		if err != nil {
			log.Printf("Error retrieving user preferences: %v", err)
			telegram.SendToTelegram(h.bot, chatID, "Error retrieving your preferences.")
		} else {
			message := "Your selected processes:\n"
			for _, preference := range preferences {
				message += fmt.Sprintf("• %s\n", telegram.EscapeMarkdownV2(preference))
			}
			telegram.SendToTelegram(h.bot, chatID, message)
		}

	case strings.HasPrefix(data, "details_"):
		trimmedData := strings.TrimPrefix(data, "details_")
		parts := strings.Split(trimmedData, "_")
		if len(parts) >= 2 {
			serverID := parts[len(parts)-1]
			processName := strings.Join(parts[:len(parts)-1], "_")

			serverURL, _ := url.QueryUnescape(serverID)
			if client, ok := h.supervisorClients[serverURL]; ok {
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
						} else {
							// Mark process as manually stopped
							processKey := fmt.Sprintf("%s:%s", serverURL, processName)
							h.userStoppedProcesses[processKey] = true
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
	case text == "/notify":
		h.ShowNotifyOptions(chatID)

	case strings.HasPrefix(text, "View "):
		processName := strings.TrimPrefix(text, "View ")
		processName = strings.ReplaceAll(processName, "\\_", "_")
		h.ShowProcessDetails(chatID, processName)

	case text == "/start" || text == "/help":
		h.ShowAllProcesses(chatID)

	case strings.HasPrefix(text, "Notify "):
		processes := strings.Split(strings.TrimPrefix(text, "Notify "), ",")
		for i := range processes {
			processes[i] = strings.TrimSpace(processes[i])
		}
		if err := h.SaveUserPreferences(chatID, processes); err != nil {
			telegram.SendToTelegram(h.bot, chatID, "Error saving preferences")
			log.Printf("Error saving preferences for user %d: %v", chatID, err)
		} else {
			telegram.SendToTelegram(h.bot, chatID, "Preferences saved successfully")
		}

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
	log.Println("Starting process status check...")

	for clientURL, client := range h.supervisorClients {
		log.Printf("Checking processes for server: %s", clientURL)

		processes, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting processes from %s: %v", clientURL, err)
			continue
		}
		log.Printf("Found %d processes on server %s", len(processes), clientURL)

		for _, process := range processes {
			// Create process key in the same format as saved preferences
			processName := process.Name
			if process.Group != "" {
				processName = process.Group + ":" + process.Name
			}
			processKey := fmt.Sprintf("%s:%s", clientURL, processName)

			log.Printf("Checking process: %s, Current State: %s", processKey, process.State)

			prev, exists := h.previousStatus[processKey]
			if !exists {
				log.Printf("First time seeing process %s, setting initial state: %s", processKey, process.State)
				h.previousStatus[processKey] = process.State
				continue
			}

			log.Printf("Process %s - Previous state: %s, Current state: %s", processKey, prev, process.State)

			// Check if process transitioned from RUNNING to non-RUNNING state
			if prev == "RUNNING" && process.State != "RUNNING" {
				log.Printf("State change detected for %s: %s -> %s", processKey, prev, process.State)

				isManuallyStopped := h.userStoppedProcesses[processKey]
				log.Printf("Process %s manually stopped status: %v", processKey, isManuallyStopped)

				// Only send notification if process wasn't manually stopped
				if !isManuallyStopped {
					log.Printf("Preparing notification for process %s", processKey)

					message := "*Process Status Change Alert*\n"
					message += fmt.Sprintf("*Server:* `%s`\n", telegram.EscapeMarkdownV2(clientURL))
					message += telegram.FormatProcessStatusChange(process)

					shortID := utils.GetShortServerId(clientURL)
					keyboard := [][]tgbotapi.InlineKeyboardButton{
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
					}
					markup := tgbotapi.NewInlineKeyboardMarkup(keyboard...)

					log.Printf("Querying database for users subscribed to %s", processKey)
					// Query users who have subscribed to this process
					rows, err := h.db.Query("SELECT chat_id FROM user_preferences WHERE $1 = ANY(processes)", processKey)
					if err != nil {
						log.Printf("Error querying user preferences: %v", err)
						continue
					}
					defer rows.Close()

					userCount := 0
					for rows.Next() {
						var chatID int64
						if err := rows.Scan(&chatID); err != nil {
							log.Printf("Error scanning chat ID: %v", err)
							continue
						}
						userCount++

						log.Printf("Sending notification for process %s to chat %d", processKey, chatID)
						log.Printf("Notification message: %s", message)

						if err := telegram.SendToTelegramWithInlineKeyboard(h.bot, chatID, message, markup); err != nil {
							log.Printf("Error sending notification to Telegram: %v", err)
						} else {
							log.Printf("Successfully sent notification to chat %d", chatID)
						}
					}
					log.Printf("Found %d users subscribed to process %s", userCount, processKey)
				} else {
					log.Printf("Skipping notification for manually stopped process %s", processKey)
				}
			}

			// Reset user stopped flag when process starts running again
			if process.State == "RUNNING" {
				if _, exists := h.userStoppedProcesses[processKey]; exists {
					log.Printf("Resetting manual stop flag for process %s as it's now running", processKey)
					delete(h.userStoppedProcesses, processKey)
				}
			}

			log.Printf("Updating previous status for %s to %s", processKey, process.State)
			h.previousStatus[processKey] = process.State
		}
	}
	log.Println("Completed process status check")
}

func (h *Handler) ShowAllProcesses(chatID int64) {
	var allKeyboards [][]tgbotapi.KeyboardButton

	for clientID, client := range h.supervisorClients {
		var summary strings.Builder

		summary.WriteString("*📊 Process Status Summary*\n\n")

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
					allKeyboards = append(allKeyboards, tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton(fmt.Sprintf("View %s", fullProcessName)),
					))
				}
			}
			summary.WriteString("\n")
		}

		summary.WriteString("\n💡 *Tips:*\n")
		summary.WriteString("• Click on process name to view details\n")
		summary.WriteString("• Use /help to see all commands\n")

		message := summary.String()
		msg := tgbotapi.NewMessage(chatID, message)
		msg.ParseMode = "MarkdownV2"

		err = telegram.SendToTelegram(h.bot, chatID, message)
		if err != nil {
			log.Printf("Error sending processes list for server %s to Telegram: %v", clientID, err)
		}
	}

	replyKeyboard := tgbotapi.NewReplyKeyboard(allKeyboards...)
	msg := tgbotapi.NewMessage(chatID, "Select a process to view details:")
	msg.ReplyMarkup = replyKeyboard

	err := telegram.SendToTelegramWithReplyKeyboard(h.bot, chatID, msg)
	if err != nil {
		log.Printf("Error sending reply keyboard to Telegram: %v", err)
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

func (h *Handler) SaveUserPreferences(chatID int64, processes []string) error {
	_, err := h.db.Exec("INSERT INTO user_preferences (chat_id, processes) VALUES ($1, $2) ON CONFLICT (chat_id) DO UPDATE SET processes = $2", chatID, pq.Array(processes))
	return err
}

func (h *Handler) GetUserPreferences(chatID int64) ([]string, error) {
	var preferences []string
	err := h.db.QueryRow("SELECT processes FROM user_preferences WHERE chat_id = $1", chatID).Scan(pq.Array(&preferences))
	if err != nil {
		return nil, err
	}
	return preferences, nil
}

func (h *Handler) ShowNotifyOptions(chatID int64) {
	// Get all processes with a single map to store them
	processes := make([]models.Process, 0)
	for serverURL, client := range h.supervisorClients {
		clientProcesses, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting processes from %s: %v", serverURL, err)
			continue
		}
		// Set server URL in batch
		for i := range clientProcesses {
			clientProcesses[i].ServerURL = serverURL
		}
		processes = append(processes, clientProcesses...)
	}

	// Get current preferences in one query
	preferences, _ := h.GetUserPreferences(chatID)

	// Build keyboard with current preferences
	keyboard := telegram.ChooseProcessesKeyboard(processes, preferences)

	// Send message with keyboard
	telegram.SendToTelegramWithInlineKeyboard(
		h.bot,
		chatID,
		"Choose processes to be notified about:",
		keyboard,
	)
}

func (h *Handler) SaveUserPreference(chatID int64, processName, serverURL string) error {
	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var preferences []string
	err = tx.QueryRow("SELECT processes FROM user_preferences WHERE chat_id = $1", chatID).Scan(pq.Array(&preferences))
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get preferences: %w", err)
	}

	preference := fmt.Sprintf("%s:%s", serverURL, processName)
	found := false

	// Optimize preference toggle
	for i, p := range preferences {
		if p == preference {
			// Remove preference (toggle off)
			preferences[i] = preferences[len(preferences)-1]
			preferences = preferences[:len(preferences)-1]
			found = true
			break
		}
	}

	if !found {
		// Add preference (toggle on)
		preferences = append(preferences, preference)
	}

	// Update preferences in a single query
	_, err = tx.Exec(
		"INSERT INTO user_preferences (chat_id, processes) VALUES ($1, $2) "+
			"ON CONFLICT (chat_id) DO UPDATE SET processes = $2",
		chatID,
		pq.Array(preferences),
	)
	if err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}

	return tx.Commit()
}

func (h *Handler) handleChooseCallback(query *tgbotapi.CallbackQuery, processName, shortID string) error {
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	// Find server URL once
	var serverURL string
	for url := range h.supervisorClients {
		if utils.GetShortServerId(url) == shortID {
			serverURL = url
			break
		}
	}
	if serverURL == "" {
		return fmt.Errorf("server not found for ID: %s", shortID)
	}

	// Save preference
	if err := h.SaveUserPreference(chatID, processName, serverURL); err != nil {
		return fmt.Errorf("failed to save preference: %w", err)
	}

	// Get all processes efficiently
	processes := make([]models.Process, 0)
	for sURL, client := range h.supervisorClients {
		clientProcesses, err := client.GetAllProcesses()
		if err != nil {
			log.Printf("Error getting processes from %s: %v", sURL, err)
			continue
		}
		for i := range clientProcesses {
			clientProcesses[i].ServerURL = sURL
		}
		processes = append(processes, clientProcesses...)
	}

	// Get updated preferences
	preferences, err := h.GetUserPreferences(chatID)
	if err != nil {
		return fmt.Errorf("failed to get preferences: %w", err)
	}

	// Update keyboard
	keyboard := telegram.ChooseProcessesKeyboard(processes, preferences)
	editMsg := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, keyboard)

	if _, err := h.bot.Send(editMsg); err != nil {
		return fmt.Errorf("failed to update keyboard: %w", err)
	}

	return nil
}
