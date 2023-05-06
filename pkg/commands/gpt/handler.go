package gpt

import (
	"fmt"
	"io"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

const (
	// Discord expects the auto_archive_duration to be one of the following values: 60, 1440, 4320, or 10080,
	// which represent the number of minutes before a thread is automatically archived
	// (1 hour, 1 day, 3 days, or 7 days, respectively).
	gptDiscordThreadAutoArchivewDurationMinutes = 60

	gptInteractionEmbedColor = 0x000000
	gptPendingMessage        = "⌛ Wait a moment, please..."
)

func chatGPTHandler(ctx *bot.Context, client *openai.Client, messagesCache *MessagesCache) {
	ch, err := ctx.Session.State.Channel(ctx.Interaction.ChannelID)
	if err == nil && ch.IsThread() {
		// ignore interactions invoked in threads
		log.Printf("[GID: %s, i.ID: %s] Interaction was invoked in the existing thread, ignoring\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		return
	}

	log.Printf("[GID: %s, i.ID: %s] ChatGPT interaction invoked by UserID: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, ctx.Interaction.Member.User.ID)

	err = ctx.Respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Failed to respond to interactrion with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	var prompt string
	if option, ok := ctx.Options[gptCommandOptionPrompt.string()]; ok {
		prompt = option.StringValue()
	} else {
		// We can't have empty prompt, unfortunately
		// this should not happen, discord prevents empty required options
		log.Printf("[GID: %s, i.ID: %s] Failed to parse prompt option\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ Error",
					Description: "Failed to parse prompt option",
					Color:       0xff0000,
				},
			},
		})
		return
	}

	fields := make([]*discord.MessageEmbedField, 0, 4)
	fields = append(fields, &discord.MessageEmbedField{
		Value: "\u200B",
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

	// Set context of the conversation as a system message. File option takes precedence
	if option, ok := ctx.Options[gptCommandOptionContextFile.string()]; ok {
		attachmentID := option.Value.(string)
		attachmentUrl := ctx.Interaction.ApplicationCommandData().Resolved.Attachments[attachmentID].URL
		res, err := ctx.Client.Get(attachmentUrl)
		if err != nil {
			ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
				Embeds: []*discord.MessageEmbed{
					{
						Title:       "❌ Failed to get attachment data",
						Description: err.Error(),
						Color:       0xff0000,
					},
				},
			})
			return
		}

		defer res.Body.Close()
		content, err := io.ReadAll(res.Body)
		if err != nil {
			ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
				Embeds: []*discord.MessageEmbed{
					{
						Title:       "❌ Failed to read attachment content",
						Description: err.Error(),
						Color:       0xff0000,
					},
				},
			})
			return
		}

		cacheItem.SystemMessage = &openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: string(content),
		}

		fields = append(fields, &discord.MessageEmbedField{
			Name:  gptCommandOptionContextFile.humanReadableString(),
			Value: attachmentUrl,
		})

		log.Printf("[GID: %s, i.ID: %s] Context file provided: [AID: %s]\n", ctx.Interaction.GuildID, ctx.Interaction.ID, attachmentID)
	} else if option, ok := ctx.Options[gptCommandOptionContext.string()]; ok {
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
	ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
		Embeds: []*discord.MessageEmbed{
			{
				Description: prompt,
				Color:       gptInteractionEmbedColor,
				Author: &discord.MessageEmbedAuthor{
					Name:         "OpenAI chat request by " + ctx.Interaction.Member.User.Username,
					IconURL:      ctx.Interaction.Member.User.AvatarURL("32"),
					ProxyIconURL: constants.OpenAIBlackIconURL,
				},
				Fields: fields,
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
