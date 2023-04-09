package bot

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	discord "github.com/bwmarrin/discordgo"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

type Bot struct {
	session         *discord.Session
	openaiClient    *openai.Client
	commandHandlers map[string]func(s *discord.Session, i *discord.InteractionCreate)
	messagesCache   *lru.Cache[string, *cache.ChatGPTMessagesCache]
}

var ignoredChannelsCache = make(map[string]struct{})

func NewBot(session *discord.Session, openaiClient *openai.Client) *Bot {
	cache, err := lru.New[string, *cache.ChatGPTMessagesCache](constants.DiscordThreadsCacheSize)
	if err != nil {
		panic(err)
	}
	return &Bot{
		session:         session,
		openaiClient:    openaiClient,
		commandHandlers: make(map[string]func(s *discord.Session, i *discord.InteractionCreate)),
		messagesCache:   cache,
	}
}

func (b *Bot) RegisterCommandHandler(name string, handler func(s *discord.Session, i *discord.InteractionCreate)) {
	b.commandHandlers[name] = handler
}

func (b *Bot) MessagesCache() *lru.Cache[string, *cache.ChatGPTMessagesCache] {
	return b.messagesCache
}

func (b *Bot) Run(commands []*discord.ApplicationCommand, guildID string, removeCommands bool) {
	// basic info
	b.session.AddHandler(func(s *discord.Session, r *discord.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	// Register handlers
	b.session.AddHandler(func(s *discord.Session, i *discord.InteractionCreate) {
		if h, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	b.session.AddHandler(b.handleMessageCreate)

	// IntentMessageContent is required for us to have a conversation in threads without typing any commands
	b.session.Identify.Intents = discord.MakeIntent(discord.IntentsAllWithoutPrivileged | discord.IntentMessageContent)

	// Run the bot
	err := b.session.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}

	// Register commands
	log.Println("Adding commands...")
	registeredCommands := make([]*discord.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, guildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	defer b.session.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// Unregister commands if requested
	if removeCommands {
		b.removeCommands(registeredCommands, guildID)
	}

	log.Println("Gracefully shutting down.")
}

func (b *Bot) removeCommands(registeredCommands []*discord.ApplicationCommand, guildID string) {
	log.Println("Removing commands...")
	// // We need to fetch the commands, since deleting requires the command ID.
	// // We are doing this from the returned commands on line 375, because using
	// // this will delete all the commands, which might not be desirable, so we
	// // are deleting only the commands that we added.
	// registeredCommands, err := s.ApplicationCommands(s.State.User.ID, *GuildID)
	// if err != nil {
	// 	log.Fatalf("Could not fetch registered commands: %v", err)
	// }

	for _, v := range registeredCommands {
		err := b.session.ApplicationCommandDelete(b.session.State.User.ID, guildID, v.ID)
		if err != nil {
			log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}
}

// TODO: refactor this out of this file
func (b *Bot) handleMessageCreate(s *discord.Session, m *discord.MessageCreate) {
	if s.State.User.ID == m.Author.ID {
		// ignore self messages
		return
	}

	if _, exists := ignoredChannelsCache[m.ChannelID]; exists {
		// skip over ignored channels list
		return
	}

	if m.Content == "" {
		// ignore messages with empty content
		return
	}

	if ch, err := s.State.Channel(m.ChannelID); err != nil || ch.IsThread() {
		if err != nil {
			// We need to be sure that it's a thread, and since we failed to fetch channel
			// we just log the error and move on
			log.Printf("[CHID: %s, MID: %s] Failed to get channel info with the error: %v\n", m.ChannelID, m.ID, err)
			return
		}

		if ch.ThreadMetadata.Locked || ch.ThreadMetadata.Archived {
			// We don't want to handle messages in locked or archived threads
			log.Printf("[CHID: %s] Ignoring new message [MID: %s] in a potential thread as it is locked or/and archived\n", m.ChannelID, m.ID)
			return
		}

		log.Printf("[CHID: %s] Handling new message [MID: %s] in a thread\n", m.ChannelID, m.ID)

		if !b.messagesCache.Contains(m.ChannelID) {
			isGPTThread := true

			var lastID string
			for {
				// Get messages in batches of 100 (maximum allowed by Discord API)
				batch, _ := s.ChannelMessages(ch.ID, 100, lastID, "", "")
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
					if value.ID == m.ID {
						// avoid adding current message
						continue
					}
					role := openai.ChatMessageRoleUser
					if value.Author.ID == s.State.User.ID {
						role = openai.ChatMessageRoleAssistant
					}
					content := value.Content
					// First message is always a referenced message
					// Check if it is, and then modify to get the original prompt
					if value.Type == discord.MessageTypeThreadStarterMessage {
						if value.Author.ID != s.State.User.ID {
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
								if item, ok := b.messagesCache.Get(m.ChannelID); ok {
									item.SystemMessage = systemMessage
								} else {
									b.messagesCache.Add(m.ChannelID, &cache.ChatGPTMessagesCache{
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
				if item, ok := b.messagesCache.Get(m.ChannelID); ok {
					item.Messages = append(transformed, item.Messages...)
				} else {
					b.messagesCache.Add(m.ChannelID, &cache.ChatGPTMessagesCache{
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
				b.messagesCache.Remove(m.ChannelID)
				log.Printf("[CHID: %s] Not a GPT thread, saving to ignored cache to skip over it later", m.ChannelID)
				// save threadID to cache, so we can always ignore it later
				ignoredChannelsCache[m.ChannelID] = struct{}{}
				return
			}
		}

		// Lock the thread while we are generating ChatGPT answser
		utils.ToggleDiscordThreadLock(s, m.ChannelID, true)
		// Unlock the thread at the end
		defer utils.ToggleDiscordThreadLock(s, m.ChannelID, false)

		channelMessage, err := s.ChannelMessageSendReply(m.ChannelID, constants.GenericPendingMessage, m.Reference())
		if err != nil {
			// Without reply  we cannot edit message with the response of ChatGPT
			// Maybe in the future just try to post a new message instead, but for now just cancel
			log.Printf("[CHID: %s] Failed to reply in the thread with the error: %v", m.ChannelID, err)
			return
		}

		handlers.ChatGPTRequest(handlers.ChatGPTHandlerParams{
			OpenAIClient:     b.openaiClient,
			GPTModel:         getModelFromTitle(ch.Name),
			GPTPrompt:        m.Content,
			DiscordSession:   s,
			DiscordChannelID: m.ChannelID,
			DiscordMessageID: channelMessage.ID,
			MessagesCache:    b.messagesCache,
		})
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

func reverseMessages(messages *[]openai.ChatCompletionMessage) {
	length := len(*messages)
	for i := 0; i < length/2; i++ {
		(*messages)[i], (*messages)[length-i-1] = (*messages)[length-i-1], (*messages)[i]
	}
}
