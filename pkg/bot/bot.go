package bot

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	discord "github.com/bwmarrin/discordgo"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot/handlers"
	"github.com/raikerian/go-remai-bot-discord/pkg/cache"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
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

	b.session.AddHandler(func(s *discord.Session, m *discord.MessageCreate) {
		handlers.OnDiscordMessageCreate(handlers.DiscordMessageCreateParams{
			DiscordSession:       s,
			DiscordMessage:       m,
			OpenAIClient:         b.openaiClient,
			MessagesCache:        b.messagesCache,
			IgnoredChannelsCache: &ignoredChannelsCache,
		})
	})

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
