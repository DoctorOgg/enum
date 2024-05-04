package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func RunInteractiveCommand(command string, args []string) error {
	// Initialize the command with the provided arguments.
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	// Assign the standard output and error streams.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Get a pipe to the standard input of the command.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stdin pipe: %w", err)
	}

	// Start the command asynchronously.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// Interact with the command via standard input.
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Enter input for the command:")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	// Write input to the command's standard input.
	if _, err := io.WriteString(stdinPipe, input); err != nil {
		return fmt.Errorf("error sending input to command: %w", err)
	}

	// Close the stdinPipe to signify that no more input will be sent.
	if err := stdinPipe.Close(); err != nil {
		return fmt.Errorf("error closing stdin pipe: %w", err)
	}

	// Wait for the command to complete.
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command finished with error: %w", err)
	}

	fmt.Println("Command completed successfully")
	return nil
}

func RunCommand(command string, args []string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ() // Explicitly use the current process's environment.

	// Create a buffer to capture standard output
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out // Capturing stderr in the same buffer as stdout

	// Start the command and wait for it to complete
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("command finished with error: %w\nOutput: %s", err, out.String())
	}

	// Return the output of the command
	return out.String(), nil
}
