package gpt

import "fmt"

type gptCommandOptionType uint8

const (
	gptCommandOptionPrompt      gptCommandOptionType = 1
	gptCommandOptionContext     gptCommandOptionType = 2
	gptCommandOptionModel       gptCommandOptionType = 3
	gptCommandOptionTemperature gptCommandOptionType = 4
)

func (t gptCommandOptionType) string() string {
	switch t {
	case gptCommandOptionPrompt:
		return "prompt"
	case gptCommandOptionContext:
		return "context"
	case gptCommandOptionModel:
		return "model"
	case gptCommandOptionTemperature:
		return "temperature"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func (t gptCommandOptionType) humanReadableString() string {
	switch t {
	case gptCommandOptionPrompt:
		return "Prompt"
	case gptCommandOptionContext:
		return "Context"
	case gptCommandOptionModel:
		return "Model"
	case gptCommandOptionTemperature:
		return "Temperature"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}
