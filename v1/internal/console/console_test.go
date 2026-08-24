package console

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureCollectsPrintedLines(t *testing.T) {
	var lines []string

	err := Capture(func(line string) { lines = append(lines, line) }, func() error {
		fmt.Println("first line")
		fmt.Printf("second %s\n", "line")
		fmt.Fprintln(os.Stderr, "error line")
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"first line", "second line", "error line"}, lines)
}

func TestCaptureRestoresStreamsAndReturnsError(t *testing.T) {
	originalStdout := os.Stdout

	err := Capture(func(string) {}, func() error {
		fmt.Println("work happened")
		return fmt.Errorf("work failed")
	})

	require.Error(t, err)
	assert.Equal(t, "work failed", err.Error())
	assert.Equal(t, originalStdout, os.Stdout, "stdout must be restored")
}

func TestCaptureRestoresStreamsOnPanic(t *testing.T) {
	originalStdout := os.Stdout

	assert.Panics(t, func() {
		_ = Capture(func(string) {}, func() error {
			panic("boom")
		})
	})

	assert.Equal(t, originalStdout, os.Stdout, "stdout must be restored after a panic")
}

func TestCaptureWithoutHandlerStillRuns(t *testing.T) {
	ran := false
	err := Capture(nil, func() error {
		ran = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, ran)
}

// stubPrompter records what it was asked and replies with a fixed answer.
type stubPrompter struct {
	asked  string
	answer string
	err    error
}

func (s *stubPrompter) Password(message string) (string, error) {
	s.asked = message
	return s.answer, s.err
}

func TestAskPasswordUsesActivePrompter(t *testing.T) {
	stub := &stubPrompter{answer: "hunter2"}
	restore := SetPrompter(stub)
	defer restore()

	answer, err := AskPassword("Enter root password: ")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", answer)
	assert.Equal(t, "Enter root password: ", stub.asked)
}

func TestSetPrompterRestoresPrevious(t *testing.T) {
	first := &stubPrompter{answer: "first"}
	restoreFirst := SetPrompter(first)

	second := &stubPrompter{answer: "second"}
	restoreSecond := SetPrompter(second)

	answer, err := AskPassword("prompt")
	require.NoError(t, err)
	assert.Equal(t, "second", answer)

	restoreSecond()
	answer, err = AskPassword("prompt")
	require.NoError(t, err)
	assert.Equal(t, "first", answer)

	restoreFirst()
}

func TestAskPasswordReturnsPrompterError(t *testing.T) {
	stub := &stubPrompter{err: fmt.Errorf("no terminal")}
	restore := SetPrompter(stub)
	defer restore()

	_, err := AskPassword("prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no terminal")
}
