package handlers

import (
	"log"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

type DiscordMessageCreateParams struct {
	DiscordSession       *discord.Session
	DiscordMessage       *discord.MessageCreate
	OpenAIClient         *openai.Client
	MessagesCache        *lru.Cache[string, *cache.ChatGPTMessagesCache]
	IgnoredChannelsCache *map[string]struct{}
}

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
			log.Printf("[CHID: %s, MID: %s] Failed to get channel info with the error: %v\n", params.DiscordMessage.ChannelID, params.DiscordMessage.ID, err)
			return
		}

		if ch.ThreadMetadata.Locked || ch.ThreadMetadata.Archived {
			// We don't want to handle messages in locked or archived threads
			log.Printf("[CHID: %s] Ignoring new message [MID: %s] in a potential thread as it is locked or/and archived\n", params.DiscordMessage.ChannelID, params.DiscordMessage.ID)
			return
		}

		log.Printf("[CHID: %s] Handling new message [MID: %s] in a thread\n", params.DiscordMessage.ChannelID, params.DiscordMessage.ID)

		if !params.MessagesCache.Contains(params.DiscordMessage.ChannelID) {
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
						if value.Author.ID != params.DiscordSession.State.User.ID {
							// this is not gpt thread, ignore
							// since we are wasting here a total request to discord API, need to refactor so we always fetch messages from the oldest first
							// TODO: fetch oldest first from discord api
							isGPTThread = false
							break
						}
						role = openai.ChatMessageRoleUser
						if value.ReferencedMessage != nil {
							// TODO: refactor
							lines := strings.Split(value.ReferencedMessage.Content, "\n")
							content = strings.TrimPrefix(lines[1], "> ")
							if len(lines) > 2 {
								context := strings.TrimPrefix(lines[3], "> ")
								systemMessage := &openai.ChatCompletionMessage{
									Role:    openai.ChatMessageRoleSystem,
									Content: context,
								}
								if item, ok := params.MessagesCache.Get(params.DiscordMessage.ChannelID); ok {
									item.SystemMessage = systemMessage
								} else {
									params.MessagesCache.Add(params.DiscordMessage.ChannelID, &cache.ChatGPTMessagesCache{
										SystemMessage: systemMessage,
									})
								}
							}
						}
					}
					transformed = append(transformed, openai.ChatCompletionMessage{
						Role:    role,
						Content: content,
					})
				}

				reverseMessages(&transformed)

				// Add the messages to the beginning of the main list
				if item, ok := params.MessagesCache.Get(params.DiscordMessage.ChannelID); ok {
					item.Messages = append(transformed, item.Messages...)
				} else {
					params.MessagesCache.Add(params.DiscordMessage.ChannelID, &cache.ChatGPTMessagesCache{
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
				params.MessagesCache.Remove(params.DiscordMessage.ChannelID)
				log.Printf("[CHID: %s] Not a GPT thread, saving to ignored cache to skip over it later", params.DiscordMessage.ChannelID)
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
			log.Printf("[CHID: %s] Failed to reply in the thread with the error: %v", params.DiscordMessage.ChannelID, err)
			return
		}

		OnChatGPTRequest(ChatGPTRequestParams{
			OpenAIClient:     params.OpenAIClient,
			GPTModel:         getModelFromTitle(ch.Name),
			GPTPrompt:        params.DiscordMessage.Content,
			DiscordSession:   params.DiscordSession,
			DiscordChannelID: params.DiscordMessage.ChannelID,
			DiscordMessageID: channelMessage.ID,
			MessagesCache:    params.MessagesCache,
		})
	}
}

func reverseMessages(messages *[]openai.ChatCompletionMessage) {
	length := len(*messages)
	for i := 0; i < length/2; i++ {
		(*messages)[i], (*messages)[length-i-1] = (*messages)[length-i-1], (*messages)[i]
	}
}

func getModelFromTitle(title string) string {
	if strings.Contains(title, openai.GPT3Dot5Turbo) {
		return openai.GPT3Dot5Turbo
	} else if strings.Contains(title, openai.GPT4) {
		return openai.GPT4
	}
	return constants.DefaultGPTModel
}
