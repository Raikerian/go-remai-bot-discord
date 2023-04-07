package utils

import (
	discord "github.com/bwmarrin/discordgo"
)

// ToggleThreadLock locks or unlocks a Discord thread, based on the 'locked' parameter.
func ToggleDiscordThreadLock(s *discord.Session, channelID string, locked bool) error {
	_, err := s.ChannelEditComplex(channelID, &discord.ChannelEdit{
		Locked: &locked,
	})
	if err != nil {
		return err
	}
	return nil
}
