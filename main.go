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

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
// func messageCreate(s *discord.Session, m *discord.MessageCreate) {
// 	if messages[m.ChannelID] != nil && s.State.User.ID != m.Author.ID {
// 		ChatGPT(m.ChannelID, m.ID, m.Author.Username, m.Content, m.Reference())
// 	}
// }

// func ChatGPT(channelID string, messageID string, authorUsername string, content string, messageReference *discord.MessageReference) {

// req := openai.ChatCompletionRequest{
// 	Model: openai.GPT3Dot5Turbo,
// 	// MaxTokens: 20,
// 	Messages: []openai.ChatCompletionMessage{
// 		{
// 			Role:    openai.ChatMessageRoleUser,
// 			Content: m.Content,
// 		},
// 	},
// 	Stream: true,
// }
// stream, err := openaiClient.CreateChatCompletionStream(context.Background(), req)
// if err != nil {
// 	fmt.Printf("ChatCompletionStream error: %v\n", err)
// 	return
// }
// defer stream.Close()

// fmt.Printf("Stream response: ")
// resp := ""
// for {
// 	response, err := stream.Recv()
// 	if errors.Is(err, io.EOF) {
// 		fmt.Println("\nStream finished")
// 		_, err = s.ChannelMessageEditComplex(
// 			&discord.MessageEdit{
// 				Content: &resp,
// 				ID:      m.ID,
// 				Channel: m.ChannelID,
// 			},
// 		)
// 		if err != nil {
// 			log.Fatalf("Error: %v", err)
// 		}
// 		locked := false
// 		_, err := s.ChannelEditComplex(m.ChannelID, &discord.ChannelEdit{
// 			Locked: &locked,
// 		})
// 		if err != nil {
// 			log.Fatalf("Error: %v", err)
// 		}
// 		return
// 	}

// 	if err != nil {
// 		fmt.Printf("\nStream error: %v\n", err)
// 		concatenatedContent := resp + fmt.Sprintf("\nStream error: %v", err)
// 		_, err = s.ChannelMessageEditComplex(
// 			&discord.MessageEdit{
// 				Content: &concatenatedContent,
// 				ID:      m.ID,
// 				Channel: m.ChannelID,
// 			},
// 		)
// 		if err != nil {
// 			log.Fatalf("Error: %v", err)
// 		}
// 		locked := false
// 		_, err := s.ChannelEditComplex(m.ChannelID, &discord.ChannelEdit{
// 			Locked: &locked,
// 		})
// 		if err != nil {
// 			log.Fatalf("Error: %v", err)
// 		}
// 		return
// 	}

// 	// fmt.Printf(response.Choices[0].Delta.Content)
// 	resp += response.Choices[0].Delta.Content
// 	concatenatedContent := resp + "... ⌛"
// 	_, err = s.ChannelMessageEditComplex(
// 		&discord.MessageEdit{
// 			Content: &concatenatedContent,
// 			ID:      m.ID,
// 			Channel: m.ChannelID,
// 		},
// 	)
// 	if err != nil {
// 		log.Fatalf("Error: %v", err)
// 	}
// }
// }
