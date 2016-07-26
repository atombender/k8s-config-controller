package main

import (
	"fmt"
	"strings"
)

func parseQualifiedResourceName(input string) (string, string, error) {
	parts := strings.Split(input, "/")
	if len(parts) == 1 {
		return "default", parts[0], nil
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("Expected qualified name, found %q", input)
	}
	return parts[0], parts[1], nil
}
