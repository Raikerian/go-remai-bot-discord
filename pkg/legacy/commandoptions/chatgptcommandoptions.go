package commandoptions

import "fmt"

type ChatGPTCommandOptionType uint8

const (
	ChatGPTCommandOptionPrompt  ChatGPTCommandOptionType = 1
	ChatGPTCommandOptionContext ChatGPTCommandOptionType = 2
	ChatGPTCommandOptionModel   ChatGPTCommandOptionType = 3
)

func (t ChatGPTCommandOptionType) String() string {
	switch t {
	case ChatGPTCommandOptionPrompt:
		return "prompt"
	case ChatGPTCommandOptionContext:
		return "context"
	case ChatGPTCommandOptionModel:
		return "model"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}

func (t ChatGPTCommandOptionType) HumanReadableString() string {
	switch t {
	case ChatGPTCommandOptionPrompt:
		return "Prompt"
	case ChatGPTCommandOptionContext:
		return "Context"
	case ChatGPTCommandOptionModel:
		return "Model"
	}
	return fmt.Sprintf("ApplicationCommandOptionType(%d)", t)
}
