package cache

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/sashabaranov/go-openai"
)

// type MessagesCache lru.Cache[string, *MessagesCacheData]
type MessagesCache struct {
	*lru.Cache[string, *MessagesCacheData]
}

type MessagesCacheInteractionType uint8

const (
	MessagesCacheInteractionChatGPT MessagesCacheInteractionType = 1
)

func (t MessagesCacheInteractionType) String() string {
	switch t {
	case MessagesCacheInteractionChatGPT:
		return constants.CommandTypeChatGPT
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

type MessagesCacheData struct {
	Messages        []openai.ChatCompletionMessage
	SystemMessage   *openai.ChatCompletionMessage
	InteractionType MessagesCacheInteractionType
}

func NewMessagesCache(size int) (*MessagesCache, error) {
	lruCache, err := lru.New[string, *MessagesCacheData](constants.DiscordThreadsCacheSize)
	if err != nil {
		return nil, err
	}

	return &MessagesCache{
		Cache: lruCache,
	}, nil
}
