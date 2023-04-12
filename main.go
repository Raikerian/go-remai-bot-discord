package main

import (
	"log"
	"os"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/legacy/commandhandlers"
	"github.com/raikerian/go-remai-bot-discord/pkg/legacy/commandoptions"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v2"
)

type Config struct {
	GuildID        string `yaml:"guild"`
	BotToken       string `yaml:"discordToken"`
	OpenAIToken    string `yaml:"openAIToken"`
	RemoveCommands bool   `yaml:"removeCommands"`
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

var (
	dmPermission                   = false
	defaultMemberPermissions int64 = discord.PermissionViewChannel

	chatGPTCommand = &discord.ApplicationCommand{
		Name:                     constants.CommandTypeChatGPT,
		Description:              "Start conversation with ChatGPT",
		DefaultMemberPermissions: &defaultMemberPermissions,
		DMPermission:             &dmPermission,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        commandoptions.ChatGPTCommandOptionPrompt.String(),
				Description: "ChatGPT prompt",
				Required:    true,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        commandoptions.ChatGPTCommandOptionContext.String(),
				Description: "Sets context that guides the AI assistant's behavior during the conversation",
				Required:    false,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        commandoptions.ChatGPTCommandOptionModel.String(),
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

	infoCommand = &discord.ApplicationCommand{
		Name:                     "info",
		Description:              "Show information about current version of Rem AI",
		DefaultMemberPermissions: &defaultMemberPermissions,
		DMPermission:             &dmPermission,
	}

	commands = []*discord.ApplicationCommand{
		chatGPTCommand,
		infoCommand,
	}
)

func main() {
	// Read config from file
	config := &Config{}
	err := config.ReadFromFile("credentials.yaml")
	if err != nil {
		log.Fatalf("Error reading credentials.yaml: %v", err)
	}

	var (
		discordSession *discord.Session
		openaiClient   *openai.Client
	)

	discordSession, err = discord.New("Bot " + config.BotToken)
	if err != nil {
		// Discord session is backbone of this application,
		// if can't open the session exit immediately
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	if config.OpenAIToken != "" {
		openaiClient = openai.NewClient(config.OpenAIToken)
	}

	b := bot.NewBot(discordSession, openaiClient)

	// Register command handlers
	if openaiClient != nil {
		b.RegisterCommandHandler(chatGPTCommand.Name, commandhandlers.ChatGPTCommandHandler(openaiClient, b.GPTMessagesCache()))
	}
	b.RegisterCommandHandler(infoCommand.Name, commandhandlers.InfoCommandHandler())

	// Run the bot
	b.Run(commands, config.GuildID, config.RemoveCommands)
}
