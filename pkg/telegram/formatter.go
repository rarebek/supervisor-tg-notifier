package telegram

import (
	"fmt"
	"strings"

	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
)

func EscapeMarkdownV2(input string) string {
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!", "|"}
	input = strings.ReplaceAll(input, "\\", "\\\\")
	for _, char := range specialChars {
		input = strings.ReplaceAll(input, char, "\\"+char)
	}
	return input
}

func FormatProcessList(processes []models.Process, page, totalPages int) string {
	message := fmt.Sprintf("*Processes List* \\(Page %d/%d\\)\n", page, totalPages)
	for _, process := range processes {
		escapedName := EscapeMarkdownV2(process.Name)
		escapedState := EscapeMarkdownV2(process.State)
		escapedGroup := EscapeMarkdownV2(process.Group)
		message += fmt.Sprintf("*Name:* `%s`\n*Status:* `%s`\n*Group:* `%s`\n\n", escapedName, escapedState, escapedGroup)
	}
	return message
}

func FormatProcessDetails(process models.Process) string {
	escapedName := EscapeMarkdownV2(process.Group + ":" + process.Name)
	escapedState := EscapeMarkdownV2(process.State)
	escapedDesc := EscapeMarkdownV2(process.Description)

	return fmt.Sprintf("*Process Details*\n\n"+
		"*Name:* `%s`\n"+
		"*Status:* `%s`\n"+
		"*Description:* `%s`",
		escapedName,
		escapedState,
		escapedDesc,
	)
}

func FormatProcessStatusChange(process models.Process) string {
	escapedName := EscapeMarkdownV2(process.Name)
	escapedState := EscapeMarkdownV2(process.State)
	escapedDesc := EscapeMarkdownV2(process.Description)

	return fmt.Sprintf("*Process Status Change*\n"+
		"*Name:* `%s`\n"+
		"*Status:* `%s`\n"+
		"*Error:* `%s`",
		escapedName, escapedState, escapedDesc)
}

func FormatAllProcessesList(processes []models.Process) string {
	message := "*All Processes Status*\n\n"
	for _, process := range processes {
		escapedName := EscapeMarkdownV2(process.Name)
		escapedState := EscapeMarkdownV2(process.State)
		escapedDesc := EscapeMarkdownV2(process.Description)

		message += fmt.Sprintf("*Process:* `%s`\n", escapedName)
		message += fmt.Sprintf("├ *Status:* `%s`\n", escapedState)
		message += fmt.Sprintf("└ *Info:* `%s`\n\n", escapedDesc)
	}
	return message
}
