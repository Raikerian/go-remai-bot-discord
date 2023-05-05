package gpt

import (
	"github.com/sashabaranov/go-openai"
	"github.com/tiktoken-go/tokenizer"
)

func countTokens(messages []openai.ChatCompletionMessage, model string) *int {
	var tokensPerMessage int
	var tokensPerName int
	switch model {
	case openai.GPT3Dot5Turbo:
		// gpt-3.5-turbo may change over time. Returning num tokens assuming gpt-3.5-turbo-0301
		return countTokens(messages, openai.GPT3Dot5Turbo0301)
	case openai.GPT4:
		// gpt-4 may change over time. Returning num tokens assuming gpt-4-0314
		return countTokens(messages, openai.GPT40314)
	case openai.GPT3Dot5Turbo0301:
		tokensPerMessage = 4 // every message follows <im_start>{role/name}\n{content}<im_end>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	case openai.GPT40314:
		tokensPerMessage = 3
		tokensPerName = 1
	default:
		// Not implemented
		return nil
	}

	enc, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		enc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	}

	tokens := 0
	for _, message := range messages {
		tokens += tokensPerMessage
		contentIds, _, _ := enc.Encode(message.Content)
		roleIds, _, _ := enc.Encode(message.Role)
		tokens += len(contentIds)
		tokens += len(roleIds)
		if message.Name != "" {
			tokens += tokensPerName
		}
	}
	tokens += 2 // every reply is primed with <im_start>assistant

	return &tokens
}

func countAllTokens(systemMessage *openai.ChatCompletionMessage, messages []openai.ChatCompletionMessage, model string) *int {
	if systemMessage != nil {
		messages = append(messages, *systemMessage)
	}
	return countTokens(messages, model)
}
