package gpt

import (
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

var gptDefaultModel = openai.GPT3Dot5Turbo

const (
	gptCommandName = "gpt"

	gptDiscordChannelMessagesRequestMaxRetries = 4

	// Discord expects the auto_archive_duration to be one of the following values: 60, 1440, 4320, or 10080,
	// which represent the number of minutes before a thread is automatically archived
	// (1 hour, 1 day, 3 days, or 7 days, respectively).
	gptDiscordThreadAutoArchivewDurationMinutes = 60

	gptPendingMessage = "⌛ Wait a moment, please..."
	gptEmojiAck       = "⌛"
	gptEmojiErr       = "❌"
)

func chatGPTHandler(ctx *bot.Context, client *openai.Client, messagesCache *MessagesCache) {
	ch, err := ctx.Session.State.Channel(ctx.Interaction.ChannelID)
	if err == nil && ch.IsThread() {
		// ignore interactions invoked in threads
		log.Printf("[GID: %s, i.ID: %s] Interaction was invoked in the existing thread, ignoring\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		return
	}

	log.Printf("[GID: %s, i.ID: %s] ChatGPT interaction invoked by UserID: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, ctx.Interaction.Member.User.ID)

	var prompt string
	if option, ok := ctx.Options[gptCommandOptionPrompt.string()]; ok {
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
		Name:  gptCommandOptionPrompt.humanReadableString(),
		Value: prompt,
	})

	// Prepare cache item
	cacheItem := &MessagesCacheData{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	}

	// Set context of the conversation as a system message
	if option, ok := ctx.Options[gptCommandOptionContext.string()]; ok {
		context := option.StringValue()
		cacheItem.SystemMessage = &openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: context,
		}
		fields = append(fields, &discord.MessageEmbedField{
			Name:  gptCommandOptionContext.humanReadableString(),
			Value: context,
		})
		log.Printf("[GID: %s, i.ID: %s] Context provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, context)
	}

	model := gptDefaultModel
	if option, ok := ctx.Options[gptCommandOptionModel.string()]; ok {
		model = option.StringValue()
		log.Printf("[GID: %s, i.ID: %s] Model provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, model)
	}
	cacheItem.Model = model
	fields = append(fields, &discord.MessageEmbedField{
		Name:  gptCommandOptionModel.humanReadableString(),
		Value: model,
	})

	if option, ok := ctx.Options[gptCommandOptionTemperature.string()]; ok {
		temp := float32(option.FloatValue())
		cacheItem.Temperature = &temp
		fields = append(fields, &discord.MessageEmbedField{
			Name:  gptCommandOptionTemperature.humanReadableString(),
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
					Title:  "OpenAI GPT request by " + ctx.Interaction.Member.User.Username + "#" + ctx.Interaction.Member.User.Discriminator,
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
		AutoArchiveDuration: gptDiscordThreadAutoArchivewDurationMinutes,
		Invitable:           false,
	})

	if err != nil {
		// Without thread we cannot reply our answer
		log.Printf("[GID: %s, i.ID: %s] Failed to create a thread with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	// Lock the thread while we are generating ChatGPT answser
	utils.ToggleDiscordThreadLock(ctx.Session, thread.ID, true)

	channelMessage, err := utils.DiscordChannelMessageSend(ctx.Session, thread.ID, gptPendingMessage, nil)
	if err != nil {
		// Without reply  we cannot edit message with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, i.ID: %s] Failed to reply in the thread with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	messagesCache.Add(thread.ID, cacheItem)

	log.Printf("[GID: %s, i.ID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, cacheItem.Model, len(cacheItem.Messages))
	resp, err := sendChatGPTRequest(client, cacheItem)
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

	// Unlock the thread at the end
	defer utils.ToggleDiscordThreadLock(ctx.Session, thread.ID, false)

	go generateThreadTitleBasedOnInitialPrompt(ctx, client, thread.ID, cacheItem.Messages)

	log.Printf("[GID: %s, i.ID: %s] ChatGPT Request [Model: %s] responded with a usage: [PromptTokens: %d, CompletionTokens: %d, TotalTokens: %d]\n", ctx.Interaction.GuildID, ctx.Interaction.ID, cacheItem.Model, resp.usage.PromptTokens, resp.usage.CompletionTokens, resp.usage.TotalTokens)

	messages := splitMessage(resp.content)
	err = utils.DiscordChannelMessageEdit(ctx.Session, channelMessage.ID, channelMessage.ChannelID, &messages[0], nil)
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

	if len(messages) > 1 {
		// if there are more messages, send them as a thread reply
		for _, message := range messages[1:] {
			channelMessage, err = utils.DiscordChannelMessageSend(ctx.Session, thread.ID, message, nil)
			if err != nil {
				log.Printf("[GID: %s, i.ID: %s] Discord API failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
			}
		}
	}

	attachUsageInfo(ctx.Session, channelMessage, resp.usage, cacheItem.Model)
}

func chatGPTMessageHandler(ctx *bot.MessageContext, client *openai.Client, messagesCache *MessagesCache, ignoredChannelsCache *IgnoredChannelsCache) {
	if !shouldHandleMessageType(ctx.Message.Type) {
		// ignore message types that should not be handled by this command
		return
	}

	if ctx.Session.State.User.ID == ctx.Message.Author.ID {
		// ignore self messages
		return
	}

	if _, exists := (*ignoredChannelsCache)[ctx.Message.ChannelID]; exists {
		// skip over ignored channels list
		return
	}

	if ctx.Message.Content == "" {
		// ignore messages with empty content
		return
	}

	ch, err := ctx.Session.State.Channel(ctx.Message.ChannelID)
	if err != nil {
		log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel info with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err)
		return
	}

	if !ch.IsThread() {
		// ignore non threads
		(*ignoredChannelsCache)[ctx.Message.ChannelID] = struct{}{}
		return
	}

	if ch.ThreadMetadata != nil && (ch.ThreadMetadata.Locked || ch.ThreadMetadata.Archived) {
		// We don't want to handle messages in locked or archived threads
		log.Printf("[GID: %s, CHID: %s, MID: %s] Ignoring new message in a potential thread as it is locked or/and archived\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
		return
	}

	log.Printf("[GID: %s, CHID: %s, MID: %s] Handling new message in a potential GPT thread\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)

	cacheItem, ok := messagesCache.Get(ctx.Message.ChannelID)
	if !ok {
		isGPTThread := true
		cacheItem = &MessagesCacheData{}

		var lastID string
		retries := 0
		for {
			if retries >= gptDiscordChannelMessagesRequestMaxRetries {
				// max retries reached
				break
			}
			// Get messages in batches of 100 (maximum allowed by Discord API)
			batch, err := ctx.Session.ChannelMessages(ch.ID, 100, lastID, "", "")
			if err != nil {
				// Since we cannot fetch messages, that means we cannot determine whether this a GPT thread,
				// and if it was, we cannot get the full context to provide a better user experience. Do retries
				// and print the error in the log
				log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages with the error: %v. Retries left: %d\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err, (gptDiscordChannelMessagesRequestMaxRetries - retries))
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
						model = gptDefaultModel
					}
					if temperature != nil {
						cacheItem.Temperature = temperature
					}

					cacheItem.SystemMessage = systemMessage
					cacheItem.Model = model
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

		if retries >= gptDiscordChannelMessagesRequestMaxRetries {
			// max retries reached on fetching messages
			log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages. Reached max retries\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			return
		}

		if !isGPTThread {
			// this was not a GPT thread
			log.Printf("[GID: %s, CHID: %s, MID: %s] Not a GPT thread, saving to ignored cache to skip over it later\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			// save threadID to ignored cache, so we can always ignore it later
			(*ignoredChannelsCache)[ctx.Message.ChannelID] = struct{}{}
			return
		}

		messagesCache.Add(ctx.Message.ChannelID, cacheItem)
	} else {
		cacheItem.Messages = append(cacheItem.Messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: ctx.Message.Content,
		})
	}

	// adjust messages to fit token limit of the model
	tokenCount := countAllTokens(cacheItem.SystemMessage, cacheItem.Messages, cacheItem.Model)
	if tokenCount != nil {
		cacheItem.TokenCount = *tokenCount
		adjustMessageTokens(cacheItem)
	}

	// Lock the thread while we are generating ChatGPT answser
	utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, true)
	// Unlock the thread at the end
	defer utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, false)

	ctx.AddReaction(gptEmojiAck)
	defer ctx.RemoveReaction(gptEmojiAck)
	ctx.ChannelTyping()

	log.Printf("[GID: %s, CHID: %s] ChatGPT Request invoked with [Model: %s]. Current cache size: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, cacheItem.Model, len(cacheItem.Messages))
	resp, err := sendChatGPTRequest(client, cacheItem)
	if err != nil {
		// ChatGPT failed for whatever reason, tell users about it
		log.Printf("[GID: %s, CHID: %s] ChatGPT request ChatCompletion failed with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, err)
		ctx.AddReaction(gptEmojiErr)
		ctx.EmbedReply(&discord.MessageEmbed{
			Title:       "❌ OpenAI API failed",
			Description: err.Error(),
			Color:       0xff0000,
		})
		return
	}

	log.Printf("[GID: %s, CHID: %s] ChatGPT Request [Model: %s] responded with a usage: [PromptTokens: %d, CompletionTokens: %d, TotalTokens: %d]\n", ctx.Message.GuildID, ctx.Message.ChannelID, cacheItem.Model, resp.usage.PromptTokens, resp.usage.CompletionTokens, resp.usage.TotalTokens)

	messages := splitMessage(resp.content)
	var replyMessage *discord.Message
	for _, message := range messages {
		replyMessage, err = ctx.Reply(message)
		if err != nil {
			log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to reply in the thread with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err)
			ctx.AddReaction(gptEmojiErr)
			ctx.EmbedReply(&discord.MessageEmbed{
				Title:       "❌ Discord API Error",
				Description: err.Error(),
				Color:       0xff0000,
			})
			return
		}
	}

	attachUsageInfo(ctx.Session, replyMessage, resp.usage, cacheItem.Model)
}

func Command(client *openai.Client, completionModels []string, messagesCache *MessagesCache, ignoredChannelsCache *IgnoredChannelsCache) *bot.Command {
	temperatureOptionMinValue := 0.0
	opts := []*discord.ApplicationCommandOption{
		{
			Type:        discord.ApplicationCommandOptionString,
			Name:        gptCommandOptionPrompt.string(),
			Description: "ChatGPT prompt",
			Required:    true,
		},
		{
			Type:        discord.ApplicationCommandOptionString,
			Name:        gptCommandOptionContext.string(),
			Description: "Sets context that guides the AI assistant's behavior during the conversation",
			Required:    false,
		},
	}
	numberOfModels := len(completionModels)
	if numberOfModels > 0 {
		gptDefaultModel = completionModels[0] // set first model as default one
	}
	if numberOfModels > 1 {
		var modelChoices []*discord.ApplicationCommandOptionChoice
		for i, model := range completionModels {
			name := model
			if i == 0 {
				name += " (Default)"
			}
			modelChoices = append(modelChoices, &discord.ApplicationCommandOptionChoice{
				Name:  name,
				Value: model,
			})
		}
		opts = append(opts, &discord.ApplicationCommandOption{
			Type:        discord.ApplicationCommandOptionString,
			Name:        gptCommandOptionModel.string(),
			Description: "GPT model",
			Required:    false,
			Choices:     modelChoices,
		})
	}
	opts = append(opts, &discord.ApplicationCommandOption{
		Type:        discord.ApplicationCommandOptionNumber,
		Name:        gptCommandOptionTemperature.string(),
		Description: "What sampling temperature to use, between 0.0 and 2.0. Lower - more focused and deterministic",
		MinValue:    &temperatureOptionMinValue,
		MaxValue:    2.0,
		Required:    false,
	})
	return &bot.Command{
		Name:        gptCommandName,
		Description: "Start conversation with ChatGPT",
		Options:     opts,
		Handler: bot.HandlerFunc(func(ctx *bot.Context) {
			chatGPTHandler(ctx, client, messagesCache)
		}),
		MessageHandler: bot.MessageHandlerFunc(func(ctx *bot.MessageContext) {
			chatGPTMessageHandler(ctx, client, messagesCache, ignoredChannelsCache)
		}),
	}
}
