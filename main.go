package main

import (
	"context"
	"fmt"
	openai "github.com/sashabaranov/go-openai"
	"log"
	"os"
)

func fail(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func answerQuestion(question string) string {
	str, err := os.ReadFile("API_KEY")
	fail(err)

	apiKey := string(str)
	if apiKey == "" {
		log.Fatalln("Missing API key")
	}

	str, err = os.ReadFile("instructions.txt")
	fail(err)

	instructions := string(str)
	if instructions == "" {
		log.Fatalln("Missing instructions")
	}

	client := openai.NewClient(apiKey)

	// https://pkg.go.dev/github.com/PullRequestInc/go-gpt3#CompletionRequest
	resp, err := client.CreateChatCompletion(context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: instructions + "\n\n" + question,
				},
			},
		})

	fail(err)
	return resp.Choices[0].Message.Content
}

func main() {
	for {
		var question string
		_, err := fmt.Scanln(&question)
		if err != nil {
			continue
		}
		fmt.Println(answerQuestion(question))
	}
}
