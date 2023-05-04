package chat

import (
	"github.com/sashabaranov/go-openai"
	"github.com/tiktoken-go/tokenizer"
)

func tokenCount(messages []openai.ChatCompletionMessage, model string) *int {
	enc, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		enc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	}

	var tokensPerMessage int
	var tokensPerName int
	switch model {
	case openai.GPT3Dot5Turbo:
		tokensPerMessage = 4 // every message follows <im_start>{role/name}\n{content}<im_end>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	case openai.GPT4:
		tokensPerMessage = 3
		tokensPerName = 1
	default:
		// Not implemented
		return nil
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
