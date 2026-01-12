package cmdutil

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func ConfirmPrompt(in io.Reader, out io.Writer, message string) (bool, error) {
	if message == "" {
		message = "Continue?"
	}
	if _, err := fmt.Fprintf(out, "%s [y/N]: ", message); err != nil {
		return false, err
	}
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}
