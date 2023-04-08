package commandhandlers

import (
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	openai "github.com/sashabaranov/go-openai"
)

const (
	ChatGPTCommandOptionPrompt  = "prompt"
	ChatGPTCommandOptionContext = "context"
	ChatGPTCommandOptionModel   = "model"
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

		response := fmt.Sprintf("<@%s> asked:\n> %s", i.Member.User.ID, prompt)

		// Set context of the conversation as a system message
		var context string
		if option, ok := optionMap[ChatGPTCommandOptionContext]; ok {
			context = option.StringValue()
			response += fmt.Sprintf("\nand provided the following context:\n> %s", context)
			log.Printf("[i.ID: %s] Context provided: %s\n", i.Interaction.ID, context)
		}

		model := constants.DefaultGPTModel
		if option, ok := optionMap[ChatGPTCommandOptionModel]; ok {
			model = option.StringValue()
			log.Printf("[i.ID: %s] Model provided: %s\n", i.Interaction.ID, model)
		}

		// Respond to interaction with a reference and user ping
		interactrionRespond(s, i.Interaction, response)

		// Get interaction ID so we can create a thread on top of it
		m, err := s.InteractionResponse(i.Interaction)
		if err != nil {
			// Without interaction reference we cannot create a thread with the response of ChatGPT
			// Maybe in the future just try to post a new message instead, but for now just cancel
			log.Printf("[i.ID: %s] Failed to get interaction reference with the error: %v\n", i.Interaction.ID, err)
			interactionEdit(s, i.Interaction, fmt.Sprintf("Failed to get interaction reference with error: %v", err))
			return
		}

		// Create thread with or send message to the existing thread containing pending request
		if ch, err := s.State.Channel(m.ChannelID); err != nil || !ch.IsThread() {
			thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discord.ThreadStart{
				Name:                model + " conversation with " + i.Interaction.Member.User.Username,
				AutoArchiveDuration: 60,
				Invitable:           false,
			})
			if err != nil {
				// Without thread we cannot reply our answer
				log.Printf("[i.ID: %s] Failed to create a thread with the error: %v\n", i.Interaction.ID, err)
				return
			}

			// Lock the thread while we are generating ChatGPT answser
			utils.ToggleDiscordThreadLock(s, thread.ID, true)

			// temp: GPT4 unsupported
			if model == openai.GPT4 {
				utils.DiscordChannelMessageSend(s, thread.ID, "Oh no! 😕 The model \"gpt-4\" doesn't work yet. But don't fret! It will be available at some point soon. Meanwhile, go bug <@184088426973233153> about it 🤔", nil)
				return
			}

			// Unlock the thread at the end
			defer utils.ToggleDiscordThreadLock(s, thread.ID, false)

			channelMessage, err := utils.DiscordChannelMessageSend(s, thread.ID, constants.GenericPendingMessage, nil)
			if err != nil {
				// Without reply  we cannot edit message with the response of ChatGPT
				// Maybe in the future just try to post a new message instead, but for now just cancel
				log.Printf("[i.ID: %s] Failed to reply in the thread with the error: %v\n", i.Interaction.ID, err)
				return
			}

			// Set context of the conversation as a system message
			if context != "" {
				(*messagesCache)[thread.ID] = append((*messagesCache)[thread.ID], openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleSystem,
					Content: context,
				})
			}

			handlers.ChatGPTRequest(handlers.ChatGPTHandlerParams{
				OpenAIClient:     openaiClient,
				GPTModel:         model,
				GPTPrompt:        prompt,
				DiscordSession:   s,
				DiscordChannelID: thread.ID,
				DiscordMessageID: channelMessage.ID,
				MessagesCache:    messagesCache,
			})
		}
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
