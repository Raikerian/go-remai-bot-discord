package gpt

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type chatGPTResponse struct {
	content string
	usage   openai.Usage
}

func sendChatGPTRequest(client *openai.Client, cacheItem *MessagesCacheData, functions ...*openai.FunctionDefine) (*chatGPTResponse, error) {
	// Create message with ChatGPT
	messages := cacheItem.Messages
	if cacheItem.SystemMessage != nil {
		messages = append([]openai.ChatCompletionMessage{*cacheItem.SystemMessage}, messages...)
	}

	req := openai.ChatCompletionRequest{
		Model:    cacheItem.Model,
		Messages: messages,
	}

	if functions != nil {
		req.Functions = functions
		req.FunctionCall = "auto"
	}

	if cacheItem.Temperature != nil {
		req.Temperature = *cacheItem.Temperature
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		req,
	)
	if err != nil {
		return nil, err
	}

	if resp.Choices[0].Message.FunctionCall != nil && resp.Choices[0].Message.FunctionCall.Name == "googlesearch" {
	}

	// Save response to context cache
	responseContent := resp.Choices[0].Message.Content
	cacheItem.Messages = append(cacheItem.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: responseContent,
	})
	cacheItem.TokenCount = resp.Usage.TotalTokens
	return &chatGPTResponse{
		content: responseContent,
		usage:   resp.Usage,
	}, nil
}
