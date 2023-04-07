package handlers

import (
	"context"
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	openai "github.com/sashabaranov/go-openai"
)

func ChatGPT(openaiClient *openai.Client, gptModel string, s *discord.Session, channelID string, messageID string, authorUsername string, content string, messageReference *discord.MessageReference, messagesCache *map[string][]openai.ChatCompletionMessage) {
	// Initialize messageCache if it's nil
	if messagesCache == nil {
		tempMessagesCache := make(map[string][]openai.ChatCompletionMessage)
		messagesCache = &tempMessagesCache
	}

	// Create thread or send message to the existing thread
	if ch, err := s.State.Channel(channelID); err != nil || !ch.IsThread() {
		thread, err := s.MessageThreadStartComplex(channelID, messageID, &discord.ThreadStart{
			Name:                gptModel + " conversation with " + authorUsername,
			AutoArchiveDuration: 60,
			Invitable:           false,
		})
		if err != nil {
			log.Fatalf("Error: %v", err)
			return
		}
		channelMessage, err := s.ChannelMessageSend(thread.ID, "⌛ Wait a moment, please...")
		if err != nil {
			log.Fatalf("Error: %v", err)
			return
		}
		channelID = thread.ID
		messageID = channelMessage.ID
	} else {
		if ch.ThreadMetadata.Locked {
			// temp safety condition to ignore messages until thread is unclocked
			// TODO: handle this better
			return
		}
		channelMessage, err := s.ChannelMessageSendReply(channelID, "⌛ Wait a moment, please...", messageReference)
		if err != nil {
			log.Fatalf("Error: %v", err)
			return
		}
		messageID = channelMessage.ID
	}

	// Lock the thread while we are generating ChatGPT answser
	err := utils.ToggleDiscordThreadLock(s, channelID, true)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Prepare message
	(*messagesCache)[channelID] = append((*messagesCache)[channelID], openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: content,
	})
	log.Println((*messagesCache)[channelID])

	// Create message with ChatGPT
	resp, err := openaiClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    gptModel,
			Messages: (*messagesCache)[channelID],
		},
	)
	if err != nil {
		// TODO: handle this error better
		fmt.Printf("ChatCompletion error: %v\n", err)
		return
	}

	// Save response to context cache
	responseContent := resp.Choices[0].Message.Content
	(*messagesCache)[channelID] = append((*messagesCache)[channelID], openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: responseContent,
	})
	log.Print("Model: " + gptModel)
	log.Println((*messagesCache)[channelID])

	_, err = s.ChannelMessageEditComplex(
		&discord.MessageEdit{
			Content: &responseContent,
			ID:      messageID,
			Channel: channelID,
		},
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Unlock the thread
	err = utils.ToggleDiscordThreadLock(s, channelID, false)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}
