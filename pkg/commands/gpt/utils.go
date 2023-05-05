package gpt

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

// See https://openai.com/pricing
const (
	gptPricePerTokenGPT3Dot5Turbo0301 = 0.000002

	gptPricePerPromptTokenGPT40314     = 0.00003
	gptPricePerCompletionTokenGPT40314 = 0.00006

	gptPricePerPromptTokenGPT432K0314     = 0.00006
	gptPricePerCompletionTokenGPT432K0314 = 0.00012
)

const (
	gptTruncateLimitGPT3Dot5Turbo0301 = 3500
	gptTruncateLimitGPT40314          = 6500
	gptTruncateLimitGPT432K0314       = 30500
)

const gptOpenAIBlackIconURL = "https://ph-files.imgix.net/b739ac93-2899-4cc1-a893-40ea8afde77e.png"

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

func adjustMessageTokens(cacheItem *MessagesCacheData) {
	var truncateLimit int
	switch cacheItem.Model {
	case openai.GPT3Dot5Turbo, openai.GPT3Dot5Turbo0301:
		// gpt-3.5-turbo may change over time. Assigning truncate limit assuming gpt-3.5-turbo-0301
		truncateLimit = gptTruncateLimitGPT3Dot5Turbo0301
	case openai.GPT4, openai.GPT40314:
		// gpt-4 may change over time. Assigning truncate limit assuming gpt-4-0314
		truncateLimit = gptTruncateLimitGPT40314
	case openai.GPT432K, openai.GPT432K0314:
		// gpt-4-32k may change over time. Assigning truncate limit assuming gpt-4-32k-0314
		truncateLimit = gptTruncateLimitGPT432K0314
	default:
		// Not implemented
		return
	}

	for cacheItem.TokenCount > truncateLimit {
		cacheItem.Messages = cacheItem.Messages[1:]
		tokens := countAllTokens(cacheItem.SystemMessage, cacheItem.Messages, cacheItem.Model)
		if tokens == nil {
			return
		}
		cacheItem.TokenCount = *tokens
	}
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
		Model:       openai.GPT3TextDavinci003,
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
				IconURL: gptOpenAIBlackIconURL,
			},
		},
	})
}

func generateCost(usage openai.Usage, model string) string {
	var cost float64

	switch model {
	case openai.GPT3Dot5Turbo, openai.GPT3Dot5Turbo0301:
		// gpt-3.5-turbo may change over time. Calculating usage assuming gpt-3.5-turbo-0301
		cost = float64(usage.TotalTokens) * gptPricePerTokenGPT3Dot5Turbo0301
	case openai.GPT4, openai.GPT40314:
		// gpt-4 may change over time. Calculating usage assuming gpt-4-0314
		cost = float64(usage.PromptTokens)*gptPricePerPromptTokenGPT40314 + float64(usage.CompletionTokens)*gptPricePerCompletionTokenGPT40314
	case openai.GPT432K, openai.GPT432K0314:
		// gpt-4-32k may change over time. Calculating usage assuming gpt-4-32k-0314
		cost = float64(usage.PromptTokens)*gptPricePerPromptTokenGPT432K0314 + float64(usage.CompletionTokens)*gptPricePerCompletionTokenGPT432K0314
	default:
		// Not implemented
		return ""
	}

	return fmt.Sprintf("\nLLM Cost: $%f", cost)
}
