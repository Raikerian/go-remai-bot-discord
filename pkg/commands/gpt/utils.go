package gpt

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

// See https://openai.com/pricing
const (
	gptPricePerPromptTokenGPT3Dot5Turbo16K     = 0.000001
	gptPricePerCompletionTokenGPT3Dot5Turbo16K = 0.000002

	gptPricePerPromptTokenGPT4Turbo     = 0.00001
	gptPricePerCompletionTokenGPT4Turbo = 0.00003
)

const (
	gptTruncateLimitGPT3Dot5Turbo16K = 14000
	gptTruncateLimitGPT4Turbo        = 20000
)

func shouldHandleMessageType(t discord.MessageType) bool {
	return t == discord.MessageTypeDefault || t == discord.MessageTypeReply
}

type chatGPTResponse struct {
	content string
	usage   openai.Usage
}

func sendChatGPTRequest(client *openai.Client, cacheItem *MessagesCacheData) (*chatGPTResponse, error) {
	// Create message with ChatGPT
	messages := cacheItem.Messages
	if cacheItem.SystemMessage != nil {
		messages = append([]openai.ChatCompletionMessage{*cacheItem.SystemMessage}, messages...)
	}

	req := openai.ChatCompletionRequest{
		Model:    cacheItem.Model,
		Messages: messages,
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

func getUrlData(client *http.Client, url string) (string, error) {
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	content, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func getContentOrURLData(client *http.Client, s string) (content string, err error) {
	if utils.IsURL(s) {
		content, err = getUrlData(client, s)
	}
	return content, err
}

func parseInteractionReply(discordMessage *discord.Message) (prompt string, context string, model string, temperature *float32) {
	if discordMessage.Embeds == nil || len(discordMessage.Embeds) == 0 {
		return
	}

	for _, embed := range discordMessage.Embeds {
		if embed.Description != "" {
			prompt = embed.Description
		}
		for _, field := range embed.Fields {
			switch field.Name {
			case gptCommandOptionPrompt.humanReadableString():
				prompt = field.Value
			case gptCommandOptionContext.humanReadableString():
				if context == "" {
					// file context always gets precedence
					context = field.Value
				}
			case gptCommandOptionContextFile.humanReadableString():
				context = field.Value
			case gptCommandOptionModel.humanReadableString():
				model = field.Value
			case gptCommandOptionTemperature.humanReadableString():
				parsedValue, err := strconv.ParseFloat(field.Value, 32)
				if err != nil {
					log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to parse temperature value from the message with the error: %v\n", discordMessage.GuildID, discordMessage.ChannelID, discordMessage.ID, err)
					continue
				}
				temp := float32(parsedValue)
				temperature = &temp
			}
		}
	}

	return
}

func modelTruncateLimit(model string) *int {
	var truncateLimit int
	switch model {
	case openai.GPT3Dot5Turbo16K:
		truncateLimit = gptTruncateLimitGPT3Dot5Turbo16K
	case constants.GPT4TurboPreview:
		truncateLimit = gptTruncateLimitGPT4Turbo
	default:
		// Not implemented
		return nil
	}
	return &truncateLimit
}

func adjustMessageTokens(cacheItem *MessagesCacheData) {
	truncateLimit := modelTruncateLimit(cacheItem.Model)
	if truncateLimit == nil {
		return
	}

	for cacheItem.TokenCount > *truncateLimit {
		message := cacheItem.Messages[0]
		cacheItem.Messages = cacheItem.Messages[1:]
		removedTokens := countMessageTokens(message, cacheItem.Model)
		if removedTokens == nil {
			return
		}
		cacheItem.TokenCount -= *removedTokens
	}
}

func isCacheItemWithinTruncateLimit(cacheItem *MessagesCacheData) (ok bool, count int) {
	truncateLimit := modelTruncateLimit(cacheItem.Model)
	if truncateLimit == nil {
		return true, 0
	}

	tokens := countAllMessagesTokens(cacheItem.SystemMessage, cacheItem.Messages, cacheItem.Model)
	if tokens == nil {
		return true, 0
	}
	cacheItem.TokenCount = *tokens

	return *tokens <= *truncateLimit, *tokens
}

func generateThreadTitleBasedOnInitialPrompt(ctx *bot.Context, client *openai.Client, threadID string, messages []openai.ChatCompletionMessage) {
	conversation := make([]map[string]string, len(messages))
	for i, msg := range messages {
		conversation[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Combine the conversation messages into a single string
	var conversationTextBuilder strings.Builder
	for _, msg := range conversation {
		conversationTextBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg["role"], msg["content"]))
	}
	conversationText := conversationTextBuilder.String()

	// Create a prompt that asks the model to generate a title
	prompt := fmt.Sprintf("%s\nGenerate a short and concise title summarizing the conversation in the same language. The title must not contain any quotes. The title should be no longer than 60 characters:", conversationText)

	resp, err := client.CreateCompletion(context.Background(), openai.CompletionRequest{
		Model:       openai.GPT3Dot5TurboInstruct,
		Prompt:      prompt,
		Temperature: 0.5,
		MaxTokens:   75,
	})
	if err != nil {
		log.Printf("[GID: %s, threadID: %s] Failed to generate thread title with the error: %v\n", ctx.Interaction.GuildID, threadID, err)
		return
	}

	_, err = ctx.Session.ChannelEditComplex(threadID, &discord.ChannelEdit{
		Name: resp.Choices[0].Text,
	})
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Failed to update thread title with the error: %v\n", ctx.Interaction.GuildID, threadID, err)
	}
}

func attachUsageInfo(s *discord.Session, m *discord.Message, usage openai.Usage, model string) {
	extraInfo := fmt.Sprintf("Completion Tokens: %d, Total: %d%s", usage.CompletionTokens, usage.TotalTokens, generateCost(usage, model))

	utils.DiscordChannelMessageEdit(s, m.ID, m.ChannelID, nil, []*discord.MessageEmbed{
		{
			Footer: &discord.MessageEmbedFooter{
				Text:    extraInfo,
				IconURL: constants.OpenAIBlackIconURL,
			},
		},
	})
}

func generateCost(usage openai.Usage, model string) string {
	var cost float64

	switch model {
	case openai.GPT3Dot5Turbo16K:
		cost = float64(usage.PromptTokens)*gptPricePerPromptTokenGPT3Dot5Turbo16K + float64(usage.CompletionTokens)*gptPricePerCompletionTokenGPT3Dot5Turbo16K
	case constants.GPT4TurboPreview:
		cost = float64(usage.PromptTokens)*gptPricePerPromptTokenGPT4Turbo + float64(usage.CompletionTokens)*gptPricePerCompletionTokenGPT4Turbo
	default:
		// Not implemented
		return ""
	}

	return fmt.Sprintf("\nLLM Cost: $%f", cost)
}
