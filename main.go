package main

import (
	"flag"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/commandhandlers"
	openai "github.com/sashabaranov/go-openai"
)

// Bot parameters
var (
	GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
	BotToken       = flag.String("discord-token", "", "Bot access token")
	OpenAIToken    = flag.String("openai-token", "", "OpenAI access token")
	RemoveCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
)

func init() { flag.Parse() }

var (
	discordSession *discord.Session
	openaiClient   *openai.Client
)

func init() {
	var err error
	discordSession, err = discord.New("Bot " + *BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	openaiClient = openai.NewClient(*OpenAIToken)
}

var (
	dmPermission                   = false
	defaultMemberPermissions int64 = discord.PermissionViewChannel

	chatGPT3Command = &discord.ApplicationCommand{
		Name:                     "chatgpt3",
		Description:              "Start conversation with ChatGPT using ChatGPT-3.5 model",
		DefaultMemberPermissions: &defaultMemberPermissions,
		DMPermission:             &dmPermission,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        "prompt",
				Description: "ChatGPT-3.5 prompt",
				Required:    true,
				Options: []*discord.ApplicationCommandOption{

					{
						Type:        discord.ApplicationCommandOptionString,
						Name:        "prompt",
						Description: "ChatGPT-3.5 prompt",
						Required:    true,
					},
				},
			},
		},
	}

	chatGPT4Command = &discord.ApplicationCommand{
		Name:                     "chatgpt4",
		Description:              "Start conversation with ChatGPT using ChatGPT-4 model",
		DefaultMemberPermissions: &defaultMemberPermissions,
		DMPermission:             &dmPermission,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        "prompt",
				Description: "ChatGPT-4 prompt",
				Required:    true,
				Options: []*discord.ApplicationCommandOption{

					{
						Type:        discord.ApplicationCommandOptionString,
						Name:        "prompt",
						Description: "ChatGPT-4 prompt",
						Required:    true,
					},
				},
			},
		},
	}

	commands = []*discord.ApplicationCommand{
		chatGPT3Command,
		chatGPT4Command,
	}
)

func main() {
	b := bot.NewBot(discordSession, openaiClient)

	// Register command handlers
	b.RegisterCommandHandler(chatGPT3Command.Name, commandhandlers.ChatGPTCommandHandler(openaiClient, openai.GPT3Dot5Turbo, b.MessagesCache()))
	b.RegisterCommandHandler(chatGPT4Command.Name, commandhandlers.ChatGPTCommandHandler(openaiClient, openai.GPT4, b.MessagesCache()))

	// Run the bot
	b.Run(commands, *GuildID, *RemoveCommands)
}
