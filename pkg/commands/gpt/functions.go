package gpt

import "github.com/sashabaranov/go-openai"

func googleSearchGPTFunction() *openai.FunctionDefine {
	return &openai.FunctionDefine{
		Name:        "googlesearch",
		Description: "",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"query": {
					Type:        openai.JSONSchemaTypeString,
					Description: "The search query for google",
				},
			},
			Required: []string{"query"},
		},
	}
}
