package commands

import discord "github.com/bwmarrin/discordgo"

type Handler interface {
	HandleCommand(ctx *Context)
}

type HandlerFunc func(ctx *Context)

func (f HandlerFunc) HandleCommand(ctx *Context) { f(ctx) }

type MessageHandler interface {
	HandleMessageCommand(ctx *MessageContext) bool
}

type MessageHandlerFunc func(ctx *MessageContext) bool

func (f MessageHandlerFunc) HandleMessageCommand(ctx *MessageContext) bool { return f(ctx) }

type Command struct {
	Name                     string
	Description              string
	DMPermission             bool
	DefaultMemberPermissions int64
	Options                  []*discord.ApplicationCommandOption

	Handler     Handler
	Middlewares []Handler

	MessageHandler     MessageHandler
	MessageMiddlewares []MessageHandler
}

func (cmd Command) ApplicationCommand() *discord.ApplicationCommand {
	applicationCommand := &discord.ApplicationCommand{
		Name:                     cmd.Name,
		Description:              cmd.Description,
		DMPermission:             &cmd.DMPermission,
		DefaultMemberPermissions: &cmd.DefaultMemberPermissions,
		Options:                  cmd.Options,
	}
	return applicationCommand
}