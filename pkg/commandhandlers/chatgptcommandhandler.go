package commandhandlers

import (
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	openai "github.com/sashabaranov/go-openai"
)

const (
	ChatGPTCommandOptionPrompt = "prompt"
	ChatGPTCommandOptionModel  = "model"

	DefaultGPTModel = openai.GPT3Dot5Turbo
)

func ChatGPTCommandHandler(openaiClient *openai.Client, messagesCache *map[string][]openai.ChatCompletionMessage) func(s *discord.Session, i *discord.InteractionCreate) {
	return func(s *discord.Session, i *discord.InteractionCreate) {
		log.Printf("[i.ID: %s] Interaction invoked by [UID: %s, Name: %s]\n", i.Interaction.ID, i.Member.User.ID, i.Member.User.Username)

		// Access options in the order provided by the user.
		options := i.ApplicationCommandData().Options

		// Or convert the slice into a map
		optionMap := make(map[string]*discord.ApplicationCommandInteractionDataOption, len(options))
		for _, opt := range options {
			optionMap[opt.Name] = opt
		}

		// Get the value from the option map.
		// When the option exists, ok = true
		var prompt string
		if option, ok := optionMap[ChatGPTCommandOptionPrompt]; ok {
			// Option values must be type asserted from interface{}.
			// Discordgo provides utility functions to make this simple.
			prompt = option.StringValue()
		} else {
			// We can't have empty prompt, unfortunately
			// this should not happen, discord prevents empty required options
			log.Printf("[i.ID: %s] Failed to parse prompt option\n", i.Interaction.ID)
			interactrionRespond(s, i.Interaction, "ERROR: Failed to parse prompt option")
			return
		}

		model := DefaultGPTModel
		if option, ok := optionMap[ChatGPTCommandOptionModel]; ok {
			model = option.StringValue()
			log.Printf("[i.ID: %s] Model provided: %s\n", i.Interaction.ID, model)
		}

		interactrionRespond(s, i.Interaction, fmt.Sprintf("<@%s> asked:\n> %s", i.Member.User.ID, prompt))

		m, err := s.InteractionResponse(i.Interaction)
		if err != nil {
			// Without interaction reference we cannot edit the response with chatGPT
			// Maybe in the future just try to post a new message instead, but for now just cancel
			log.Printf("[i.ID: %s] Failed to get interaction reference with error: %v\n", i.Interaction.ID, err)
			interactionEdit(s, i.Interaction, fmt.Sprintf("Failed to get interaction reference with error: %v", err))
			return
		}

		handlers.HandleChatGPTRequest(
			openaiClient,
			model,
			s,
			m.ChannelID,
			m.ID,
			i.Interaction.Member.User.Username,
			prompt,
			nil,
			messagesCache,
		)
	}
}

func interactrionRespond(s *discord.Session, i *discord.Interaction, content string) {
	s.InteractionRespond(i, &discord.InteractionResponse{
		Type: discord.InteractionResponseChannelMessageWithSource,
		Data: &discord.InteractionResponseData{
			Content: content,
		},
	})
}

func interactionEdit(s *discord.Session, i *discord.Interaction, content string) {
	s.InteractionResponseEdit(i, &discord.WebhookEdit{
		Content: &content,
	})
}
