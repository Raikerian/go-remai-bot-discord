package dalle

import (
	"fmt"

	discord "github.com/bwmarrin/discordgo"
	"github.com/raikerian/go-remai-bot-discord/pkg/constants"
	"github.com/sashabaranov/go-openai"
)

const (
	imageDefaultSize = openai.CreateImageSize1024x1024

	dalle2PriceSize256x256   = 0.016
	dalle2PriceSize512x512   = 0.018
	dalle2PriceSize1024x1024 = 0.02

	dalle3PriceSize1024x1024        = 0.04
	dalle3PriceSize1024x1792x1024   = 0.08
	dalle3PriceSize1024x1024HD      = 0.08
	dalle3PriceSize1024x1792x1024HD = 0.12
)

func priceForResponse(n int, size, model, quality string) float64 {
	switch size {
	case openai.CreateImageSize256x256:
		return float64(n) * dalle2PriceSize256x256
	case openai.CreateImageSize512x512:
		return float64(n) * dalle2PriceSize512x512
	case openai.CreateImageSize1024x1024:
		if model == openai.CreateImageModelDallE3 {
			switch quality {
			case openai.CreateImageQualityHD:
				return float64(n) * dalle3PriceSize1024x1024HD
			case openai.CreateImageQualityStandard:
				return float64(n) * dalle3PriceSize1024x1024
			}
			return 0
		}
		return float64(n) * dalle2PriceSize1024x1024
	case openai.CreateImageSize1792x1024, openai.CreateImageSize1024x1792:
		if quality == openai.CreateImageQualityHD {
			return float64(n) * dalle3PriceSize1024x1792x1024HD
		}
		return float64(n) * dalle3PriceSize1024x1792x1024
	}

	return 0
}

func imageCreationUsageEmbedFooter(model string, size string, number int, quality string) *discord.MessageEmbedFooter {
	extraInfo := fmt.Sprintf("Size: %s", size)
	if model == openai.CreateImageModelDallE2 && number > 1 {
		extraInfo += fmt.Sprintf(", Images: %d", number)
	}
	price := priceForResponse(number, size, model, quality)
	if price > 0 {
		extraInfo += fmt.Sprintf("\nGeneration Cost: $%g", price)
	}
	return &discord.MessageEmbedFooter{
		Text:    extraInfo,
		IconURL: constants.OpenAIBlackIconURL,
	}
}

func imageSizeToWidthHeight(size string) (int, int) {
	switch size {
	case openai.CreateImageSize256x256:
		return 256, 256
	case openai.CreateImageSize512x512:
		return 512, 512
	case openai.CreateImageSize1024x1024:
		return 1024, 1024
	case openai.CreateImageSize1792x1024:
		return 1792, 1024
	case openai.CreateImageSize1024x1792:
		return 1024, 1792
	}
	return 0, 0
}
