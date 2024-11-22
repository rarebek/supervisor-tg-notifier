package models

type Process struct {
	Name        string
	State       string
	Description string
	ServerURL   string
	Group       string
}

type User struct {
	ChatID           int64
	ChoosenProcesses []string
}
