// Package config reads service settings from the environment. Every service's
// main.go used to define its own copy of these two helpers.
package config

import (
	"os"
	"strconv"
)

// Env returns the variable's value, or fallback when it is unset or empty.
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvInt returns the variable as a positive integer, or fallback when it is
// unset, empty, not a number, or not positive.
func EnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
