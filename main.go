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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openai "github.com/sashabaranov/go-openai"
	logrus "github.com/sirupsen/logrus"
	"math/rand"
)

type debugMode_t int

const (
	__debugModeOff debugMode_t = iota
	debugModeSimple
	debugModeAdvanced
)

type rateLimitTestMode_t int

const (
	rateLimitByUUID rateLimitTestMode_t = iota
	rateLimitByIpAddrHash
	rateLimitByUUIDAndIpAddrHash
)

const (
	instructions1 = `You are an assistant who answers career-related questions about a software engineer named Kris Cherven. The following
is information about his career. In this information, there is a 'facts section' and a 'resume section'. Information in the facts section
takes priority over information in the resume section. The resume section starts after the text BEGINNING OF RESUME SECTION and ends at the
text END OF RESUME SECTION. The facts section starts after the text BEGINNING OF FACTS SECTION and ends at the text END OF FACTS SECTION.
When answering questions about the school Kris Cherven went to, talk about Grand Circus Java Bootcamp. Do not mention the 'facts section'
or the 'resume section', or "the information provided" or any other meta-information provided in this paragraph when answering questions.
The information about Kris Cherven is as follows:`

	instructions2 = `Please answer the last of the following questions about Kris Cherven, using the preceding chat history as context.
In the chat history, you are "AI" and the questioner is "USER". However, new messages should never be prefixed with "AI:". Also remember
that you only have about 10 KB of chat history. Please try to answer the question briefly. If you do not understand the question, or if
the question is not a valid English question, please ask the questioner to clarify what they are asking:`

	debugMode = debugModeSimple

	storageLimitPerClient = 1024 * 10
	rateLimitMessage      = "Sorry, but please wait a couple minutes before sending another message."

	GCMessageThreshold = 10000
	GCTimeThreshold    = 7200
)

var (
	facts = []string{
		// <2023-09-06 Wed> GPT-3 thinks its October 2022 unless you tell it otherwise
		fmt.Sprintf("The current date is %s %d, %d.", time.Now().Month(), time.Now().Day(), time.Now().Year()),
		"Kris Cherven is 24 years old.",
	}
	log               = logrus.New()
	rateLimitCount    = 10
	rateLimitDelay    = 120
	rateLimitTestMode = rateLimitByUUIDAndIpAddrHash
	falseResponseN    = make(map[string]uint64)
)

func fail(err error) {
	if err != nil {
		panic(err)
	}
}

func assert(cond bool, message string) {
	if !cond {
		panic(message)
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

type Maybe_t[T any] struct {
	v  T
	ok bool
}

func Maybe[T any](x T) Maybe_t[T] {
	return Maybe_t[T]{x, true}
}

type WIPsettings struct {
	chatbotEnabled    Maybe_t[bool]
	falseResponse     Maybe_t[bool]
	maxQuestionLength Maybe_t[int]
}

type settings struct {
	chatbotEnabled    bool
	falseResponse     bool
	maxQuestionLength int
}

func getSettings() settings {
	settings_ := WIPsettings{}

	loadSettings := func(fileName string, oldSettings *settings) settings {
		lines := strings.Split(readFile(fileName), "\n")
		for i, line := range lines {
			if i == len(lines)-1 && strings.Trim(line, " ") == "" {
				break
			}
			kv := strings.Split(line, "=")
			if len(kv) != 2 {
				log.Errorf("Found malformed line '%s' in %s; skipping", line, fileName)
				continue
			}
			setting := kv[0]
			val := kv[1]
			switch setting {
			case "chatbot-enabled":
				if val == "true" || val == "false" {
					b, err := strconv.ParseBool(val)
					settings_.chatbotEnabled = Maybe(b)
					fail(err)
				} else {
					log.Fatalf("%s: Setting '%s' has invalid val '%v'", fileName, setting, val)
				}
			case "false-response":
				if val == "true" || val == "false" {
					b, err := strconv.ParseBool(val)
					settings_.falseResponse = Maybe(b)
					fail(err)
				} else {
					log.Fatalf("%s: Setting '%s' has invalid val '%v'", fileName, setting, val)
				}
			case "max-question-length":
				len, err := strconv.ParseUint(val, 10, 16)
				if err == nil {
					settings_.maxQuestionLength = Maybe(int(len))
				} else {
					log.Fatalf("%s: Setting '%s' has invalid val '%v'", fileName, setting, val)
				}
			default:
				log.Errorf("%s: Found setting '%s' with val '%v', but it's not a valid setting.", fileName, setting, val)
			}
		}
		if oldSettings == nil {
			if !settings_.chatbotEnabled.ok {
				log.Fatalf("%s: Missing setting: chatbot-enabled", fileName)
			}
			if !settings_.falseResponse.ok {
				log.Fatalf("%s: Missing setting: false-response", fileName)
			}
			if !settings_.maxQuestionLength.ok {
				log.Fatalf("%s: Missing setting: max-question-length", fileName)
			}
		} else {
			if !settings_.chatbotEnabled.ok {
				settings_.chatbotEnabled = Maybe(oldSettings.chatbotEnabled)
			}
			if !settings_.falseResponse.ok {
				settings_.falseResponse = Maybe(oldSettings.falseResponse)
			}
			if !settings_.maxQuestionLength.ok {
				settings_.maxQuestionLength = Maybe(oldSettings.maxQuestionLength)
			}
		}
		return settings{
			settings_.chatbotEnabled.v,
			settings_.falseResponse.v,
			settings_.maxQuestionLength.v,
		}
	}

	settings := loadSettings("./settings", nil)
	if fileExists("./local-settings") {
		settings = loadSettings("./local-settings", &settings)
	}

	return settings
}

func debugln(yes bool, x ...interface{}) {
	if !yes {
		return
	}
	assert(len(x) > 0, "debugln called with only one argument")
	switch x[0].(type) {
	case string:
		// go vet (and therefore go test) fails if we call a custom function with <fmt.Printf>-style
		// formatting directives (e.g., %s, %d), so we use dollar-sign style directives (e.g., $s, $d) instead
		fmt.Printf(strings.Replace(x[0].(string), "$", "%", -1)+"\n", x[1:]...)
	default:
		fmt.Println(x...)
	}
}

func answerQuestion(uuid string, ipAddrHash string, question string, settings settings, ctx context.Context,
	conn *pgx.Conn, client *openai.Client, debugMode debugMode_t) string {

	if settings.chatbotEnabled == false {
		return "Sorry, but I cannot answer your question at the moment. Please try again later."
	}

	// Only relevant when portfolio-chatbot is run interactively; It's impossible to send empty messages via the frontend
	if len(strings.Trim(question, " \t\n\r\v\f")) == 0 {
		return "Please ask me a question."
	}

	if len(question) > settings.maxQuestionLength {
		return fmt.Sprintf("You question is too long (>%d characters). Please condense it and try again.",
			settings.maxQuestionLength)
	}

	exec := func(query string, args ...any) {
		_, err := conn.Exec(ctx, query, args...)
		fail(err)
	}

	query := func(query string, args ...any) pgx.Rows {
		rows, err := conn.Query(ctx, query, args...)
		fail(err)
		return rows
	}

	rows := query(`SELECT key
								 FROM ratelimit
								 WHERE (key = $1 OR key = $2)
								 AND count >= $3
								 AND EXTRACT(EPOCH FROM (current_timestamp-timestamp_)) < $4`,
		uuid, ipAddrHash, rateLimitCount, rateLimitDelay)

	defer fail(rows.Err())
	defer rows.Close()

	if rows.Next() {
		return rateLimitMessage
	}

	for _, key := range []string{uuid, ipAddrHash} {
		rows = query(`SELECT key
									FROM ratelimit
									WHERE key = $1
									AND count > 1
									AND EXTRACT(EPOCH FROM (current_timestamp-timestamp_)) >= $2`,
			key, rateLimitDelay)

		if rows.Next() {
			rows.Close()
			fail(rows.Err())
			exec("UPDATE ratelimit SET count = 0 WHERE key = $1", key)
		} else {
			rows.Close()
			fail(rows.Err())
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Every ~10,000 messages (within an order of magnitude of 10 MB of data), cleanup
	// messages older than 2 hours (GCTimeThreshold seconds). FIXME write a rationale for this
	rnum := rng.Intn(GCMessageThreshold)

	if rnum < 1 {
		debugln(debugMode >= debugModeSimple, "Running random GC")
		exec(`DELETE FROM message_queue
					WHERE id IN (
							SELECT id
							FROM message_queue
							WHERE uuid = (
								SELECT uuid
								FROM last_activity
								WHERE uuid = $1
								LIMIT 1
							)
						  AND EXTRACT(EPOCH FROM (current_timestamp-timestamp_)) >= $2
					)`, uuid, GCTimeThreshold)
	}

	exec("INSERT INTO message_queue (uuid, message) VALUES ($1, $2)",
		uuid, fmt.Sprintf("USER: %s", question))

	exec(`INSERT INTO last_activity (uuid) VALUES ($1)
													 ON CONFLICT (uuid)
													 DO UPDATE SET timestamp_ = DEFAULT`, uuid)

	if rateLimitTestMode == rateLimitByUUID || rateLimitTestMode == rateLimitByUUIDAndIpAddrHash {
		exec(`INSERT INTO ratelimit (key) VALUES ($1)
														ON CONFLICT (key)
														DO UPDATE SET count = ratelimit.count + 1, timestamp_ = DEFAULT`, uuid)
	}

	if rateLimitTestMode == rateLimitByIpAddrHash || rateLimitTestMode == rateLimitByUUIDAndIpAddrHash {
		exec(`INSERT INTO ratelimit (key) VALUES ($1)
													 ON CONFLICT (key)
													 DO UPDATE SET count = ratelimit.count + 1, timestamp_ = DEFAULT`, ipAddrHash)
	}

	rows = query("SELECT message FROM message_queue WHERE uuid = $1 ORDER BY timestamp_ ASC", uuid)

	defer fail(rows.Err())
	defer rows.Close()

	var recentQuestions []string
	var questionSizes []int
	var questionsSize int

	for rows.Next() {
		var question string
		if err := rows.Scan(&question); err != nil {
			panic(err)
		}
		recentQuestions = append(recentQuestions, question)
		questionSizes = append(questionSizes, len(question))
		questionsSize += len(question)
	}

	// Start deleting questions after this user has consumed more than ~10KB storage
	removedOldStorage := false
	for questionsSize > storageLimitPerClient {
		removedOldStorage = true
		uuid_prefix := strings.Split(uuid, "-")[0]
		if debugMode >= debugModeSimple {
			fmt.Printf("Nuking questions for %s... due to storage exceeding %dKB (%d bytes left)\n",
				uuid_prefix,
				storageLimitPerClient/1024,
				questionsSize)
		}
		exec(`DELETE FROM message_queue
					WHERE id = (
						SELECT id
						FROM message_queue
						WHERE uuid = $1
						ORDER BY timestamp_ ASC
						LIMIT 1
				)`, uuid)
		questionsSize -= questionSizes[0]
		questionSizes = questionSizes[1:]
	}

	if removedOldStorage {
		debugln(debugMode >= debugModeSimple, "Done nuking questions for $s. New storage is $d bytes\n", uuid, questionsSize)
	}

	debugln(debugMode >= debugModeSimple,
		"--- BEGIN PREVIOUS CONVERSATION LOG ---\n"+strings.Join(recentQuestions, "\n")+"\n--- END PREVIOUS CONVERSATION LOG ---")

	content := information() + "\n" + strings.Join(recentQuestions, "\n")

	if settings.falseResponse || client == nil {
		falseResponseN[uuid]++
		response := fmt.Sprintf("Response message #%d", falseResponseN[uuid])
		exec(`INSERT INTO message_queue (uuid, message) VALUES ($1, $2)`, uuid, fmt.Sprintf("AI: %s", response))
		return response
	} else {
		// https://pkg.go.dev/github.com/sashabaranov/go-openai#Client.CreateChatCompletion
		resp, err := client.CreateChatCompletion(ctx,
			openai.ChatCompletionRequest{
				Model:     openai.GPT4,
				MaxTokens: 200,
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: content,
					},
				},
			})

		fail(err)
		response := resp.Choices[0].Message.Content
		exec(`INSERT INTO message_queue (uuid, message) VALUES ($1, $2)`, uuid, fmt.Sprintf("AI: %s", response))

		return response
	}
}

func setupDB(ctx context.Context) *pgx.Conn {

	conn, err := pgx.Connect(ctx, "postgres://portfolio_cb_user@localhost:5432/portfolio_cb")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}

	exec := func(query string, args ...any) {
		_, err := conn.Exec(ctx, query, args...)
		fail(err)
	}

	exec(`CREATE TABLE IF NOT EXISTS message_queue (id SERIAL PRIMARY KEY,
																									uuid TEXT,
																									message TEXT,
																									timestamp_ TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	exec(`CREATE TABLE IF NOT EXISTS last_activity (uuid TEXT PRIMARY KEY,
																									timestamp_ TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	exec(`CREATE TABLE IF NOT EXISTS ratelimit (key TEXT PRIMARY KEY,
																							count INTEGER DEFAULT 1,
																							timestamp_ TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	return conn
}

func main() {

	settings := getSettings() // handle failure early
	ctx := context.Background()
	conn := setupDB(ctx)
	defer conn.Close(ctx)

	if len(os.Args) > 1 {
		// command mode
		if len(os.Args) == 4 {
			fmt.Println(answerQuestion(os.Args[1], os.Args[2], os.Args[3], settings, ctx, conn, initializeClient(), __debugModeOff))
		} else {
			fmt.Println("Error: Wrong format: Should be ./portfolio-chatbot {uuid} {ipAddrHash} \"{question}\".")
		}
	} else {
		// interactive mode
		log.SetLevel(logrus.DebugLevel)
		scanner := bufio.NewScanner(os.Stdin)
		uuid_ := uuid.NewString()
		fakeAddressHash := uuid.NewString()
		client := initializeClient()
		fmt.Println("(interactive mode) Hello! I am portfolio-chatbot. Please go ahead and ask me any questions you have about Kris!")
		for scanner.Scan() {
			settings = getSettings()
			fmt.Println(answerQuestion(uuid_, fakeAddressHash, scanner.Text(), settings, ctx, conn, client, debugMode))
		}
	}
}
