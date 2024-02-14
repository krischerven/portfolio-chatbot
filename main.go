/* *****************************************************************
   * main.go: The entire source code for the chatbot on this page. *
   * https://git.krischerven.info/dev/portfolio-chatbot            *
   ***************************************************************** */

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	logrus "github.com/sirupsen/logrus"
)

const (
	instructions1 = `You are an assistant who answers career-related questions about a software engineer named Kris Cherven. The following
is information about his career. In this information, there is a 'facts section' and a 'resume section'. Information in the facts section
takes priority over information in the resume section. The resume section starts after the text BEGINNING OF RESUME SECTION and ends at the
text END OF RESUME SECTION. The facts section starts after the text BEGINNING OF FACTS SECTION and ends at the text END OF FACTS SECTION.
When answering questions about the school Kris Cherven went to, talk about Grand Circus Java Bootcamp. Do not mention the 'facts section'
or the 'resume section', or "the information provided" or any other meta-information provided in this paragraph when answering questions.
The information about Kris Cherven is as follows:`

	instructions2 = `Please answer the following question about Kris Cherven. If you do not understand the question, or if the question is
not a valid English question, please ask the questioner to clarify what they are asking:`
)

var (
	facts = []string{
		// <2023-09-06 Wed> GPT-3 thinks its October 2022 unless you tell it otherwise
		fmt.Sprintf("The current date is %s %d, %d.", time.Now().Month(), time.Now().Day(), time.Now().Year()),
		"Kris Cherven is 24 years old.",
	}
	log = logrus.New()
)

func fail(err error) {
	if err != nil {
		panic(err)
	}
}

func readFile(name string) string {
	bs, err := os.ReadFile(name)
	fail(err)
	return string(bs)
}

func initializeClient() *openai.Client {

	apiKey := strings.TrimRight(readFile("API_KEY"), "\r\n")
	if apiKey == "" {
		log.Fatal("Missing API key")
	}

	return openai.NewClient(apiKey)
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	panic(err)
}

func information() string {
	outFile := "instructions.txt"
	{
		if fileExists("resume.pdf") {
			fail(exec.Command("pdftotext", "resume.pdf").Run())
			// Use the resume.pdf from the parent project (portfolio-webpage)
		} else if fileExists("../portfolio-webpage-untracked/resume.pdf") {
			fail(exec.Command("pdftotext", "../portfolio-webpage-untracked/resume.pdf").Run())
			fail(exec.Command("mv", "../portfolio-webpage-untracked/resume.txt", "resume.txt").Run())
		} else {
			log.Fatal("resume.pdf does not exist; aborting")
		}
		fail(exec.Command("mv", "resume.txt", outFile).Run())
		text := readFile(outFile)
		text = strings.Replace(instructions1, "\n", " ", -1) + "\n\nBEGINNING OF RESUME SECTION\n\n" + text + "\n\nEND OF RESUME SECTION\n\n"
		text = text + "BEGINNING OF FACTS SECTION\n\n" + strings.Join(facts, "\n") + "\n\nEND OF FACTS SECTION\n\n" + instructions2 + "\n"
		os.WriteFile(outFile, []byte(text), 0644)
	}
	return readFile(outFile)
}

type settings struct {
	chatbotEnabled bool
}

func getSettings() settings {
	settings := settings{}
	lines := strings.Split(readFile("./settings"), "\n")
	for i, line := range lines {
		if i == len(lines)-1 && strings.Trim(line, " ") == "" {
			break
		}
		kv := strings.Split(line, "=")
		if len(kv) != 2 {
			log.Errorf("Found malformed line '%s' in ./settings; skipping", line)
			continue
		}
		setting := kv[0]
		val := kv[1]
		switch setting {
		case "chatbot-enabled":
			if val == "true" || val == "false" {
				var err error
				settings.chatbotEnabled, err = strconv.ParseBool(val)
				fail(err)
			} else {
				log.Fatalf("Setting '%s' has invalid val '%v'", setting, val)
			}
		default:
			log.Errorf("Found setting '%s' with val '%v', but it's not a valid setting.", setting, val)
		}
	}
	return settings
}

func answerQuestion(question string, client *openai.Client) string {

	settings := getSettings()
	if settings.chatbotEnabled == false {
		return "Sorry, but I cannot answer your question at the moment. Please try again later."
	}

	content := information() + "\n" + question

	// https://pkg.go.dev/github.com/sashabaranov/go-openai#Client.CreateChatCompletion
	resp, err := client.CreateChatCompletion(context.Background(),
		openai.ChatCompletionRequest{
			Model:     openai.GPT3Dot5Turbo,
			MaxTokens: 200,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: content,
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
		log.SetLevel(logrus.DebugLevel)
		scanner := bufio.NewScanner(os.Stdin)
		client := initializeClient()
		fmt.Println("(interactive mode) Hello! I am portfolio-chatbot. Please go ahead and ask me any questions you have about Kris!")
		for scanner.Scan() {
			fmt.Println(answerQuestion(scanner.Text(), client))
		}
	}
}
