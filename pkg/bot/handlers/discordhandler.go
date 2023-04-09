package handlers

import (
	"log"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/commandhandlers/commandoptions"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

type DiscordMessageCreateParams struct {
	DiscordSession       *discord.Session
	DiscordMessage       *discord.MessageCreate
	OpenAIClient         *openai.Client
	GPTMessagesCache     *cache.GPTMessagesCache
	IgnoredChannelsCache *map[string]struct{}
}

// TODO: refactor this, otherwise its going to be hard to handle different commands
func OnDiscordMessageCreate(params DiscordMessageCreateParams) {
	if params.DiscordSession.State.User.ID == params.DiscordMessage.Author.ID {
		// ignore self messages
		return
	}

	if _, exists := (*params.IgnoredChannelsCache)[params.DiscordMessage.ChannelID]; exists {
		// skip over ignored channels list
		return
	}

	if params.DiscordMessage.Content == "" {
		// ignore messages with empty content
		return
	}

	if ch, err := params.DiscordSession.State.Channel(params.DiscordMessage.ChannelID); err != nil || ch.IsThread() {
		if err != nil {
			// We need to be sure that it's a thread, and since we failed to fetch channel
			// we just log the error and move on
			log.Printf("[GID: %s, CHID: %s, MID: %s] Failed to get channel info with the error: %v\n", params.DiscordMessage.GuildID, params.DiscordMessage.ChannelID, params.DiscordMessage.ID, err)
			return
		}

		if ch.ThreadMetadata.Locked || ch.ThreadMetadata.Archived {
			// We don't want to handle messages in locked or archived threads
			log.Printf("[GID: %s, CHID: %s, MID: %s] Ignoring new message in a potential thread as it is locked or/and archived\n", params.DiscordMessage.GuildID, params.DiscordMessage.ChannelID, params.DiscordMessage.ID)
			return
		}

		log.Printf("[GID: %s, CHID: %s, MID: %s] Handling new message in a thread\n", params.DiscordMessage.GuildID, params.DiscordMessage.ChannelID, params.DiscordMessage.ID)

		if !params.GPTMessagesCache.Contains(params.DiscordMessage.ChannelID) {
			isGPTThread := true

			var lastID string
			for {
				// Get messages in batches of 100 (maximum allowed by Discord API)
				batch, _ := params.DiscordSession.ChannelMessages(ch.ID, 100, lastID, "", "")
				if err != nil {
					// Since we cannot fetch messages, that means we cannot determine whether this a GPT thread,
					// and if it was, we cannot get the full context to provide a better user experience. Silently return
					// and print the error in the log
					// TODO: in the unfortunate event of discord API failing, we will cache this thread as non GPT thread and
					// will ignore it until bot is restarted. In this particular event I believe its fair to not cache it to ignored list
					isGPTThread = false
					break
				}

				transformed := make([]openai.ChatCompletionMessage, 0, len(batch))
				for _, value := range batch {
					if value.ID == params.DiscordMessage.ID {
						// avoid adding current message
						continue
					}
					role := openai.ChatMessageRoleUser
					if value.Author.ID == params.DiscordSession.State.User.ID {
						role = openai.ChatMessageRoleAssistant
					}
					content := value.Content
					// First message is always a referenced message
					// Check if it is, and then modify to get the original prompt
					if value.Type == discord.MessageTypeThreadStarterMessage {
						if value.Author.ID != params.DiscordSession.State.User.ID || value.ReferencedMessage == nil {
							// this is not gpt thread, ignore
							// since we are wasting here a total request to discord API, need to refactor so we always fetch messages from the oldest first
							// TODO: fetch oldest first from discord api
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

						if item, ok := params.GPTMessagesCache.Get(params.DiscordMessage.ChannelID); ok {
							item.SystemMessage = systemMessage
							item.GPTModel = model
						} else {
							params.GPTMessagesCache.Add(params.DiscordMessage.ChannelID, &cache.GPTMessagesCacheData{
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
				if item, ok := params.GPTMessagesCache.Get(params.DiscordMessage.ChannelID); ok {
					item.Messages = append(transformed, item.Messages...)
				} else {
					params.GPTMessagesCache.Add(params.DiscordMessage.ChannelID, &cache.GPTMessagesCacheData{
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

			if !isGPTThread {
				// this was not a GPT thread, clear cache in case and move on
				// TODO: remove cache clear when above request is fixed to have oldest first, as we wont have any cache that way
				params.GPTMessagesCache.Remove(params.DiscordMessage.ChannelID)
				log.Printf("[GID: %s, CHID: %s] Not a GPT thread, saving to ignored cache to skip over it later", params.DiscordMessage.GuildID, params.DiscordMessage.ChannelID)
				// save threadID to cache, so we can always ignore it later
				(*params.IgnoredChannelsCache)[params.DiscordMessage.ChannelID] = struct{}{}
				return
			}
		}

		// Lock the thread while we are generating ChatGPT answser
		utils.ToggleDiscordThreadLock(params.DiscordSession, params.DiscordMessage.ChannelID, true)
		// Unlock the thread at the end
		defer utils.ToggleDiscordThreadLock(params.DiscordSession, params.DiscordMessage.ChannelID, false)

		channelMessage, err := params.DiscordSession.ChannelMessageSendReply(params.DiscordMessage.ChannelID, constants.GenericPendingMessage, params.DiscordMessage.Reference())
		if err != nil {
			// Without reply  we cannot edit message with the response of ChatGPT
			// Maybe in the future just try to post a new message instead, but for now just cancel
			log.Printf("GID: %s, [CHID: %s] Failed to reply in the thread with the error: %v", params.DiscordMessage.GuildID, params.DiscordMessage.ChannelID, err)
			return
		}

		OnChatGPTRequest(ChatGPTRequestParams{
			OpenAIClient:     params.OpenAIClient,
			GPTPrompt:        params.DiscordMessage.Content,
			DiscordSession:   params.DiscordSession,
			DiscordGuildID:   params.DiscordMessage.GuildID,
			DiscordChannelID: params.DiscordMessage.ChannelID,
			DiscordMessageID: channelMessage.ID,
			GPTMessagesCache: params.GPTMessagesCache,
		})
	}
}

func reverseMessages(messages *[]openai.ChatCompletionMessage) {
	length := len(*messages)
	for i := 0; i < length/2; i++ {
		(*messages)[i], (*messages)[length-i-1] = (*messages)[length-i-1], (*messages)[i]
	}
}

func parseInteractionReply(discordMessage *discord.Message) (prompt string, context string, model string) {
	if discordMessage.Embeds != nil && len(discordMessage.Embeds) > 0 {
		for _, value := range discordMessage.Embeds[0].Fields {
			switch value.Name {
			case commandoptions.ChatGPTCommandOptionPrompt.HumanReadableString():
				prompt = value.Value
			case commandoptions.ChatGPTCommandOptionContext.HumanReadableString():
				context = value.Value
			case commandoptions.ChatGPTCommandOptionModel.HumanReadableString():
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
