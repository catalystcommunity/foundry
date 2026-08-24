// Package console carries operator-facing input and output.
//
// Foundry's operations print progress to the console and sometimes ask for a
// password. The CLI reads and writes the terminal, but the web UI runs the
// same operations in a process nobody is watching, so it needs the output as
// data and needs a way to answer a prompt from the browser. This package
// supplies both without every operation having to know which one is driving.
package console

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// Capture sends every line written to os.Stdout and os.Stderr to handler
// while fn runs. Output still reaches the original streams, so a terminal
// session shows the same text it always did.
//
// Capture replaces process-wide streams, so only one may run at a time.
func Capture(handler func(line string), fn func() error) error {
	if handler == nil {
		return fn()
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		// Capturing output is a convenience; never fail the work over it
		return fn()
	}

	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer

	var readerDone sync.WaitGroup
	readerDone.Add(1)
	go func() {
		defer readerDone.Done()
		scanner := bufio.NewScanner(reader)
		// Allow long lines, such as an embedded manifest
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			handler(line)
			io.WriteString(originalStdout, line+"\n")
		}
	}()

	// Restore on the way out, including when fn panics
	defer func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		writer.Close()
		readerDone.Wait()
		reader.Close()
	}()

	return fn()
}

// Prompter asks the operator for input that configuration cannot supply.
// Implementations must be safe to call from any goroutine.
type Prompter interface {
	// Password asks for a secret. The value must never be echoed or logged.
	Password(message string) (string, error)
}

var (
	prompterMu     sync.RWMutex
	activePrompter Prompter = TerminalPrompter{}
)

// SetPrompter installs the prompter that AskPassword uses and returns a
// function that puts the previous one back.
func SetPrompter(prompter Prompter) func() {
	prompterMu.Lock()
	previous := activePrompter
	activePrompter = prompter
	prompterMu.Unlock()

	return func() {
		prompterMu.Lock()
		activePrompter = previous
		prompterMu.Unlock()
	}
}

// AskPassword asks the operator for a secret through the active prompter.
func AskPassword(message string) (string, error) {
	prompterMu.RLock()
	prompter := activePrompter
	prompterMu.RUnlock()

	return prompter.Password(message)
}

// TerminalPrompter reads a secret from the terminal without echoing it.
type TerminalPrompter struct{}

// Password prints the message and reads the reply from standard input.
func (TerminalPrompter) Password(message string) (string, error) {
	fmt.Print(message)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return string(secret), nil
}
