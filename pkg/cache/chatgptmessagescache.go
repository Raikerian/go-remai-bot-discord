package cache

import "github.com/sashabaranov/go-openai"

type ChatGPTMessagesCache struct {
	Messages      []openai.ChatCompletionMessage
	SystemMessage *openai.ChatCompletionMessage
}
