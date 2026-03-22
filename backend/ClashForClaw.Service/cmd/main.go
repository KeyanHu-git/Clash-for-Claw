package main

import (
	"os"

	"clash-for-claw-service/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}

