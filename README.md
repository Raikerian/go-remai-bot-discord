# GO-RemAI-Bot-Discord

A simple discord bot that makes ChatGPT requests and holds a conversation in discord threads. More features (like Dalle and, potentially, Bard) are coming soon!

The project is written in Golang because the whole point is for me to learn that language.

## Quick start

1. Make sure you have docker CLI and make installed

1. Rename `credentials_example.yaml` to `credentials.yaml`. Fill in the required fields (like discord token and OpenAI token)

1. Simply run `make execute`

    > ***Note:*** Your bot must have `Message Content Intent` permission enabled in the Discord dev portal. We need to read messages in the threads to have a proper AI conversation.

1. `/info` in your server to list bot info such as version and commands
