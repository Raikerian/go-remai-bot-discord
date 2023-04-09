package handlers

import (
	"context"
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/sashabaranov/go-openai"
)

type ChatGPTRequestParams struct {
	OpenAIClient     *openai.Client
	GPTModel         string
	GPTPrompt        string
	DiscordSession   *discord.Session
	DiscordGuildID   string
	DiscordChannelID string
	DiscordMessageID string
	MessagesCache    *lru.Cache[string, *cache.ChatGPTMessagesCache]
}

func OnChatGPTRequest(params ChatGPTRequestParams) {
	cache, ok := params.MessagesCache.Get(params.DiscordChannelID)
	if !ok {
		panic(fmt.Sprintf("[GID: %s, CHID: %s] Failed to retrieve messages cache for channel", params.DiscordGuildID, params.DiscordChannelID))
	}

	log.Printf("[GID: %s, CHID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v\n", params.DiscordGuildID, params.DiscordChannelID, params.GPTModel, len(cache.Messages))

	cache.Messages = append(cache.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: params.GPTPrompt,
	})

	// Create message with ChatGPT
	messages := cache.Messages
	if cache.SystemMessage != nil {
		messages = append([]openai.ChatCompletionMessage{*cache.SystemMessage}, messages...)
	}
	resp, err := params.OpenAIClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    params.GPTModel,
			Messages: messages,
			// Temperature: 0.1,
		},
	)
	if err != nil {
		// ChatGPT failed for whatever reason, tell users about it
		log.Printf("[GID: %s, CHID: %s] ChatGPT request ChatCompletion failed with the error: %v\n", params.DiscordGuildID, params.DiscordChannelID, err)
		discordChannelMessageEdit(params.DiscordSession, params.DiscordMessageID, params.DiscordChannelID, params.DiscordGuildID, fmt.Sprintf("❌ ChatGPT request ChatCompletion failed with the error: %v", err))
		return
	}

	// Save response to context cache
	responseContent := resp.Choices[0].Message.Content
	log.Printf("[GID: %s, CHID: %s] ChatGPT Request [Model: %s] responded with a usage: [PromptTokens: %d, CompletionTokens: %d, TotalTokens: %d]\n", params.DiscordGuildID, params.DiscordChannelID, params.GPTModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	cache.Messages = append(cache.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: responseContent,
	})

	discordChannelMessageEdit(params.DiscordSession, params.DiscordMessageID, params.DiscordChannelID, params.DiscordGuildID, responseContent)
}

func discordChannelMessageEdit(s *discord.Session, messageID string, channelID string, guildID string, content string) {
	_, err := s.ChannelMessageEditComplex(
		&discord.MessageEdit{
			Content: &content,
			ID:      messageID,
			Channel: channelID,
		},
	)
	if err != nil {
		log.Printf("[GID: %s, CHID: %s] Failed to edit message [MID: %s] with the error: %v\n", guildID, channelID, messageID, err)
	}
}
