package bot

import (
	"log"
	"os"
	"os/signal"
	"strings"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	openai "github.com/sashabaranov/go-openai"
)

type Bot struct {
	session         *discord.Session
	openaiClient    *openai.Client
	commandHandlers map[string]func(s *discord.Session, i *discord.InteractionCreate)
	messagesCache   map[string][]openai.ChatCompletionMessage
}

func NewBot(session *discord.Session, openaiClient *openai.Client) *Bot {
	return &Bot{
		session:         session,
		openaiClient:    openaiClient,
		commandHandlers: make(map[string]func(s *discord.Session, i *discord.InteractionCreate)),
		messagesCache:   make(map[string][]openai.ChatCompletionMessage),
	}
}

func (b *Bot) RegisterCommandHandler(name string, handler func(s *discord.Session, i *discord.InteractionCreate)) {
	b.commandHandlers[name] = handler
}

func (b *Bot) MessagesCache() *map[string][]openai.ChatCompletionMessage {
	return &b.messagesCache
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

	// Run the bot
	err := b.session.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}

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
	signal.Notify(stop, os.Interrupt)
	log.Println("Press Ctrl+C to exit")
	<-stop

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

func (b *Bot) handleMessageCreate(s *discord.Session, m *discord.MessageCreate) {
	if s.State.User.ID == m.Author.ID {
		// ignore self messages
		return
	}
	if m.Content == "" {
		// TODO: handle empty content
		log.Fatalf("Message from " + m.Author.Username + " is empty.")
		return
	}
	// log.Println("Message received: " + m.Content + " from " + m.Author.Username)
	if ch, err := s.State.Channel(m.ChannelID); err != nil || ch.IsThread() {
		log.Println("Message received: " + m.Content + " from " + m.Author.Username)

		if b.messagesCache[m.ChannelID] == nil {
			var (
				lastID string
			)

			for {
				// Get messages in batches of 100 (maximum allowed by Discord API)
				batch, _ := s.ChannelMessages(ch.ID, 100, lastID, "", "")
				// if err != nil {
				// 	return nil, err
				// }

				transformed := make([]openai.ChatCompletionMessage, len(batch))
				for i, value := range batch {
					role := openai.ChatMessageRoleUser
					if value.Author.ID == s.State.User.ID {
						role = openai.ChatMessageRoleAssistant
					}
					content := value.Content
					// First message is always a referenced message
					// Check if it is, and then modify to get the original prompt
					if value.ReferencedMessage != nil {
						role = openai.ChatMessageRoleUser
						content = strings.TrimPrefix(strings.Join(strings.Split(value.ReferencedMessage.Content, "\n")[1:], "\n"), "> ")
					}
					transformed[len(batch)-i-1] = openai.ChatCompletionMessage{
						Role:    role,
						Content: content,
					}
				}

				// Add the messages to the beginning of the main list
				b.messagesCache[m.ChannelID] = append(transformed, b.messagesCache[m.ChannelID]...)

				// If there are no more messages in the thread, break the loop
				if len(batch) == 0 {
					break
				}

				// Set the lastID to the last message's ID to get the next batch of messages
				lastID = batch[len(batch)-1].ID
			}
		}

		handlers.HandleChatGPTRequest(b.openaiClient, getModelFromTitle(ch.Name), s, m.ChannelID, m.ID, m.Author.Username, m.Content, m.Reference(), &b.messagesCache)
	}
}

func getModelFromTitle(title string) string {
	if strings.Contains(title, "ChatGPT-3.5") {
		return openai.GPT3Dot5Turbo
	} else if strings.Contains(title, "ChatGPT-4") {
		return openai.GPT4
	}
	return openai.GPT3Dot5Turbo
}
