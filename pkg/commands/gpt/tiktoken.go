package gpt

import (
	"github.com/sashabaranov/go-openai"
	"github.com/tiktoken-go/tokenizer"
)

const (
	tokensPerMessage = 3
	tokensPerName    = 1
)

func countMessageTokens(message openai.ChatCompletionMessage, model string) *int {
	enc, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		enc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	}

	tokens := _countMessageTokens(enc, tokensPerMessage, tokensPerName, message)
	return &tokens
}

func countMessagesTokens(messages []openai.ChatCompletionMessage, model string) *int {
	enc, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		enc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	}

	tokens := 0
	for _, message := range messages {
		tokens += _countMessageTokens(enc, tokensPerMessage, tokensPerName, message)
	}
	tokens += 3 // every reply is primed with <im_start>assistant

	return &tokens
}

func countAllMessagesTokens(systemMessage *openai.ChatCompletionMessage, messages []openai.ChatCompletionMessage, model string) *int {
	if systemMessage != nil {
		messages = append(messages, *systemMessage)
	}
	return countMessagesTokens(messages, model)
}

func _countMessageTokens(enc tokenizer.Codec, tokensPerMessage int, tokensPerName int, message openai.ChatCompletionMessage) int {
	tokens := tokensPerMessage
	contentIds, _, _ := enc.Encode(message.Content)
	roleIds, _, _ := enc.Encode(message.Role)
	tokens += len(contentIds)
	tokens += len(roleIds)
	if message.Name != "" {
		tokens += tokensPerName
		nameIds, _, _ := enc.Encode(message.Name)
		tokens += len(nameIds)
	}
	return tokens
}
