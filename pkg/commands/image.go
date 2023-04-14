package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/utils"
	"github.com/sashabaranov/go-openai"
)

const ImageHTTPRequestTimeout = 120 * time.Second

const (
	imageCommandName = "image"

	imageDefaultSize       = openai.CreateImageSize256x256
	imageMaxFilenameLength = 250

	imagePriceSize256x256   = 0.016
	imagePriceSize512x512   = 0.018
	imagePriceSize1024x1024 = 0.02
)

type ImageCommandParams struct {
	OpenAIClient          *openai.Client
	ImageUploadHTTPClient *http.Client
}

type ImageCommandOptionType uint8

const (
	ImageCommandOptionPrompt ImageCommandOptionType = 1
	ImageCommandOptionSize   ImageCommandOptionType = 2
	ImageCommandOptionNumber ImageCommandOptionType = 3
)

func (t ImageCommandOptionType) String() string {
	switch t {
	case ImageCommandOptionPrompt:
		return "prompt"
	case ImageCommandOptionSize:
		return "size"
	case ImageCommandOptionNumber:
		return "number"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func imageInteractionResponseMiddleware(ctx *Context) {
	log.Printf("[GID: %s, i.ID: %s] Image interaction invoked by UserID: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, ctx.Interaction.Member.User.ID)

	err := ctx.Respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Failed to respond to interactrion with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		return
	}

	ctx.Next()
}

func imageModerationMiddleware(ctx *Context, params *ImageCommandParams) {
	log.Printf("[GID: %s, i.ID: %s] Performing interaction moderation middleware\n", ctx.Interaction.GuildID, ctx.Interaction.ID)

	var prompt string
	if option, ok := ctx.Options[ImageCommandOptionPrompt.String()]; ok {
		prompt = option.StringValue()
	} else {
		// We can't have empty prompt, unfortunately
		// this should not happen, discord prevents empty required options
		log.Printf("[GID: %s, i.ID: %s] Failed to parse prompt option\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		ctx.Respond(&discord.InteractionResponse{
			Type: discord.InteractionResponseChannelMessageWithSource,
			Data: &discord.InteractionResponseData{
				Content: "ERROR: Failed to parse prompt option",
			},
		})
		return
	}

	resp, err := params.OpenAIClient.Moderations(
		context.Background(),
		openai.ModerationRequest{
			Input: prompt,
		},
	)
	if err != nil {
		// do not block request if moderation api failed
		log.Printf("[GID: %s, i.ID: %s] OpenAI Moderation API request failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		ctx.Next()
		return
	}

	if resp.Results[0].Flagged {
		// response was flagged, send error
		log.Printf("[GID: %s, i.ID: %s] Interaction was flagged by Moderation API, prompt: \"%s\"\n", ctx.Interaction.GuildID, ctx.Interaction.ID, prompt)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ Error",
					Description: "The provided prompt contains text that violates OpenAI's usage policies and is not allowed by their safety system",
					Color:       0xff0000,
				},
			},
		})
		return
	}

	ctx.Next()
}

func imageHandler(ctx *Context, params *ImageCommandParams) {
	var prompt string
	if option, ok := ctx.Options[ImageCommandOptionPrompt.String()]; ok {
		prompt = option.StringValue()
	} else {
		// We can't have empty prompt, unfortunately
		// this should not happen, discord prevents empty required options
		log.Printf("[GID: %s, i.ID: %s] Failed to parse prompt option\n", ctx.Interaction.GuildID, ctx.Interaction.ID)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ Error",
					Description: " Failed to parse prompt option",
					Color:       0xff0000,
				},
			},
		})
		return
	}

	size := imageDefaultSize
	if option, ok := ctx.Options[ImageCommandOptionSize.String()]; ok {
		size = option.StringValue()
		log.Printf("[GID: %s, i.ID: %s] Image size provided: %s\n", ctx.Interaction.GuildID, ctx.Interaction.ID, size)
	}

	number := 1
	if option, ok := ctx.Options[ImageCommandOptionNumber.String()]; ok {
		number = int(option.IntValue())
		log.Printf("[GID: %s, i.ID: %s] Image number provided: %d\n", ctx.Interaction.GuildID, ctx.Interaction.ID, number)
	}

	log.Printf("[GID: %s, CHID: %s] Dalle Request [Size: %s, Number: %d] invoked", ctx.Interaction.GuildID, ctx.Interaction.ID, size, number)
	resp, err := params.OpenAIClient.CreateImage(
		context.Background(),
		openai.ImageRequest{
			Prompt:         prompt,
			N:              number,
			Size:           size,
			ResponseFormat: openai.CreateImageResponseFormatB64JSON,
		},
	)
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] OpenAI request CreateImage failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ OpenAI API failed",
					Description: err.Error(),
					Color:       0xff0000,
				},
			},
		})
		return
	}

	log.Printf("[GID: %s, i.ID: %s] Dalle Request [Size: %s, Number: %d] responded with a data array size %d\n", ctx.Interaction.GuildID, ctx.Interaction.ID, size, number, len(resp.Data))

	var files []*discord.File
	var errors []error
	for i, data := range resp.Data {
		imgBytes, err := base64.StdEncoding.DecodeString(data.B64JSON)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		// imagesBytes = append(imagesBytes, imgBytes)
		files = append(files, &discord.File{
			Name:   textToFilename(prompt, (i + 1)),
			Reader: bytes.NewReader(imgBytes),
		})
	}
	// only fail if all of the images failed to decode
	if len(files) == 0 {
		log.Printf("[GID: %s, i.ID: %s] Failed to decode Base64 image data with the errors: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, errors)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ Failed to decode Base64 image data",
					Description: fmt.Sprintf("Errors: %v", errors),
					Color:       0xff0000,
				},
			},
		})
	}

	m, err := ctx.ChannelMessageSendComplex(
		ctx.Interaction.ChannelID,
		&discord.MessageSend{
			Content: fmt.Sprintf("<@%s>\n> %s", ctx.Interaction.Member.User.ID, prompt),
			Files:   files,
		},
		discord.WithClient(params.ImageUploadHTTPClient),
	)
	if err != nil {
		log.Printf("[GID: %s, i.ID: %s] Discord API failed with the error: %v\n", ctx.Interaction.GuildID, ctx.Interaction.ID, err)
		ctx.FollowupMessageCreate(ctx.Interaction, true, &discord.WebhookParams{
			Content: fmt.Sprintf("> %s", prompt),
			Embeds: []*discord.MessageEmbed{
				{
					Title:       "❌ Discord API Error",
					Description: err.Error(),
					Color:       0xff0000,
				},
			},
		})
		return
	}

	ctx.InteractionResponseDelete(ctx.Interaction)
	attachCreateImageUsageInfo(ctx.Session, m, size, len(resp.Data))
}

func textToFilename(text string, n int) string {
	// Truncate the text
	if len(text) > imageMaxFilenameLength {
		text = text[:imageMaxFilenameLength]
	}

	// Replace spaces with underscores
	filename := strings.ReplaceAll(text, " ", "_")
	filename += fmt.Sprintf("_%d", n)

	// Ensure the filename ends with ".png"
	if !strings.HasSuffix(filename, ".png") {
		filename += ".png"
	}

	return filename
}

func priceForResponse(n int, size string) float64 {
	switch size {
	case openai.CreateImageSize256x256:
		return float64(n) * imagePriceSize256x256
	case openai.CreateImageSize512x512:
		return float64(n) * imagePriceSize512x512
	case openai.CreateImageSize1024x1024:
		return float64(n) * imagePriceSize1024x1024
	}

	return 0
}

func attachCreateImageUsageInfo(s *discord.Session, m *discord.Message, size string, number int) {
	extraInfo := fmt.Sprintf("Size: %s, Images: %d", size, number)
	price := priceForResponse(number, size)
	if price > 0 {
		extraInfo += fmt.Sprintf("\nLLM Cost: $%g", price)
	}
	utils.DiscordChannelMessageEdit(s, m.ID, m.ChannelID, nil, []*discord.MessageEmbed{
		{
			Fields: []*discord.MessageEmbedField{
				{
					Name:  "Usage",
					Value: extraInfo,
				},
			},
		},
	})
}

func ImageCommand(params *ImageCommandParams) *Command {
	numberOptionMinValue := 1.0
	return &Command{
		Name:                     imageCommandName,
		Description:              "Generate creative images from textual descriptions",
		DMPermission:             false,
		DefaultMemberPermissions: discord.PermissionViewChannel,
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        ImageCommandOptionPrompt.String(),
				Description: "A text description of the desired image",
				Required:    true,
			},
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        ImageCommandOptionSize.String(),
				Description: "The size of the generated images",
				Required:    false,
				Choices: []*discord.ApplicationCommandOptionChoice{
					{
						Name:  openai.CreateImageSize256x256 + " (Default)",
						Value: openai.CreateImageSize256x256,
					},
					{
						Name:  openai.CreateImageSize512x512,
						Value: openai.CreateImageSize512x512,
					},
					{
						Name:  openai.CreateImageSize1024x1024,
						Value: openai.CreateImageSize1024x1024,
					},
				},
			},
			{
				Type:        discord.ApplicationCommandOptionInteger,
				Name:        ImageCommandOptionNumber.String(),
				Description: "The number of images to generate (default 1, max 4)",
				MinValue:    &numberOptionMinValue,
				MaxValue:    4,
				Required:    false,
			},
		},
		Handler: HandlerFunc(func(ctx *Context) {
			imageHandler(ctx, params)
		}),
		Middlewares: []Handler{
			HandlerFunc(imageInteractionResponseMiddleware),
			HandlerFunc(func(ctx *Context) {
				imageModerationMiddleware(ctx, params)
			}),
		},
	}
}
