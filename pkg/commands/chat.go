package commands

import (
	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/client"
	"github.com/raikerian/go-remai-bot-discord/pkg/commands/gpt"
	"github.com/sashabaranov/go-openai"
)

const chatCommandName = "chat"

type ChatCommandParams struct {
	OpenAIClient           *openai.Client
	OpenAICompletionModels []string
	GPTMessagesCache       *gpt.MessagesCache
	IgnoredChannelsCache   *gpt.IgnoredChannelsCache
	GoogleSearchClient     *client.GoogleSearch
}

func ChatCommand(params *ChatCommandParams) *bot.Command {
	return &bot.Command{
		Name:                     chatCommandName,
		Description:              "Start conversation with LLM",
		DMPermission:             false,
		DefaultMemberPermissions: discord.PermissionViewChannel,
		Type:                     discord.ChatApplicationCommand,
		SubCommands: bot.NewRouter([]*bot.Command{
			gpt.Command(params.OpenAIClient, params.OpenAICompletionModels, params.GPTMessagesCache, params.IgnoredChannelsCache, params.GoogleSearchClient),
		}),
	}
}
