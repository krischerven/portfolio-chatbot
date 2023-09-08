package main

import (
	"bufio"
	"context"
	"fmt"
	openai "github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
)

const (
	instructions = `The following is information about a software engineer named Kris Cherven. There is a resume section and a facts section.
Information in the facts section must take priority over information in the resume section. The resume section starts after the text
"BEGINNING OF RESUME SECTION" and ends after the text "END OF RESUME SECTION". The facts section starts after the text "BEGINNING OF FACTS
SECTION" and ends after the text "END OF FACTS SECTION". After the facts section you will be asked a question. When answering questions about
the school Kris Cherven went to, keep in mind that you should consider Grand Circus Java Bootcamp to be his main school. Also, do not mention
the "facts section" or the "resume section" when answering questions. These are internal sections that are of no interest to the person
asking the question. Never use phrases like "based on the information provided."; instead, act as if you are a human being who knows all of
the information you are being asked for.`
)

var (
	facts = []string{
		// <2023-09-06 Wed> GPT-3 thinks its October 2022 unless you tell it otherwise
		"The current date is September 6, 2023.",
		"Kris Cherven is 24 years old.",
	}
	logger = log.New()
)

func fail(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func readFile(name string) string {
	bs, err := os.ReadFile(name)
	fail(err)
	return string(bs)
}

func initializeClient() *openai.Client {

	apiKey := readFile("API_KEY")
	if apiKey == "" {
		log.Fatal("Missing API key")
	}

	return openai.NewClient(apiKey)
}

func information() string {
	outFile := "instructions.txt"
	{
		fail(exec.Command("pdftotext", "resume.pdf").Run())
		fail(exec.Command("mv", "resume.txt", outFile).Run())
		text := readFile(outFile)
		text = strings.Replace(instructions, "\n", " ", -1) + "\n\nBEGINNING OF RESUME SECTION\n\n" + text + "\n\nEND OF RESUME SECTION\n\n"
		text = text + "BEGINNING OF FACTS SECTION\n\n" + strings.Join(facts, "\n") + "\n\nEND OF FACTS SECTION\n"
		os.WriteFile(outFile, []byte(text), 0644)
	}
	return readFile(outFile)
}

func answerQuestion(question string, client *openai.Client) string {

	// https://pkg.go.dev/github.com/PullRequestInc/go-gpt3#CompletionRequest
	resp, err := client.CreateChatCompletion(context.Background(),
		openai.ChatCompletionRequest{
			Model:     openai.GPT3Dot5Turbo,
			MaxTokens: 200,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: information() + "\n" + question,
				},
			},
		})

	fail(err)
	return resp.Choices[0].Message.Content
}

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "--question" {
			if len(os.Args) > 2 {
				fmt.Println(answerQuestion(os.Args[2], initializeClient()))
			} else {
				fmt.Println("Error: Missing question after '--question'.")
			}
		} else {
			fmt.Println("Error: Invalid argument(s). Try ./portfolio-chatbot --question \"Who is Kris?\"")
		}
	} else {
		logger.SetLevel(log.DebugLevel)
		scanner := bufio.NewScanner(os.Stdin)
		client := initializeClient()
		fmt.Println("(interactive mode) Hello! I am portfolio-chatbot. Please go ahead and ask me any questions you have about Kris!")
		for scanner.Scan() {
			fmt.Println(answerQuestion(scanner.Text(), client))
		}
	}
}
