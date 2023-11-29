package dalle

import (
	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/bot"
	"github.com/sashabaranov/go-openai"
)

const commandName = "dalle"

func Command(client *openai.Client) *bot.Command {
	// numberOptionMinValue := 1.0
	return &bot.Command{
		Name:        commandName,
		Description: "Generate creative images from textual descriptions using OpenAI Dalle 2",
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        imageCommandOptionPrompt.String(),
				Description: "A text description of the desired image",
				Required:    true,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        imageCommandOptionModel.String(),
				Description: "Dall-e model",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  openai.CreateImageModelDallE3 + " (Default)",
						Value: openai.CreateImageModelDallE3,
					},
					{
						Name:  openai.CreateImageModelDallE2,
						Value: openai.CreateImageModelDallE2,
					},
				},
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        imageCommandOptionSize.String(),
				Description: "The size of the generated images",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					// Dall-e 2-only sizes
					{
						Name:  openai.CreateImageSize256x256 + " (v2 only)",
						Value: openai.CreateImageSize256x256,
					},
					{
						Name:  openai.CreateImageSize512x512 + " (v2 only)",
						Value: openai.CreateImageSize512x512,
					},
					// Supported by both
					{
						Name:  openai.CreateImageSize1024x1024 + " (Default)",
						Value: openai.CreateImageSize1024x1024,
					},
					// Dall-e 3-only sizes
					{
						Name:  openai.CreateImageSize1792x1024 + " (v3 only)",
						Value: openai.CreateImageSize1792x1024,
					},
					{
						Name:  openai.CreateImageSize1024x1792 + " (v3 only)",
						Value: openai.CreateImageSize1024x1792,
					},
				},
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        imageCommandOptionStyle.String(),
				Description: "The style of the generated images (v3 only)",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  openai.CreateImageStyleVivid + " (Default)",
						Value: openai.CreateImageStyleVivid,
					},
					{
						Name:  openai.CreateImageStyleNatural,
						Value: openai.CreateImageStyleNatural,
					},
				},
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        imageCommandOptionQuality.String(),
				Description: "The quality of the generated images (v3 only)",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  openai.CreateImageQualityStandard + " (Default)",
						Value: openai.CreateImageQualityStandard,
					},
					{
						Name:  openai.CreateImageQualityHD,
						Value: openai.CreateImageQualityHD,
					},
				},
			},
			// Temp hiding this as Dall-e 3 currently doesn't support more than 1 image in request
			// {
			// 	Type:        discord.ApplicationCommandOptionInteger,
			// 	Name:        imageCommandOptionNumber.String(),
			// 	Description: "The number of images to generate (default 1, max 4)",
			// 	MinValue:    &numberOptionMinValue,
			// 	MaxValue:    4,
			// 	Required:    false,
			// },
		},
		Handler: bot.HandlerFunc(func(ctx *bot.Context) {
			imageHandler(ctx, client)
		}),
		Middlewares: []bot.Handler{
			bot.HandlerFunc(imageInteractionResponseMiddleware),
			bot.HandlerFunc(func(ctx *bot.Context) {
				imageModerationMiddleware(ctx, client)
			}),
		},
	}
}
