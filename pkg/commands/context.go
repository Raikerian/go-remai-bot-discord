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
