package dalle

import "fmt"

type imageCommandOptionType uint8

const (
	imageCommandOptionPrompt  imageCommandOptionType = 1
	imageCommandOptionSize    imageCommandOptionType = 2
	imageCommandOptionNumber  imageCommandOptionType = 3
	imageCommandOptionModel   imageCommandOptionType = 4
	imageCommandOptionQuality imageCommandOptionType = 5
	imageCommandOptionStyle   imageCommandOptionType = 6
)

func (t imageCommandOptionType) String() string {
	switch t {
	case imageCommandOptionPrompt:
		return "prompt"
	case imageCommandOptionSize:
		return "size"
	case imageCommandOptionNumber:
		return "number"
	case imageCommandOptionModel:
		return "model"
	case imageCommandOptionQuality:
		return "quality"
	case imageCommandOptionStyle:
		return "style"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}
