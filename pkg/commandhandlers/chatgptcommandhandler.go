package commandhandlers

import (
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	openai "github.com/sashabaranov/go-openai"
)

func ChatGPTCommandHandler(openaiClient *openai.Client, gptModel string, messagesCache *map[string][]openai.ChatCompletionMessage) func(s *discord.Session, i *discord.InteractionCreate) {
	return func(s *discord.Session, i *discord.InteractionCreate) {
		iData := i.ApplicationCommandData()
		if len(iData.Options) < 0 {
			// TODO: throw err
		}

		lastOption := iData.Options[len(iData.Options)-1]
		// msgformat += fmt.Sprintf("> prompt: %s\n", lastOption.Value)

		// Perform a type assertion to convert the Value to a string
		lastOptionValue, ok := lastOption.Value.(string)
		if !ok {
			// Handle the case when the type assertion fails
		}

		// Respond with pending message
		s.InteractionRespond(i.Interaction, &discord.InteractionResponse{
			Type: discord.InteractionResponseChannelMessageWithSource,
			Data: &discord.InteractionResponseData{
				Content: fmt.Sprintf("<@%s> asked:\n> %s", i.Interaction.Member.User.ID, lastOptionValue),
			},
		})

		m, err := s.InteractionResponse(i.Interaction)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		handlers.ChatGPT(openaiClient, gptModel, s, m.ChannelID, m.ID, i.Interaction.Member.User.Username, lastOptionValue, nil, messagesCache)
	}
}
