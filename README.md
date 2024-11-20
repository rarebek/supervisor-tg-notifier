# Supervisor Telegram Notifier

Supervisor Telegram Notifier is an open-source project that integrates Supervisor process control with Telegram notifications. It allows you to manage and monitor your processes directly from a Telegram bot.

## Features

- List all processes managed by Supervisor
- Start and stop processes
- View detailed information about each process
- Receive notifications when process statuses change
- Paginated view for processes
- Inline keyboard for easy interaction

## Installation

1. Clone the repository:
    ```sh
    git clone https://github.com/rarebek/supervisor-tg-notifier.git
    cd supervisor-tg-notifier
    ```

2. Install dependencies:
    ```sh
    go mod tidy
    ```

3. Create a `.env` file in the root directory and add the following environment variables:
    ```env
    TELEGRAM_BOT_TOKEN=your_telegram_bot_token
    PROCESSES_PER_PAGE=5
    TELEGRAM_CHAT_ID=your_telegram_chat_id
    SERVER_URL=http://127.0.0.1:9001/RPC2
    ```

    - `TELEGRAM_BOT_TOKEN`: Your Telegram bot token obtained from BotFather.
    - `PROCESSES_PER_PAGE`: Number of processes to display per page in the paginated view.
    - `TELEGRAM_CHAT_ID`: The chat ID where the bot will send notifications.
    - `SERVER_URL`: The URL of your Supervisor XML-RPC interface.

## Usage

1. Build and run the project:
    ```sh
    go build -o supervisor-tg-notifier cmd/main.go
    ./supervisor-tg-notifier
    ```

2. Interact with the bot on Telegram:
    - Send "List Processes" to list all processes.
    - Send "Start <process_name>" to start a process.
    - Send "Stop <process_name>" to stop a process.
    - Send "Show All" or "/all" to view all processes with their statuses.

## Project Structure

- [cmd/main.go](cmd/main.go): Entry point of the application.
- [pkg/bot/handler.go](pkg/bot/handler.go): Handles Telegram bot updates and interactions.
- [pkg/config/config.go](pkg/config/config.go): Loads and manages configuration from environment variables.
- [pkg/models/process.go](pkg/models/process.go): Defines the `Process` model.
- [pkg/supervisor/client.go](pkg/supervisor/client.go): Interacts with the Supervisor XML-RPC interface.
- [pkg/telegram/formatter.go](pkg/telegram/formatter.go): Formats messages for Telegram.
- [pkg/telegram/keyboard.go](pkg/telegram/keyboard.go): Builds inline keyboards for Telegram.
- [pkg/telegram/sender.go](pkg/telegram/sender.go): Sends messages to Telegram.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.