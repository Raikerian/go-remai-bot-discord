package commands

import (
	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
)

const (
	infoCommandName = "info"
)

func infoHandler(ctx *Context) {
	ctx.Respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseChannelMessageWithSource,
		Data: &discord.InteractionResponseData{
			// Note: only visible to the user who invoked the command
			Flags: discord.MessageFlagsEphemeral,
			// Content: "Surprise!",
			Components: []discord.MessageComponent{
				discord.ActionsRow{
					Components: []discord.MessageComponent{
						&discord.Button{
							Label: "Source code",
							Style: discord.LinkButton,
							URL:   "https://github.com/Raikerian/go-remai-bot-discord",
						},
					},
				},
			},
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "Bot Version",
					Description: "Version: " + constants.Version,
					Color:       0x00bfff,
				},
			},
		},
	})
}

func InfoCommand() Command {
	return Command{
		Name:                     infoCommandName,
		Description:              "Show information about current version of Rem AI",
		DMPermission:             true,
		DefaultMemberPermissions: discord.PermissionViewChannel,
		Handler:                  HandlerFunc(infoHandler),
	}
}
