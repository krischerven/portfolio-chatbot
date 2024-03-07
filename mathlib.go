package main

import (
	"golang.org/x/exp/constraints"
	"math"
)

func Max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	}
	return y
}

func Ceil[T constraints.Float](x T) int {
	return int(math.Ceil(float64(x)))
}
