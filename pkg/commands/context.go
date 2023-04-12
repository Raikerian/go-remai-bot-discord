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

func (ctx *Context) Response() (*discord.Message, error) {
	return ctx.Session.InteractionResponse(ctx.Interaction)
}

func (ctx *Context) EditMessage(messageID string, channelID string, content string) error {
	_, err := ctx.Session.ChannelMessageEditComplex(
		&discord.MessageEdit{
			Content: &content,
			ID:      messageID,
			Channel: channelID,
		},
	)
	return err
}

func (ctx *MessageContext) Reply(content string) (m *discord.Message, err error) {
	m, err = ctx.Session.ChannelMessageSendReply(
		ctx.Message.ChannelID,
		content,
		ctx.Message.Reference(),
	)
	return
}

func (ctx *MessageContext) Edit(messageID string, channelID string, content string) error {
	_, err := ctx.Session.ChannelMessageEditComplex(
		&discord.MessageEdit{
			Content: &content,
			ID:      messageID,
			Channel: channelID,
		},
	)
	return err
}
