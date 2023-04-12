package commands

import (
	discord "github.com/bwmarrin/discordgo"
)

type OptionsMap = map[string]*discord.ApplicationCommandInteractionDataOption

type Context struct {
	*discord.Session
	Caller      *Command
	Interaction *discord.Interaction
	Options     OptionsMap

	Handlers []Handler
}

type MessageContext struct {
	*discord.Session
	Caller  *Command
	Message *discord.Message

	Handlers []MessageHandler
}

func (ctx *Context) Respond(response *discord.InteractionResponse) error {
	return ctx.Session.InteractionRespond(ctx.Interaction, response)
}

func (ctx *Context) Edit(content string) error {
	_, err := ctx.Session.InteractionResponseEdit(ctx.Interaction, &discord.WebhookEdit{
		Content: &content,
	})
	return err
}
