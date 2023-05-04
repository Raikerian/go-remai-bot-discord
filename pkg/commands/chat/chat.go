package chat

import (
	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/sashabaranov/go-openai"
)

const (
	chatCommandName = "chat"
)

type IgnoredChannelsCache map[string]struct{}

type CommandParams struct {
	OpenAIClient         *openai.Client
	CompletionModels     []string
	GPTMessagesCache     *cache.GPTMessagesCache
	IgnoredChannelsCache *IgnoredChannelsCache
}

func Command(params *CommandParams) *bot.Command {
	return &bot.Command{
		Name:                     chatCommandName,
		Description:              "Start conversation with LLM",
		DMPermission:             false,
		DefaultMemberPermissions: discord.PermissionViewChannel,
		Type:                     discord.ChatApplicationCommand,
		SubCommands: bot.NewRouter([]*bot.Command{
			gptCommand(params),
		}),
	}
}
