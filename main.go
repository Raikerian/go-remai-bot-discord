package main

import (
	"log"
	"net/http"
	"os"

	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/commands"
	"github.com/raikerian/go-remai-bot-discord/pkg/commands/chat"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Discord struct {
		Token          string `yaml:"token"`
		Guild          string `yaml:"guild"`
		RemoveCommands bool   `yaml:"removeCommands"`
	} `yaml:"discord"`
	OpenAI struct {
		APIKey           string   `yaml:"apiKey"`
		CompletionModels []string `yaml:"completionModels"`
	} `yaml:"openAI"`
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
	discordBot   *bot.Bot
	openaiClient *openai.Client

	gptMessagesCache      *cache.GPTMessagesCache
	ignoredChannelsCache  = make(chat.IgnoredChannelsCache)
	imageUploadHTTPClient *http.Client
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
	if config.OpenAI.APIKey != "" {
		openaiClient = openai.NewClient(config.OpenAI.APIKey) // initialize OpenAI client first

		discordBot.Router.Register(chat.Command(&chat.CommandParams{
			OpenAIClient:         openaiClient,
			GPTMessagesCache:     gptMessagesCache,
			IgnoredChannelsCache: &ignoredChannelsCache,
			CompletionModels:     config.OpenAI.CompletionModels,
		}))

		imageUploadHTTPClient = &http.Client{Timeout: (commands.ImageHTTPRequestTimeout)}
		discordBot.Router.Register(commands.ImageCommand(&commands.ImageCommandParams{
			OpenAIClient:          openaiClient,
			ImageUploadHTTPClient: imageUploadHTTPClient,
		}))
	}
	discordBot.Router.Register(commands.InfoCommand())

	// Run the bot
	discordBot.Run(config.Discord.Guild, config.Discord.RemoveCommands)
}
