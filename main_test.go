package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func testAssert(t *testing.T, cond bool) {
	if !cond {
		t.Fail()
	}
}

func testRateLimit(t *testing.T) {

	// globals
	rateLimitCount = 5
	rateLimitDelay = 1

	ctx := context.Background()
	conn := setupDB(ctx)
	defer conn.Close(ctx)

	uuid_ := uuid.NewString()
	ipAddrHash := uuid.NewString()
	question := "Where is Kris?"
	debugMode := __debugModeOff

	for i := 0; i < rateLimitCount; i++ {
		if answerQuestion(uuid_, ipAddrHash, question, getSettings(), ctx, conn, nil, debugMode) !=
			fmt.Sprintf("Response message #%d", i+1) {
			t.Fail()
		}
	}

	if answerQuestion(uuid_, ipAddrHash, question, getSettings(), ctx, conn, nil, debugMode) !=
		rateLimitMessage {
		t.Fail()
	}

	time.Sleep(time.Second * time.Duration(rateLimitDelay))

	for i := rateLimitCount; i < rateLimitCount*2; i++ {
		if answerQuestion(uuid_, ipAddrHash, question, getSettings(), ctx, conn, nil, debugMode) !=
			fmt.Sprintf("Response message #%d", i+1) {
			t.Fail()
		}
	}

	if answerQuestion(uuid_, ipAddrHash, question, getSettings(), ctx, conn, nil, debugMode) !=
		rateLimitMessage {
		t.Fail()
	}

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
		rows, err := conn.Query(ctx, query, args...)
		fail(err)
		return rows
	}

	getDataSoFar := func() []string {
		rows := query("SELECT message FROM message_queue WHERE uuid = $1 ORDER BY timestamp_ ASC", uuid_)

		defer fail(rows.Err())
		defer rows.Close()

		var recentQuestions []string

		for rows.Next() {
			var question string
			if err := rows.Scan(&question); err != nil {
				panic(err)
			}
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
