package gpt

import (
	"github.com/sashabaranov/go-openai"
	"github.com/tiktoken-go/tokenizer"
)

func tokensConfiguration(model string) (ok bool, tokensPerMessage int, tokensPerName int) {
	ok = true

	switch model {
	case openai.GPT3Dot5Turbo, openai.GPT3Dot5Turbo0301:
		// gpt-3.5-turbo may change over time. Returning num tokens assuming gpt-3.5-turbo-0301
		tokensPerMessage = 4 // every message follows <im_start>{role/name}\n{content}<im_end>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	case openai.GPT4, openai.GPT40314:
		// gpt-4 may change over time. Returning num tokens assuming gpt-4-0314
		tokensPerMessage = 3
		tokensPerName = 1
	default:
		// Not implemented
		ok = false
		return
	}

	return
}

func countMessageTokens(message openai.ChatCompletionMessage, model string) *int {
	ok, tokensPerMessage, tokensPerName := tokensConfiguration(model)
	if !ok {
		return nil
	}

	enc, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		enc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	}

	tokens := tokensPerMessage
	contentIds, _, _ := enc.Encode(message.Content)
	roleIds, _, _ := enc.Encode(message.Role)
	tokens += len(contentIds)
	tokens += len(roleIds)
	if message.Name != "" {
		tokens += tokensPerName
	}

	return &tokens
}

func countMessagesTokens(messages []openai.ChatCompletionMessage, model string) *int {
	ok, tokensPerMessage, tokensPerName := tokensConfiguration(model)
	if !ok {
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

func countAllMessagesTokens(systemMessage *openai.ChatCompletionMessage, messages []openai.ChatCompletionMessage, model string) *int {
	if systemMessage != nil {
		messages = append(messages, *systemMessage)
	}
	return countMessagesTokens(messages, model)
}
