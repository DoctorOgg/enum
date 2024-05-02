package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// SSHCommand executes a command on a remote host using SSH with the SSH agent and returns the output
func SSHCommand(host, command string) (string, error) {
	// Get the current system user
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("unable to get current user: %v", err)
	}

	// Connect to the SSH agent
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return "", fmt.Errorf("failed to connect to SSH agent: %v", err)
	}
	defer sshAgent.Close()

	agentClient := agent.NewClient(sshAgent)
	authMethod := ssh.PublicKeysCallback(agentClient.Signers)

	// Set up the SSH client configuration
	config := &ssh.ClientConfig{
		User: currentUser.Username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Note: Insecure; see below for production recommendation
	}

	// Establish the SSH connection
	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Create a new SSH session
	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Capture the output of the remote command
	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("failed to run command: %v", err)
	}

	return stdoutBuf.String(), nil
}

// SSHCommand executes a command on a remote host using SSH with the SSH agent and streams the output to the console
func SSHCommandStream(host, command string) error {
	// Get the current system user
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to get current user: %v", err)
	}

	// Connect to the SSH agent
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("failed to connect to SSH agent: %v", err)
	}
	defer sshAgent.Close()

	agentClient := agent.NewClient(sshAgent)
	authMethod := ssh.PublicKeysCallback(agentClient.Signers)

	// Set up the SSH client configuration
	config := &ssh.ClientConfig{
		User: currentUser.Username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Note: Insecure; should implement proper host key checking
	}

	// Establish the SSH connection
	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Create a new SSH session
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Connect session output directly to os.Stdout and os.Stderr
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Run the command
	err = session.Run(command)
	if err != nil {
		return fmt.Errorf("failed to run command: %v", err)
	}

	return nil
}

// SSHInteractiveShell starts a fully interactive shell session on the remote host via SSH
// If command is provided, it will be executed in the shell session.
func SSHInteractiveShell(host string, command string) error {
	// Get the current system user
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to get current user: %v", err)
	}

	// Connect to the SSH agent
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("failed to connect to SSH agent: %v", err)
	}
	defer sshAgent.Close()

	agentClient := agent.NewClient(sshAgent)
	authMethod := ssh.PublicKeysCallback(agentClient.Signers)

	// Set up the SSH client configuration
	config := &ssh.ClientConfig{
		User: currentUser.Username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Note: Insecure; should implement proper host key checking
	}

	// Establish the SSH connection
	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Create a new SSH session
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Request pseudo terminal
	if err := session.RequestPty("xterm", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,     // Disable echoing
		ssh.TTY_OP_ISPEED: 14400, // Set input speed
		ssh.TTY_OP_OSPEED: 14400, // Set output speed
	}); err != nil {
		return fmt.Errorf("request for pseudo terminal failed: %s", err)
	}

	// Save old terminal state and disable local echo
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %v", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Setup input and output to connect local and remote terminals
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	// Start remote shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %s", err)
	}

	// Forward local stdin to remote stdin
	go func() {
		defer stdinPipe.Close()
		_, err = io.Copy(stdinPipe, os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to forward local stdin:", err)
		}
	}()

	// Wait for the session to exit
	err = session.Wait()
	if err != nil {
		return fmt.Errorf("error waiting for SSH session: %v", err)
	}

	return nil
}
