package main

import (
	"log"
	"os"

	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/commands"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Discord struct {
		Token          string `yaml:"token"`
		GuildID        string `yaml:"guild"`
		RemoveCommands bool   `yaml:"removeCommands"`
	}

	OpenAIAPIKey string `yaml:"openAIAPIKey"`
}

func (c *Config) ReadFromFile(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, c)
	if err != nil {
		return err
	}
	return nil
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// var (
// 	dmPermission                   = false
// 	defaultMemberPermissions int64 = discord.PermissionViewChannel

// 	chatGPTCommand = &discord.ApplicationCommand{
// 		Name:                     constants.CommandTypeChatGPT,
// 		Description:              "Start conversation with ChatGPT",
// 		DefaultMemberPermissions: &defaultMemberPermissions,
// 		DMPermission:             &dmPermission,
// 		Options: []*discord.ApplicationCommandOption{
// 			{
// 				Type:        discord.ApplicationCommandOptionString,
// 				Name:        commandoptions.ChatGPTCommandOptionPrompt.String(),
// 				Description: "ChatGPT prompt",
// 				Required:    true,
// 			},
// 			{
// 				Type:        discord.ApplicationCommandOptionString,
// 				Name:        commandoptions.ChatGPTCommandOptionContext.String(),
// 				Description: "Sets context that guides the AI assistant's behavior during the conversation",
// 				Required:    false,
// 			},
// 			{
// 				Type:        discord.ApplicationCommandOptionString,
// 				Name:        commandoptions.ChatGPTCommandOptionModel.String(),
// 				Description: "GPT model",
// 				Required:    false,
// 				Choices: []*discord.ApplicationCommandOptionChoice{
// 					{
// 						Name:  "GPT-3.5-Turbo (Default)",
// 						Value: openai.GPT3Dot5Turbo,
// 					},
// 					{
// 						Name:  "GPT-4",
// 						Value: openai.GPT4,
// 					},
// 				},
// 			},
// 		},
// 	}

// 	infoCommand = &discord.ApplicationCommand{
// 		Name:                     "info",
// 		Description:              "Show information about current version of Rem AI",
// 		DefaultMemberPermissions: &defaultMemberPermissions,
// 		DMPermission:             &dmPermission,
// 	}

// 	commands = []*discord.ApplicationCommand{
// 		chatGPTCommand,
// 		infoCommand,
// 	}
// )

var (
	discordBot   *bot.Bot
	openaiClient *openai.Client

	gptMessagesCache     *cache.GPTMessagesCache
	ignoredChannelsCache = make(commands.IgnoredChannelsCache)
)

func main() {
	// Read config from file
	config := &Config{}
	err := config.ReadFromFile("credentials.yaml")
	if err != nil {
		log.Fatalf("Error reading credentials.yaml: %v", err)
	}

	// Initialize cache
	gptMessagesCache, err = cache.NewGPTMessagesCache(constants.DiscordThreadsCacheSize)
	if err != nil {
		log.Fatalf("Error initializing GPTMessagesCache: %v", err)
	}

	// Initialize discord bot
	discordBot, err = bot.NewBot(config.Discord.Token)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	// Register commands
	if config.OpenAIAPIKey != "" {
		openaiClient = openai.NewClient(config.OpenAIAPIKey) // initialize OpenAI client first
		discordBot.Router.Register(commands.ChatGPTCommand(&commands.ChatGPTCommandParams{
			OpenAIClient:         openaiClient,
			MessagesCache:        gptMessagesCache,
			IgnoredChannelsCache: &ignoredChannelsCache,
		}))
	}
	discordBot.Router.Register(commands.InfoCommand())

	// Run the bot
	discordBot.Run(config.Discord.GuildID, config.Discord.RemoveCommands)
}
