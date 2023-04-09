package cache

import (
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/sashabaranov/go-openai"
)

type GPTMessagesCache struct {
	*lru.Cache[string, *GPTMessagesCacheData]
}

type GPTMessagesCacheData struct {
	Messages      []openai.ChatCompletionMessage
	SystemMessage *openai.ChatCompletionMessage
	GPTModel      string
}

func NewGPTMessagesCache(size int) (*GPTMessagesCache, error) {
	lruCache, err := lru.New[string, *GPTMessagesCacheData](constants.DiscordThreadsCacheSize)
	if err != nil {
		return nil, err
	}

	return &GPTMessagesCache{
		Cache: lruCache,
	}, nil
}
