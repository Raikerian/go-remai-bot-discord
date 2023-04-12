package commands

import (
	"fmt"
	"log"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

type IgnoredChannelsCache map[string]struct{}

type ChatGPTCommandParams struct {
	OpenAIClient         *openai.Client
	MessagesCache        *cache.GPTMessagesCache
	IgnoredChannelsCache *IgnoredChannelsCache
}

const DiscordChannelMessagesRequestMaxRetries = 4

type ChatGPTCommandOptionType uint8

const (
	ChatGPTCommandOptionPrompt  ChatGPTCommandOptionType = 1
	ChatGPTCommandOptionContext ChatGPTCommandOptionType = 2
	ChatGPTCommandOptionModel   ChatGPTCommandOptionType = 3
)

func (t ChatGPTCommandOptionType) String() string {
	switch t {
	case ChatGPTCommandOptionPrompt:
		return "prompt"
	case ChatGPTCommandOptionContext:
		return "context"
	case ChatGPTCommandOptionModel:
		return "model"
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
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func handler(ctx *Context, params *ChatGPTCommandParams) {
	ch, err := ctx.Session.State.Channel(ctx.Interaction.ChannelID)
	if err == nil && ch.IsThread() {
		// ignore interactions invoked in threads
		log.Printf("[GID: %s, i.ID: %s] Interaction was invoked in the existing thread, ignoring\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		return
	}

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

	// Set context of the conversation as a system message
	var context string
	if option, ok := ctx.Options[ChatGPTCommandOptionContext.String()]; ok {
		context = option.StringValue()
		// response += fmt.Sprintf("\nand provided the following context:\n> %s", context)
		fields = append(fields, &discord.MessageEmbedField{
			Name:  ChatGPTCommandOptionContext.HumanReadableString(),
			Value: context,
		})
		log.Printf("[GID: %s, i.ID: %s] Context provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, context)
	}

	model := constants.DefaultGPTModel
	if option, ok := ctx.Options[ChatGPTCommandOptionModel.String()]; ok {
		model = option.StringValue()
		log.Printf("[GID: %s, i.ID: %s] Model provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, model)
	}
	fields = append(fields, &discord.MessageEmbedField{
		Name:  ChatGPTCommandOptionModel.HumanReadableString(),
		Value: model,
	})

	// Respond to interaction with a reference and user ping
	ctx.Respond(&discord.InteractionResponse{
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

	// Get interaction ID so we can create a thread on top of it
	m, err := ctx.Session.InteractionResponse(ctx.Interaction)
	if err != nil {
		// Without interaction reference we cannot create a thread with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, i.ID: %s] Failed to get interaction reference with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		ctx.Edit(fmt.Sprintf("Failed to get interaction reference with error: %v", err))
		return
	}

	ch, err = ctx.Session.State.Channel(m.ChannelID)
	if err != nil || !ch.IsThread() {
		log.Printf("[GID: %s, i.ID: %s] Message reply was not in a thread, or there was an error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	thread, err := ctx.Session.MessageThreadStartComplex(m.ChannelID, m.ID, &discord.ThreadStart{
		Name:                model + " conversation with " + ctx.Interaction.Member.User.Username,
		AutoArchiveDuration: constants.DiscordThreadAutoArchivewDurationMinutes,
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

	channelMessage, err := utils.DiscordChannelMessageSend(ctx.Session, thread.ID, constants.GenericPendingMessage, nil)
	if err != nil {
		// Without reply  we cannot edit message with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, i.ID: %s] Failed to reply in the thread with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	// Set context of the conversation as a system message
	cache := &cache.GPTMessagesCacheData{
		GPTModel: model,
	}
	params.MessagesCache.Add(thread.ID, cache)
	if context != "" {
		cache.SystemMessage = &openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: context,
		}
	}

	// TODO: make ChatGPT request
	print(channelMessage)
}

func messageHandler(ctx *MessageContext, params *ChatGPTCommandParams) (hit bool) {
	if ctx.Session.State.User.ID == ctx.Message.Author.ID {
		// ignore self messages
		return false
	}

	if _, exists := (*params.IgnoredChannelsCache)[ctx.Message.ChannelID]; exists {
		// skip over ignored channels list
		return
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

	if !params.MessagesCache.Contains(ctx.Message.ChannelID) {
		isGPTThread := true

		var lastID string
		retries := 0
		for {
			if retries >= DiscordChannelMessagesRequestMaxRetries {
				// max retries reached
				break
			}
			// Get messages in batches of 100 (maximum allowed by Discord API)
			batch, err := ctx.Session.ChannelMessages(ch.ID, 100, lastID, "", "")
			if err != nil {
				// Since we cannot fetch messages, that means we cannot determine whether this a GPT thread,
				// and if it was, we cannot get the full context to provide a better user experience. Do retries
				// and print the error in the log
				log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages with the error: %v. Retries left: %d\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err, (DiscordChannelMessagesRequestMaxRetries - retries))
				retries++
				continue
			}

			transformed := make([]openai.ChatCompletionMessage, 0, len(batch))
			for _, value := range batch {
				if value.ID == ctx.Message.ID {
					// avoid adding current message
					continue
				}
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

					prompt, context, model := parseInteractionReply(value.ReferencedMessage)
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
						model = constants.DefaultGPTModel
					}

					if item, ok := params.MessagesCache.Get(ctx.Message.ChannelID); ok {
						item.SystemMessage = systemMessage
						item.GPTModel = model
					} else {
						params.MessagesCache.Add(ctx.Message.ChannelID, &cache.GPTMessagesCacheData{
							SystemMessage: systemMessage,
							GPTModel:      model,
						})
					}
				}
				transformed = append(transformed, openai.ChatCompletionMessage{
					Role:    role,
					Content: content,
				})
			}

			reverseMessages(&transformed)

			// Add the messages to the beginning of the main list
			if item, ok := params.MessagesCache.Get(ctx.Message.ChannelID); ok {
				item.Messages = append(transformed, item.Messages...)
			} else {
				params.MessagesCache.Add(ctx.Message.ChannelID, &cache.GPTMessagesCacheData{
					Messages: transformed,
				})
			}

			// If there are no more messages in the thread, break the loop
			if len(batch) == 0 {
				break
			}

			// Set the lastID to the last message's ID to get the next batch of messages
			lastID = batch[len(batch)-1].ID
		}

		if retries >= DiscordChannelMessagesRequestMaxRetries {
			// max retries reached on fetching messages
			// remove cache to make sure we retry again next time
			log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel messages. Reached max retries\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			params.MessagesCache.Remove(ctx.Message.ChannelID)
			return false
		}

		if !isGPTThread {
			// this was not a GPT thread, clear cache in case and move on
			params.MessagesCache.Remove(ctx.Message.ChannelID)
			log.Printf("[GID: %s, CHID: %s, MID: %s] Not a GPT thread, saving to ignored cache to skip over it later\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID)
			// save threadID to cache, so we can always ignore it later
			(*params.IgnoredChannelsCache)[ctx.Message.ChannelID] = struct{}{}
			return false
		}
	}

	// Lock the thread while we are generating ChatGPT answser
	utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, true)
	// Unlock the thread at the end
	defer utils.ToggleDiscordThreadLock(ctx.Session, ctx.Message.ChannelID, false)

	channelMessage, err := ctx.Session.ChannelMessageSendReply(ctx.Message.ChannelID, constants.GenericPendingMessage, ctx.Message.Reference())
	if err != nil {
		// Without reply  we cannot edit message with the response of ChatGPT
		// Maybe in the future just try to post a new message instead, but for now just cancel
		log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to reply in the thread with the error: %v\n", ctx.Message.GuildID, ctx.Message.ChannelID, ctx.Message.ID, err)
		return false
	}

	// TODO: make ChatGPT request
	print(channelMessage)

	return true
}

func parseInteractionReply(discordMessage *discord.Message) (prompt string, context string, model string) {
	if discordMessage.Embeds != nil && len(discordMessage.Embeds) > 0 {
		for _, value := range discordMessage.Embeds[0].Fields {
			switch value.Name {
			case ChatGPTCommandOptionPrompt.HumanReadableString():
				prompt = value.Value
			case ChatGPTCommandOptionContext.HumanReadableString():
				context = value.Value
			case ChatGPTCommandOptionModel.HumanReadableString():
				model = value.Value
			}
		}

		return
	}

	// old format for backwards compatibility with threads from v0.0.x
	// remove at some point later
	if strings.Contains(discordMessage.Content, "\n") {
		lines := strings.Split(discordMessage.Content, "\n")
		prompt = strings.TrimPrefix(lines[1], "> ")
		if len(lines) > 2 {
			context = strings.TrimPrefix(lines[3], "> ")
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

func ChatGPTCommand(params *ChatGPTCommandParams) (c Command) {
	return Command{
		Name:                     "chatgpt",
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
		},
		Handler: HandlerFunc(func(ctx *Context) {
			handler(ctx, params)
		}),
		MessageHandler: MessageHandlerFunc(func(ctx *MessageContext) bool {
			return messageHandler(ctx, params)
		}),
	}
}
