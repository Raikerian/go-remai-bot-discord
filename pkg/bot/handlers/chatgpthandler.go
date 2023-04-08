package handlers

import (
	"context"
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	openai "github.com/sashabaranov/go-openai"
)

type ChatGPTHandlerParams struct {
	OpenAIClient     *openai.Client
	GPTModel         string
	GPTPrompt        string
	DiscordSession   *discord.Session
	DiscordChannelID string
	DiscordMessageID string
	MessagesCache    *map[string][]openai.ChatCompletionMessage
}

func ChatGPTRequest(params ChatGPTHandlerParams) {
	log.Printf("[CHID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v", params.DiscordChannelID, params.GPTModel, len(*params.MessagesCache))

	// Prepare message
	if params.MessagesCache == nil {
		// Initialize messageCache if it's nil
		tempMessagesCache := make(map[string][]openai.ChatCompletionMessage)
		params.MessagesCache = &tempMessagesCache
	}
	(*params.MessagesCache)[params.DiscordChannelID] = append((*params.MessagesCache)[params.DiscordChannelID], openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: params.GPTPrompt,
	})

	// Create message with ChatGPT
	resp, err := params.OpenAIClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    params.GPTModel,
			Messages: (*params.MessagesCache)[params.DiscordChannelID],
		},
	)
	if err != nil {
		// ChatGPT failed for whatever reason, tell users about it
		log.Printf("[CHID: %s] ChatGPT request ChatCompletion failed with the error: %v", params.DiscordChannelID, err)
		discordChannelMessageEdit(params.DiscordSession, params.DiscordMessageID, params.DiscordChannelID, fmt.Sprintf("❌ ChatGPT request ChatCompletion failed with the error: %v", err))
		return
	}

	// Save response to context cache
	responseContent := resp.Choices[0].Message.Content
	log.Printf("[CHID: %s] ChatGPT Request [Model: %s] responded with a message: %s", params.DiscordChannelID, params.GPTModel, responseContent)
	(*params.MessagesCache)[params.DiscordChannelID] = append((*params.MessagesCache)[params.DiscordChannelID], openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: responseContent,
	})

	discordChannelMessageEdit(params.DiscordSession, params.DiscordMessageID, params.DiscordChannelID, responseContent)
}

func discordChannelMessageEdit(s *discord.Session, messageID string, channelID string, content string) {
	_, err := s.ChannelMessageEditComplex(
		&discord.MessageEdit{
			Content: &content,
			ID:      messageID,
			Channel: channelID,
		},
	)
	if err != nil {
		log.Printf("[CHID: %s] Failed to edit message [MID: %s] with the error: %v", channelID, messageID, err)
	}
}
