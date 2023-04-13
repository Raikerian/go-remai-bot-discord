package bot

import (
	"fmt"
	"log"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/commands"
)

type Router struct {
	commands           map[string]*commands.Command
	registeredCommands []*discord.ApplicationCommand
}

func NewRouter() *Router {
	return &Router{
		commands: make(map[string]*commands.Command),
	}
}

func (r *Router) Register(cmd *commands.Command) {
	if _, ok := r.commands[cmd.Name]; !ok {
		r.commands[cmd.Name] = cmd
	}
}

func (r *Router) HandleInteraction(s *discord.Session, i *discord.InteractionCreate) {
	if i.Type != discord.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	cmd := r.commands[data.Name]
	if cmd == nil {
		return
	}

	ctx := commands.NewContext(s, cmd, i.Interaction, append(cmd.Middlewares, cmd.Handler))
	ctx.Next()
}

func (r *Router) HandleMessage(s *discord.Session, m *discord.MessageCreate) {
	for _, cmd := range r.commands {
		if cmd.MessageHandler != nil {
			ctx := commands.NewMessageContext(s, cmd, m.Message, cmd.MessageMiddlewares)

			hit := cmd.MessageHandler.HandleMessageCommand(ctx)
			if !hit {
				continue
			}

			ctx.Next()
		}
	}
}

func (r *Router) Sync(s *discord.Session, guild string) (err error) {
	if s.State.User == nil {
		return fmt.Errorf("cannot determine application id")
	}

	var commands []*discord.ApplicationCommand
	for _, c := range r.commands {
		commands = append(commands, c.ApplicationCommand())
	}

	r.registeredCommands, err = s.ApplicationCommandBulkOverwrite(s.State.User.ID, guild, commands)
	return
}

func (r *Router) ClearCommands(s *discord.Session, guild string) (errors []error) {
	if s.State.User == nil {
		return []error{fmt.Errorf("cannot determine application id")}
	}

	for _, v := range r.registeredCommands {
		err := s.ApplicationCommandDelete(s.State.User.ID, guild, v.ID)
		if err != nil {
			log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return errors
}
