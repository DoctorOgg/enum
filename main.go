package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"enum/aws"
	"enum/ssh"

	"github.com/spf13/cobra"
)

var (
	version                    = "dev"
	commit                     = "none"
	date                       = "unknown"
	human_readable_comand_name = "enum"
	awsProfile                 = "default"
	ActiveConfig               Config
)

type Config struct {
	ClusterName string
}

func main() {
	awsProfile = os.Getenv("AWS_PROFILE")

	rootCmd := &cobra.Command{
		Use:   human_readable_comand_name,
		Short: "Enumerate this and that",
		Long:  `This is a tool to help troubleshoot ECS clusters using ec2 worker nodes.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVarP(&ActiveConfig.ClusterName, "cluster", "c", "", "Name of the ECS cluster (required)")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number of " + human_readable_comand_name,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s, commit %s, built at %s\n", human_readable_comand_name, version, commit, date)
		},
	})

	listEc2InstancesCmd := &cobra.Command{
		Use:   "list-ec2",
		Short: "List EC2 instances for a cluster",
		Run: func(cmd *cobra.Command, args []string) {
			if err := listEC2Instances(); err != nil {
				log.Printf("Error listing EC2 instances: %v", err)
			}
		},
	}
	rootCmd.AddCommand(listEc2InstancesCmd)

	listECSClusters := &cobra.Command{
		Use:   "list-ecs",
		Short: "List ECS clusters",
		Run: func(cmd *cobra.Command, args []string) {
			if err := aws.ListECSClusters(awsProfile); err != nil {
				log.Printf("Error listing ECS Clusters: %v", err)
			}
		},
	}
	rootCmd.AddCommand(listECSClusters)

	var searchTerm string
	findCmd := &cobra.Command{
		Use:   "find [search-term]",
		Short: "Find running containers by search term",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				find("")
			} else {
				searchTerm = args[0]
				find(searchTerm)
			}
		},
	}
	rootCmd.AddCommand(findCmd)

	inspectCmd := &cobra.Command{
		Use:   "inspect [container-id]",
		Short: "Inspect a container by its ID",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument
		Run: func(cmd *cobra.Command, args []string) {
			containerID := args[0]
			if err := inspectContainer(containerID); err != nil {
				log.Printf("Error inspecting container %s: %v", containerID, err)
			}
		},
	}
	rootCmd.AddCommand(inspectCmd)

	logsCmd := &cobra.Command{
		Use:   "logs [container-id]",
		Short: "Follow the logs of a container by its ID",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument
		Run: func(cmd *cobra.Command, args []string) {
			containerID := args[0]
			if err := followContainerLogs(containerID); err != nil {
				log.Printf("Error following logs for container %s: %v", containerID, err)
			}
		},
	}
	rootCmd.AddCommand(logsCmd)

	shellCmd := &cobra.Command{
		Use:   "shell [container-id] [shell] [args...]",
		Short: "Start an interactive shell session in a specified container with an optional shell",
		Args:  cobra.MinimumNArgs(1), // Requires at least one argument
		Run: func(cmd *cobra.Command, args []string) {
			containerID := args[0]
			shellArgs := args[1:]
			if err := shell(containerID, shellArgs); err != nil {
				log.Fatalf("Failed to start interactive session: %v", err)
			}
		},
	}
	rootCmd.AddCommand(shellCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func listEC2Instances() error {
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile, false)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	if len(instances) == 0 {
		log.Println("No EC2 instances found for the specified cluster.")
		return nil
	}

	aws.DisplayEC2Instances(instances)
	return nil
}

func find(searchTerm string) {
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile, true)
	if err != nil {
		log.Fatalf("Error fetching instances: %v", err)
	}

	// Define column widths.
	const (
		instanceWidth   = 20
		idWidth         = 12
		statusWidth     = 12
		runningForWidth = 15
		nameWidth       = 60
	)

	// Print the table header with fixed width for each column.
	fmt.Printf("%-*s %-*s %-*s %-*s %-*s\n",
		instanceWidth, "EC2 Instance",
		idWidth, "Container ID",
		statusWidth, "Status",
		runningForWidth, "Running For",
		nameWidth, "Container Name")

	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue // Skip if no SSH access
		}

		var cmd string
		if searchTerm == "" {
			cmd = "sudo docker ps --format '{{.Names}}\t{{.ID}}\t{{.Status}}\t{{.RunningFor}}'"
		} else {
			cleanedSearchTerm := strings.ReplaceAll(searchTerm, " ", "")
			cmd = fmt.Sprintf("sudo docker ps --format '{{.Names}}\t{{.ID}}\t{{.Status}}\t{{.RunningFor}}' | grep '%s'", cleanedSearchTerm)
		}

		// Execute the command and collect output
		output, err := ssh.SSHCommand(instance.PrivateIP, cmd, false, true)
		if err != nil {
			log.Printf("Error executing command on instance %s: %v", instance.Name, err)
			continue
		}

		// Split output by lines and format each line according to defined widths
		for _, line := range strings.Split(output, "\n") {
			if line != "" {
				parts := strings.Split(line, "\t")
				if len(parts) >= 4 { // Ensure the line has all expected fields to prevent errors
					fmt.Printf("%-*s %-*s %-*s %-*s %-*s\n",
						instanceWidth, instance.Name,
						idWidth, parts[1],
						statusWidth, parts[2],
						runningForWidth, parts[3],
						nameWidth, parts[0])
				}
			}
		}
	}
}

func inspectContainer(containerID string) error {
	// Fetch the list of EC2 instances in the cluster.
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile, true)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}

		// Check if the container is running on the instance.
		checkCmd := fmt.Sprintf("sudo docker ps --filter \"id=%s\" --format '{{.ID}}'", containerID)
		checkOutput, err := ssh.SSHCommand(instance.PrivateIP, checkCmd, false, false)
		if err != nil {
			log.Printf("Error checking container on instance %s: %v", instance.InstanceID, err)
			continue
		}
		if checkOutput == "" {
			continue // No container with the specified ID was found on this host.
		}

		// If the container ID matches the expected ID, inspect it.
		inspectCmd := fmt.Sprintf("sudo docker inspect %s", containerID)
		inspectOutput, err := ssh.SSHCommand(instance.PrivateIP, inspectCmd, false, false)
		if err != nil {
			log.Printf("Error executing inspect on instance %s: %v", instance.InstanceID, err)
			continue
		}

		if inspectOutput != "" {
			fmt.Printf("---------- Inspect output from %s ----------\n", instance.Name)
			fmt.Println(inspectOutput)
			return nil // Stop after successful inspection, as only one such container should exist.
		}
	}

	fmt.Println("Container not found on any instance.")
	return nil
}

func followContainerLogs(containerID string) error {
	// Fetch the list of EC2 instances in the cluster.
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile, true)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	found := false
	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}

		// Check if the container is running on the instance.
		checkCmd := fmt.Sprintf("sudo docker ps --filter \"id=%s\" --format '{{.ID}}'", containerID)
		checkOutput, err := ssh.SSHCommand(instance.PrivateIP, checkCmd, false, false)
		if err != nil {
			log.Printf("Error checking container on instance %s: %v", instance.InstanceID, err)
			continue
		}
		if checkOutput == "" {
			continue // No container with the specified ID was found on this host.
		}

		// If the container ID matches the expected ID, follow its logs.
		logCmd := fmt.Sprintf("sudo docker logs -f %s", containerID)
		fmt.Printf("Attempting to follow logs on instance %s (%s)\n", instance.InstanceID, instance.Name)
		// Execute SSH command to follow logs, streaming directly to console
		logErr := ssh.SSHCommandStream(instance.PrivateIP, logCmd)
		if logErr != nil {
			log.Printf("Error executing command on instance %s: %v", instance.InstanceID, logErr)
			continue
		}
		found = true
		break
	}

	if !found {
		fmt.Println("Container not found on any instance or unable to connect.")
	}

	return nil
}

func shell(containerID string, args []string) error {
	// Fetch EC2 instances for the specified cluster
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile, true)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	// Set default shell if no arguments are provided
	var fullCommand string
	if len(args) == 0 {
		fullCommand = "/bin/sh"
	} else {
		fullCommand = strings.Join(args, " ")
	}

	// Flag to indicate if the container was found
	found := false

	// Loop through each EC2 instance
	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}

		// SSH command to search for the container
		checkCmd := fmt.Sprintf("sudo docker ps --filter \"id=%s\" --format '{{.ID}}'", containerID)
		output, err := ssh.SSHCommand(instance.PrivateIP, checkCmd, false, false)
		if err != nil {
			log.Printf("Error executing command on instance %s: %v", instance.InstanceID, err)
			continue
		}

		// If the container is found on this instance, start an interactive shell session
		if output != "" {
			fmt.Printf("Container %s found on instance %s (%s). Starting shell session...\n", containerID, instance.InstanceID, instance.Name)
			err := ssh.SSHInteractiveShell(instance.PrivateIP, containerID, fullCommand)
			if err != nil {
				log.Printf("Error starting interactive shell session: %v", err)
				continue
			}
			found = true
			break
		}
	}

	if !found {
		fmt.Println("Container not found on any instance or unable to connect.")
	}

	return nil
}
