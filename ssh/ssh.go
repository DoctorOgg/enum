package ssh

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// SSHCommand executes a command on a remote host using SSH with the SSH agent and returns the output
func SSHCommand(host, command string, verbose, ignoreExitCode bool) (string, error) {
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

	if verbose {
		fmt.Printf("Attempting to connect to SSH host %s@%s\n", currentUser.Username, host)
	}

	// Establish the SSH connection
	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return "", fmt.Errorf("failed to dial SSH: %v", err)
	}
	defer conn.Close()

	if verbose {
		fmt.Println("SSH connection established")
	}

	// Create a new SSH session
	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	if verbose {
		fmt.Printf("Running command: %s\n", command)
	}

	// Capture the output of the remote command
	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf
	err = session.Run(command)

	if err != nil {
		_, ok := err.(*ssh.ExitError)
		if ok && ignoreExitCode {
			// If ignoring exit codes, return the output anyway
			if verbose {
				fmt.Println("Ignoring failed exit code")
			}
			return stdoutBuf.String(), nil
		}
		return "", fmt.Errorf("failed to run command '%s': %v\nStderr: %s", command, err, stderrBuf.String())
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

func SSHInteractiveShell(host string, containerID string, command string) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to get current user: %v", err)
	}

	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("failed to connect to SSH agent: %v", err)
	}
	defer sshAgent.Close()

	agentClient := agent.NewClient(sshAgent)
	authMethod := ssh.PublicKeysCallback(agentClient.Signers)

	config := &ssh.ClientConfig{
		User: currentUser.Username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// This checks if the input is a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fd := int(os.Stdin.Fd())
		state, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to make terminal raw: %v", err)
		}
		defer term.Restore(fd, state)

		w, h, err := term.GetSize(fd)
		if err != nil {
			return fmt.Errorf("failed to get terminal size: %v", err)
		}

		if err := session.RequestPty("xterm", h, w, ssh.TerminalModes{
			ssh.ECHO:          1,     // enable echoing
			ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
		}); err != nil {
			return fmt.Errorf("request for pseudo terminal failed: %s", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Warning: The input device is not a TTY. Interactive session may not behave as expected.")
	}

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	// Concatenate shell command with arguments
	fullCommand := fmt.Sprintf("sudo docker exec -it %s %s", containerID, command)

	if fullCommand != "" {
		if err := session.Run(fullCommand); err != nil {
			return fmt.Errorf("failed to run command: %v", err)
		}
	} else {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("failed to start shell: %v", err)
		}
		if err := session.Wait(); err != nil {
			return fmt.Errorf("shell exited with error: %v", err)
		}
	}

	return nil
}
