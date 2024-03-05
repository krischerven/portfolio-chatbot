package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"testing"
	"time"
)

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
