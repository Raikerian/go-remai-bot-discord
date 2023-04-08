package main

import (
	"flag"
	"log"
	"os"

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
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var err error
	discordSession, err = discord.New("Bot " + *BotToken)
	if err != nil {
		// Discord session is backbone of this application,
		// if can't open the session exit immediately
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	if OpenAIToken != nil {
		openaiClient = openai.NewClient(*OpenAIToken)
	}
}

var (
	dmPermission                   = false
	defaultMemberPermissions int64 = discord.PermissionViewChannel

	chatGPTCommand = &discord.ApplicationCommand{
		Name:                     "chatgpt",
		Description:              "Start conversation with ChatGPT",
		DefaultMemberPermissions: &defaultMemberPermissions,
		DMPermission:             &dmPermission,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        commandhandlers.ChatGPTCommandOptionPrompt,
				Description: "ChatGPT prompt",
				Required:    true,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        commandhandlers.ChatGPTCommandOptionModel,
				Description: "GPT model",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  "GPT-3.5-Turbo (Default)",
						Value: openai.GPT3Dot5Turbo,
					},
					{
						Name:  "GPT-4",
						Value: openai.GPT4,
					},
				},
			},
		},
	}

	commands = []*discord.ApplicationCommand{
		chatGPTCommand,
	}
)

func main() {
	b := bot.NewBot(discordSession, openaiClient)

	// Register command handlers
	if openaiClient != nil {
		b.RegisterCommandHandler(chatGPTCommand.Name, commandhandlers.ChatGPTCommandHandler(openaiClient, b.MessagesCache()))
	}

	// Run the bot
	b.Run(commands, *GuildID, *RemoveCommands)
}
