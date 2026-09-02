package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/SomtoJF/lsm-tree/database"
	"github.com/SomtoJF/lsm-tree/initializer"
	"github.com/chzyer/readline"
)

func execCommand(db database.Database, command string) {
	input := strings.TrimSuffix(command, "\n")
	input = strings.TrimSpace(input)
	args := strings.Split(input, " ")
	switch args[0] {
	case "set":
		if len(args) != 3 {
			slog.Error("Error: The -set flag requires two arguments.")
		}
		key, err := strconv.Atoi(args[1])
		if err != nil {
			slog.Error("Error: Invalid key. %v", err)
		}
		value := args[2]
		_, err = db.Set(key, value)
		if err != nil {
			slog.Error("Error: Failed to set key-value pair. %v", err)
		}
		os.Stdout.WriteString("Key-value pair set successfully.")
	case "get":
		if len(args) != 2 {
			slog.Error("Error: The -get flag requires one argument.")
		}
		key, err := strconv.Atoi(args[1])
		if err != nil {
			slog.Error("Error: Invalid key.", err)
		}
		value, err := db.Get(key)
		if err != nil {
			slog.Error("Error: Key not found.", err)
		}
		os.Stdout.WriteString(value)
	case "help":
		fmt.Println("Available commands:")
		fmt.Println("  set <key> <value> - Set a key-value pair")
		fmt.Println("  get <key>         - Get the value for a key")
		fmt.Println("  help              - Show this help message")
		fmt.Println("  exit              - Exit the program")
	case "exit":
		os.Exit(0)
	default:
		slog.Error("Error: Invalid command.")
		execCommand(db, "help")
	}
}

func main() {
	db := initializer.InitDB()
	log.Println("database initialized")

	rl, err := readline.New("> ")
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		input, err := rl.Readline()
		if err != nil {
			// Check if the error is a Ctrl+C (Interrupt) or Ctrl+D (EOF)
			if err == readline.ErrInterrupt {
				fmt.Println("\nReceived interrupt signal (Ctrl+C). Exiting...")
				break // Gracefully break the loop to allow defers to run
			} else if err == io.EOF {
				fmt.Println("\nReceived EOF (Ctrl+D). Exiting...")
				break
			}

			// Handle other actual errors
			fmt.Fprintln(os.Stderr, "Error:", err)
			continue
		}

		execCommand(db, input)

	}
}
