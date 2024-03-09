package main

import (
	"errors"
	"os"
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

func unwrap[T any](v T, err error) T {
	fail(err)
	return v
}

func readFile(name string) string {
	bs, err := os.ReadFile(name)
	fail(err)
	return string(bs)
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

type Maybe_t[T any] struct {
	v  T
	ok bool
}

func Maybe[T any](x T) Maybe_t[T] {
	return Maybe_t[T]{x, true}
}
