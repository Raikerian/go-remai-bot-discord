# GO-RemAI-Bot-Discord

| | |
|-|-|
| ![RemAI](https://media.discordapp.net/attachments/445276743515897860/1094553979326832690/Raikerian_Rem_from_ReZero_as_an_Artificial_Intelligence_from_th_30ad8d8c-6989-4c5a-9b1f-4dfb0442462b1.png?width=150&height=150) | A simple discord bot that makes ChatGPT requests and holds a conversation in discord threads. More features (like Dalle and, potentially, Bard) are coming soon!<br><br>The project is written in Golang because the whole point is for me to learn that language. |


## Quick start

1. Make sure you have docker CLI and make installed

1. Rename `credentials_example.yaml` to `credentials.yaml`. Fill in the required fields (like discord token and OpenAI token)

1. Simply run `make execute`

    > ***Note:*** Your bot must have `Message Content Intent` permission enabled in the Discord dev portal. We need to read messages in the threads to have a proper AI conversation.

1. `/info` in your server to list bot info such as version and commands
