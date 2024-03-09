package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"math"
)

func testAssert(t *testing.T, cond bool) {
	if !cond {
		t.Fail()
	}
}

func testRateLimit(t *testing.T) {

	settings := getSettings()

	// globals
	settings.rateLimitCount = 5
	// Anything less than ~30 is too high-resolution - ratelimits will never occur
	settings.rateLimitDelay = 30

	ctx := context.Background()
	conn := setupDB(ctx)
	defer conn.Close(ctx)

	uuid_ := uuid.NewString()
	ipAddrHash := uuid.NewString()
	question := "Where is Kris?"
	debugMode := __debugModeOff

	for i := 0; i < settings.rateLimitCount; i++ {
		testAssert(t, answerQuestion(uuid_, ipAddrHash, question, settings, ctx, conn, nil, debugMode) ==
			fmt.Sprintf("Response message #%d", i+1))
	}

	testAssert(t, answerQuestion(uuid_, ipAddrHash, question, settings, ctx, conn, nil, debugMode) ==
		rateLimitMessage(settings.rateLimitDelay/1000))

	time.Sleep(time.Millisecond * time.Duration(settings.rateLimitDelay))

	for i := settings.rateLimitCount; i < settings.rateLimitCount*2; i++ {
		testAssert(t, answerQuestion(uuid_, ipAddrHash, question, settings, ctx, conn, nil, debugMode) ==
			fmt.Sprintf("Response message #%d", i+1))
	}

	testAssert(t, answerQuestion(uuid_, ipAddrHash, question, settings, ctx, conn, nil, debugMode) ==
		rateLimitMessage(settings.rateLimitDelay/1000))

}

func TestRateLimitByUUID(t *testing.T) {
	rateLimitTestMode = rateLimitByUUID
	testRateLimit(t)
}

func TestRateLimitByIpAddrHash(t *testing.T) {
	rateLimitTestMode = rateLimitByIpAddrHash
	testRateLimit(t)
}

func TestRateLimitByUUIDAndIpAddrHash(t *testing.T) {
	rateLimitTestMode = rateLimitByUUIDAndIpAddrHash
	testRateLimit(t)
}

func TestHistoryPruning(t *testing.T) {

	ctx := context.Background()
	conn := setupDB(ctx)
	defer conn.Close(ctx)

	uuid_ := uuid.NewString()
	ipAddrHash := uuid.NewString()
	questions := []string{}

	for i := 0; i < 5; i++ {
		questions = append(questions, strings.Repeat(fmt.Sprintf("%d", i), 100))
	}

	storageLimitPerClient = 250

	query := func(query string, args ...any) pgx.Rows {
		return unwrap(conn.Query(ctx, query, args...))
	}

	getDataSoFar := func() []string {

		rows := query("SELECT message FROM message_queue WHERE uuid = $1 ORDER BY timestamp_ ASC", uuid_)
		defer finishRows(rows)

		var recentQuestions []string

		for rows.Next() {
			var question string
			fail(rows.Scan(&question))
			recentQuestions = append(recentQuestions, question)
		}

		return recentQuestions
	}

	debugMode := __debugModeOff

	printList := func(s []string) {
		fmt.Printf("[")
		for i, x := range s {
			fmt.Printf("%v", x)
			if i == len(s)-1 {
				fmt.Printf("]\n\n")
			} else {
				fmt.Println()
			}
		}
	}

	_ = printList

	answerQuestion(uuid_, ipAddrHash, questions[0], getSettings(), ctx, conn, nil, debugMode)
	testAssert(t, len(getDataSoFar()) == 2 && getDataSoFar()[0] == "USER: "+questions[0])
	// printList(getDataSoFar())

	answerQuestion(uuid_, ipAddrHash, questions[1], getSettings(), ctx, conn, nil, debugMode)
	testAssert(t, len(getDataSoFar()) == 4 && getDataSoFar()[2] == "USER: "+questions[1])
	// printList(getDataSoFar())

	answerQuestion(uuid_, ipAddrHash, questions[2], getSettings(), ctx, conn, nil, debugMode)
	testAssert(t, len(getDataSoFar()) == 4 && getDataSoFar()[2] == "USER: "+questions[2])
	// printList(getDataSoFar())

	answerQuestion(uuid_, ipAddrHash, questions[3], getSettings(), ctx, conn, nil, debugMode)
	testAssert(t, len(getDataSoFar()) == 4 && getDataSoFar()[2] == "USER: "+questions[3])
	// printList(getDataSoFar())

	answerQuestion(uuid_, ipAddrHash, questions[4], getSettings(), ctx, conn, nil, debugMode)
	testAssert(t, len(getDataSoFar()) == 4 && getDataSoFar()[2] == "USER: "+questions[4])
	// printList(getDataSoFar())
}

func TestMessageGC(t *testing.T) {

	iterationsI := 5
	// iterationsJ must be >= 2 because the GC runs before a message is sent
	iterationsJ := 2

	settings := getSettings()
	settings.rateLimitCount = iterationsI * iterationsJ

	// globals
	storageLimitPerClient = math.MaxInt64
	GCMessageThreshold = iterationsJ
	GCTimeThreshold = 100

	ctx := context.Background()
	conn := setupDB(ctx)
	defer conn.Close(ctx)

	uuid_ := uuid.NewString()
	ipAddrHash := uuid.NewString()
	question := "Where is Kris?"
	debugMode := __debugModeOff
	GCs := 0

	getMessageCount := func() int {
		query := func(query string, args ...any) pgx.Rows {
			return unwrap(conn.Query(ctx, query, args...))
		}
		rows := query(`SELECT count(message) FROM message_queue WHERE uuid = $1`, uuid_)
		var count int
		if rows.Next() {
			fail(rows.Scan(&count))
			finishRows(rows)
		}
		return count
	}

	// create settings.iterationsI*iterationsJ*2 messages
	for i := 0; i < iterationsI; i++ {
		for j := 0; j < iterationsJ; j++ {
			count := getMessageCount()
			answerQuestion(uuid_, ipAddrHash, question, settings, ctx, conn, nil, debugMode)
			if getMessageCount() < count {
				GCs++
			}
		}
		time.Sleep(time.Millisecond * time.Duration(GCTimeThreshold))
	}

	count := getMessageCount()
	fmt.Printf("TestMessageGC: count=%d\n", count)
	fmt.Printf("TestMessageGC: GCs=%d\n", GCs)
	// If one of these is true, the other must also be true. But it's trivial to test both anyway.
	testAssert(t, count < iterationsI*iterationsJ*2 && GCs > 0)
}

func BenchmarkFalseResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		conn := setupDB(ctx)
		defer conn.Close(ctx)

		uuid_ := uuid.NewString()
		ipAddrHash := uuid.NewString()
		question := "Where is Kris?"

		answerQuestion(uuid_, ipAddrHash, question, getSettings(), ctx, conn, nil, __debugModeOff)
	}
}
