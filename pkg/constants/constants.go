package constants

import "github.com/sashabaranov/go-openai"

const (
	Version = "0.1.0"

	DiscordThreadsCacheSize = 32

	GenericPendingMessage                    = "⌛ Wait a moment, please..."
	DiscordThreadAutoArchivewDurationMinutes = 60 // Discord expects the auto_archive_duration to be one of the following values: 60, 1440, 4320, or 10080, which represent the number of minutes before a thread is automatically archived (1 hour, 1 day, 3 days, or 7 days, respectively).

	DefaultGPTModel = openai.GPT3Dot5Turbo

	CommandTypeChatGPT = "chatgpt"
)
