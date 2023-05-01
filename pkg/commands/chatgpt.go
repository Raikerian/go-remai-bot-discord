package commands

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

const (
	chatGPTCommandName = "chatgpt"

	chatGPTDefaultModel                            = openai.GPT3Dot5Turbo
	chatGPTDiscordChannelMessagesRequestMaxRetries = 4

	// Discord expects the auto_archive_duration to be one of the following values: 60, 1440, 4320, or 10080,
	// which represent the number of minutes before a thread is automatically archived
	// (1 hour, 1 day, 3 days, or 7 days, respectively).
	ChatGPTDiscordThreadAutoArchivewDurationMinutes = 60

	chatGPTPendingMessage = "⌛ Wait a moment, please..."
	chatGPTEmojiAck       = "⌛"
	chatGPTEmojiErr       = "❌"

	chatGPTPricePerTokenGPT3Dot5Turbo = 0.000002
)

type IgnoredChannelsCache map[string]struct{}

type ChatGPTCommandParams struct {
	OpenAIClient         *openai.Client
	MessagesCache        *cache.GPTMessagesCache
	IgnoredChannelsCache *IgnoredChannelsCache
}

type ChatGPTCommandOptionType uint8

const (
	ChatGPTCommandOptionPrompt      ChatGPTCommandOptionType = 1
	ChatGPTCommandOptionContext     ChatGPTCommandOptionType = 2
	ChatGPTCommandOptionModel       ChatGPTCommandOptionType = 3
	ChatGPTCommandOptionTemperature ChatGPTCommandOptionType = 4
)

func (t ChatGPTCommandOptionType) String() string {
	switch t {
	case ChatGPTCommandOptionPrompt:
		return "prompt"
	case ChatGPTCommandOptionContext:
		return "context"
	case ChatGPTCommandOptionModel:
		return "model"
	case ChatGPTCommandOptionTemperature:
		return "temperature"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func (t ChatGPTCommandOptionType) HumanReadableString() string {
	switch t {
	case ChatGPTCommandOptionPrompt:
		return "Prompt"
	case ChatGPTCommandOptionContext:
		return "Context"
	case ChatGPTCommandOptionModel:
		return "Model"
	case ChatGPTCommandOptionTemperature:
		return "Temperature"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func chatGPTHandler(ctx *Context, params *ChatGPTCommandParams) {
	ch, err := ctx.Session.State.Channel(ctx.Interaction.ChannelID)
	if err == nil && ch.IsThread() {
		// ignore interactions invoked in threads
		log.Printf("[GID: %s, i.ID: %s] Interaction was invoked in the existing thread, ignoring\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		return
	}

	log.Printf("[GID: %s, i.ID: %s] ChatGPT interaction invoked by UserID: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, ctx.Interaction.Member.User.ID)

	var prompt string
	if option, ok := ctx.Options[ChatGPTCommandOptionPrompt.String()]; ok {
		prompt = option.StringValue()
	} else {
		// We can't have empty prompt, unfortunately
		// this should not happen, discord prevents empty required options
		log.Printf("[GID: %s, i.ID: %s] Failed to parse prompt option\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		ctx.Respond(&discord.InteractionResponse{
			Type: discord.InteractionResponseChannelMessageWithSource,
			Data: &discord.InteractionResponseData{
				Content: "ERROR: Failed to parse prompt option",
			},
		})
		return
	}

	fields := make([]*discord.MessageEmbedField, 0, 3)
	fields = append(fields, &discord.MessageEmbedField{
		Name:  ChatGPTCommandOptionPrompt.HumanReadableString(),
		Value: prompt,
	})

	// Prepare cache item
	cacheItem := &cache.GPTMessagesCacheData{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	}

	// Set context of the conversation as a system message
	if option, ok := ctx.Options[ChatGPTCommandOptionContext.String()]; ok {
		context := option.StringValue()
		cacheItem.SystemMessage = &openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: context,
		}
		fields = append(fields, &discord.MessageEmbedField{
			Name:  ChatGPTCommandOptionContext.HumanReadableString(),
			Value: context,
		})
		log.Printf("[GID: %s, i.ID: %s] Context provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, context)
	}

	model := chatGPTDefaultModel
	if option, ok := ctx.Options[ChatGPTCommandOptionModel.String()]; ok {
		model = option.StringValue()
		log.Printf("[GID: %s, i.ID: %s] Model provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, model)
	}
	cacheItem.GPTModel = model
	fields = append(fields, &discord.MessageEmbedField{
		Name:  ChatGPTCommandOptionModel.HumanReadableString(),
		Value: model,
	})

	if option, ok := ctx.Options[ChatGPTCommandOptionTemperature.String()]; ok {
		temp := float32(option.FloatValue())
		cacheItem.Temperature = &temp
		fields = append(fields, &discord.MessageEmbedField{
			Name:  ChatGPTCommandOptionTemperature.HumanReadableString(),
			Value: fmt.Sprintf("%g", temp),
		})
		log.Printf("[GID: %s, i.ID: %s] Temperature provided: %g\n", ctx.Interaction.GuildID, ctx.Interaction.ID, temp)
	}

	// Respond to interaction with a reference and user ping
	err = ctx.Respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseChannelMessageWithSource,
		Data: &discord.InteractionResponseData{
			Content: fmt.Sprintf("<@%s>", ctx.Interaction.Member.User.ID),
			Embeds: []*discord.MessageEmbed{
				{
					Title:  "ChatGPT request by " + ctx.Interaction.Member.User.Username + "#" + ctx.Interaction.Member.User.Discriminator,
					Fields: fields,
				},
			},
		},
	})
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Failed to respond to interactrion with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	// Get interaction ID so we can create a thread on top of it
	m, err := ctx.Response()
	if err != nil {
		// Without interaction reference we cannot create a thread with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, i.ID: %s] Failed to get interaction reference with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		ctx.Edit(fmt.Sprintf("Failed to get interaction reference with error: %v", err))
		return
	}

	ch, err = ctx.Session.State.Channel(m.ChannelID)
	if err != nil || ch.IsThread() {
		log.Printf("[GID: %s, i.ID: %s] Interaction reply was in a thread, or there was an error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	thread, err := ctx.Session.MessageThreadStartComplex(m.ChannelID, m.ID, &discord.ThreadStart{
		Name:                "New chat",
		AutoArchiveDuration: ChatGPTDiscordThreadAutoArchivewDurationMinutes,
		Invitable:           false,
	})

	if err != nil {
		// Without thread we cannot reply our answer
		log.Printf("[GID: %s, i.ID: %s] Failed to create a thread with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	// Lock the thread while we are generating ChatGPT answser
	utils.ToggleDiscordThreadLock(ctx.Session, thread.ID, true)

	// Unlock the thread at the end
	defer utils.ToggleDiscordThreadLock(ctx.Session, thread.ID, false)

	channelMessage, err := utils.DiscordChannelMessageSend(ctx.Session, thread.ID, chatGPTPendingMessage, nil)
	if err != nil {
		// Without reply  we cannot edit message with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, i.ID: %s] Failed to reply in the thread with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	params.MessagesCache.Add(thread.ID, cacheItem)

	log.Printf("[GID: %s, i.ID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, cacheItem.GPTModel, len(cacheItem.Messages))
	resp, err := sendChatGPTRequest(params.OpenAIClient, cacheItem)
	if err != nil {
		// ChatGPT failed for whatever reason, tell users about it
		log.Printf("[GID: %s, i.ID: %s] OpenAI request ChatCompletion failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		emptyString := ""
		utils.DiscordChannelMessageEdit(ctx.Session, channelMessage.ID, channelMessage.ChannelID, &emptyString, []*discord.MessageEmbed{
			{
				Title:       "❌ OpenAI API failed",
				Description: err.Error(),
				Color:       0xff0000,
			},
		})
		return
	}

	go generateThreadTitleBasedOnInitialPrompt(ctx, params.OpenAIClient, thread.ID, cacheItem.Messages)

	log.Printf("[GID: %s, i.ID: %s] ChatGPT Request [Model: %s] responded with a usage: [PromptTokens: %d, CompletionTokens: %d, TotalTokens: %d]\n", ctx.Interaction.GuildID, ctx.Interaction.ID, cacheItem.GPTModel, resp.usage.PromptTokens, resp.usage.CompletionTokens, resp.usage.TotalTokens)

	err = utils.DiscordChannelMessageEdit(ctx.Session, channelMessage.ID, channelMessage.ChannelID, &resp.content, nil)
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Discord API failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		emptyString := ""
		utils.DiscordChannelMessageEdit(ctx.Session, channelMessage.ID, channelMessage.ChannelID, &emptyString, []*discord.MessageEmbed{
			{
				Title:       "❌ Discord API Error",
				Description: err.Error(),
				Color:       0xff0000,
			},
		})
		return
	}

	attachUsageInfo(ctx.Session, channelMessage, resp.usage, cacheItem.GPTModel)
}

func chatGPTMessageHandler(ctx *MessageContext, params *ChatGPTCommandParams) (hit bool) {
	if !shouldHandleMessageType(ctx.Message.Type) {
		// ignore message types that should not be handled by this command
		return false
	}

	if ctx.Session.State.User.ID == ctx.Message.Author.ID {
		// ignore self messages
		return false
	}

	if _, exists := (*params.IgnoredChannelsCache)[ctx.Message.ChannelID]; exists {
		// skip over ignored channels list
		return false
	}

	if ctx.Message.Content == "" {
		// ignore messages with empty content
		return false
	}

	ch, err := ctx.Session.State.Channel(ctx.Message.ChannelID)
	if err != nil {
		log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel info with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err)
		return false
	}

	if !ch.IsThread() {
		// ignore non threads
		(*params.IgnoredChannelsCache)[ctx.Message.ChannelID] = struct{}{}
		return false
	}

	if ch.ThreadMetadata != nil && (ch.ThreadMetadata.Locked || ch.ThreadMetadata.Archived) {
		// We don't want to handle messages in locked or archived threads
		log.Printf("[GID: %s, CHID: %s, MID: %s] Ignoring new message in a potential thread as it is locked or/and archived\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
		return false
	}

	log.Printf("[GID: %s, CHID: %s, MID: %s] Handling new message in a potential GPT thread\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)

	cacheItem, ok := params.MessagesCache.Get(ctx.Message.ChannelID)
	if !ok {
		isGPTThread := true
		cacheItem = &cache.GPTMessagesCacheData{}

		var lastID string
		retries := 0
		for {
			if retries >= chatGPTDiscordChannelMessagesRequestMaxRetries {
				// max retries reached
				break
			}
			// Get messages in batches of 100 (maximum allowed by Discord API)
			batch, err := ctx.Session.ChannelMessages(ch.ID, 100, lastID, "", "")
			if err != nil {
				// Since we cannot fetch messages, that means we cannot determine whether this a GPT thread,
				// and if it was, we cannot get the full context to provide a better user experience. Do retries
				// and print the error in the log
				log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages with the error: %v. Retries left: %d\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err, (chatGPTDiscordChannelMessagesRequestMaxRetries - retries))
				retries++
				continue
			}

			transformed := make([]openai.ChatCompletionMessage, 0, len(batch))
			for _, value := range batch {
				role := openai.ChatMessageRoleUser
				if value.Author.ID == ctx.Session.State.User.ID {
					role = openai.ChatMessageRoleAssistant
				}
				content := value.Content
				// First message is always a referenced message
				// Check if it is, and then modify to get the original prompt
				if value.Type == discord.MessageTypeThreadStarterMessage {
					if value.Author.ID != ctx.Session.State.User.ID || value.ReferencedMessage == nil {
						// this is not gpt thread, ignore
						isGPTThread = false
						break
					}
					role = openai.ChatMessageRoleUser

					prompt, context, model, temperature := parseInteractionReply(value.ReferencedMessage)
					if prompt == "" {
						isGPTThread = false
						break
					}
					content = prompt
					var systemMessage *openai.ChatCompletionMessage
					if context != "" {
						systemMessage = &openai.ChatCompletionMessage{
							Role:    openai.ChatMessageRoleSystem,
							Content: context,
						}
					}
					if model == "" {
						model = chatGPTDefaultModel
					}
					if temperature != nil {
						cacheItem.Temperature = temperature
					}

					cacheItem.SystemMessage = systemMessage
					cacheItem.GPTModel = model
				} else if !shouldHandleMessageType(value.Type) {
					// ignore message types that are
					// not related to conversation
					continue
				}
				transformed = append(transformed, openai.ChatCompletionMessage{
					Role:    role,
					Content: content,
				})
			}

			reverseMessages(&transformed)

			// Add the messages to the beginning of the main list
			cacheItem.Messages = append(transformed, cacheItem.Messages...)

			// If there are no more messages in the thread, break the loop
			if len(batch) == 0 {
				break
			}

			// Set the lastID to the last message's ID to get the next batch of messages
			lastID = batch[len(batch)-1].ID
		}

		if retries >= chatGPTDiscordChannelMessagesRequestMaxRetries {
			// max retries reached on fetching messages
			log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages. Reached max retries\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			return false
		}

		if !isGPTThread {
			// this was not a GPT thread
			log.Printf("[GID: %s, CHID: %s, MID: %s] Not a GPT thread, saving to ignored cache to skip over it later\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			// save threadID to ignored cache, so we can always ignore it later
			(*params.IgnoredChannelsCache)[ctx.Message.ChannelID] = struct{}{}
			return false
		}

		params.MessagesCache.Add(ctx.Message.ChannelID, cacheItem)
	} else {
		cacheItem.Messages = append(cacheItem.Messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: ctx.Message.Content,
		})
	}

	// Lock the thread while we are generating ChatGPT answser
	utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, true)
	// Unlock the thread at the end
	defer utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, false)

	ctx.AddReaction(chatGPTEmojiAck)
	defer ctx.RemoveReaction(chatGPTEmojiAck)
	ctx.ChannelTyping()

	log.Printf("[GID: %s, CHID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, cacheItem.GPTModel, len(cacheItem.Messages))
	resp, err := sendChatGPTRequest(params.OpenAIClient, cacheItem)
	if err != nil {
		// ChatGPT failed for whatever reason, tell users about it
		log.Printf("[GID: %s, CHID: %s] ChatGPT request ChatCompletion failed with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, err)
		ctx.AddReaction(chatGPTEmojiErr)
		ctx.EmbedReply(&discord.MessageEmbed{
			Title:       "❌ OpenAI API failed",
			Description: err.Error(),
			Color:       0xff0000,
		})
		return true
	}

	log.Printf("[GID: %s, CHID: %s] ChatGPT Request [Model: %s] responded with a usage: [PromptTokens: %d, CompletionTokens: %d, TotalTokens: %d]\n", ctx.Message.GuildID, ctx.Message.ChannelID, cacheItem.GPTModel, resp.usage.PromptTokens, resp.usage.CompletionTokens, resp.usage.TotalTokens)

	replyMessage, err := ctx.Reply(resp.content)
	if err != nil {
		log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to reply in the thread with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err)
		ctx.AddReaction(chatGPTEmojiErr)
		ctx.EmbedReply(&discord.MessageEmbed{
			Title:       "❌ Discord API Error",
			Description: err.Error(),
			Color:       0xff0000,
		})
		return true
	}

	attachUsageInfo(ctx.Session, replyMessage, resp.usage, cacheItem.GPTModel)

	return true
}

func shouldHandleMessageType(t discord.MessageType) (ok bool) {
	return t == discord.MessageTypeDefault || t == discord.MessageTypeReply
}

func parseInteractionReply(discordMessage *discord.Message) (prompt string, context string, model string, temperature *float32) {
	if discordMessage.Embeds == nil || len(discordMessage.Embeds) == 0 {
		return
	}

	for _, value := range discordMessage.Embeds[0].Fields {
		switch value.Name {
		case ChatGPTCommandOptionPrompt.HumanReadableString():
			prompt = value.Value
		case ChatGPTCommandOptionContext.HumanReadableString():
			context = value.Value
		case ChatGPTCommandOptionModel.HumanReadableString():
			model = value.Value
		case ChatGPTCommandOptionTemperature.HumanReadableString():
			parsedValue, err := strconv.ParseFloat(value.Value, 32)
			if err != nil {
				log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to parse temperature value from the message with the error: %v\n", discordMessage.GuildID, discordMessage.ChannelID, discordMessage.ID, err)
				continue
			}
			temp := float32(parsedValue)
			temperature = &temp
		}
	}

	return
}

func reverseMessages(messages *[]openai.ChatCompletionMessage) {
	length := len(*messages)
	for i := 0; i < length/2; i++ {
		(*messages)[i], (*messages)[length-i-1] = (*messages)[length-i-1], (*messages)[i]
	}
}

type chatGPTResponse struct {
	content string
	usage   openai.Usage
}

func sendChatGPTRequest(client *openai.Client, cacheItem *cache.GPTMessagesCacheData) (*chatGPTResponse, error) {
	// Create message with ChatGPT
	messages := cacheItem.Messages
	if cacheItem.SystemMessage != nil {
		messages = append([]openai.ChatCompletionMessage{*cacheItem.SystemMessage}, messages...)
	}

	req := openai.ChatCompletionRequest{
		Model:    cacheItem.GPTModel,
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
	return &chatGPTResponse{
		content: responseContent,
		usage:   resp.Usage,
	}, nil
}

func generateThreadTitleBasedOnInitialPrompt(ctx *Context, client *openai.Client, threadID string, messages []openai.ChatCompletionMessage) {
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
		MaxTokens:   50,
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
	extraInfo := fmt.Sprintf("Completion Tokens: %d, Total: %d", usage.CompletionTokens, usage.TotalTokens)
	if model == openai.GPT3Dot5Turbo {
		extraInfo += fmt.Sprintf("\nLLM Cost: $%f", float64(usage.TotalTokens)*chatGPTPricePerTokenGPT3Dot5Turbo)
	}
	utils.DiscordChannelMessageEdit(s, m.ID, m.ChannelID, nil, []*discord.MessageEmbed{
		{
			Fields: []*discord.MessageEmbedField{
				{
					Name:  "Usage",
					Value: extraInfo,
				},
			},
		},
	})
}

func ChatGPTCommand(params *ChatGPTCommandParams) *Command {
	temperatureOptionMinValue := 0.0
	return &Command{
		Name:                     chatGPTCommandName,
		Description:              "Start conversation with ChatGPT",
		DMPermission:             false,
		DefaultMemberPermissions: discord.PermissionViewChannel,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        ChatGPTCommandOptionPrompt.String(),
				Description: "ChatGPT prompt",
				Required:    true,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        ChatGPTCommandOptionContext.String(),
				Description: "Sets context that guides the AI assistant's behavior during the conversation",
				Required:    false,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        ChatGPTCommandOptionModel.String(),
				Description: "GPT model",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  "GPT-3.5-Turbo (Default)",
						Value: openai.GPT3Dot5Turbo,
					},
					{
						Name:  "GPT-4",
						Value: openai.GPT4,
					},
				},
			},
			{
				Type:        discord.ApplicationCommandOptionNumber,
				Name:        ChatGPTCommandOptionTemperature.String(),
				Description: "What sampling temperature to use, between 0 and 2. Lower - more focused and deterministic",
				MinValue:    &temperatureOptionMinValue,
				MaxValue:    2.0,
				Required:    false,
			},
		},
		Handler: HandlerFunc(func(ctx *Context) {
			chatGPTHandler(ctx, params)
		}),
		MessageHandler: MessageHandlerFunc(func(ctx *MessageContext) bool {
			return chatGPTMessageHandler(ctx, params)
		}),
	}
}
