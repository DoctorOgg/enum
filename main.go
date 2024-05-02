package main

import (
	"fmt"
	"log"
	"os"

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
		Use:   "shell [container-id]",
		Short: "Start an interactive shell session in a specified container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			containerID := args[0]
			if err := shell(containerID); err != nil {
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
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile)
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
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile)
	if err != nil {
		log.Fatalf("Error fetching instances: %v", err)
	}

	for _, instance := range instances {
		if instance.PrivateIP != "" { // assuming SSH is possible
			var cmd string
			if searchTerm == "" {
				cmd = "sudo docker ps --format '{{.ID}} {{.Status}} {{.RunningFor}} {{.Names}}'"
			} else {
				cmd = fmt.Sprintf("sudo docker ps --format '{{.ID}} {{.Status}} {{.RunningFor}} {{.Names}}'  | grep '%s'", searchTerm)
			}
			output, err := ssh.SSHCommand(instance.PrivateIP, cmd)
			if err != nil {
				log.Printf("Error executing command on instance %s: %v", instance.InstanceID, err)
				continue
			}
			fmt.Printf("---------- %s ----------\n", instance.Name)
			fmt.Printf(output)
		}
	}
}

func inspectContainer(containerID string) error {
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	found := false
	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}
		cmd := fmt.Sprintf("sudo docker inspect %s", containerID)
		output, err := ssh.SSHCommand(instance.PrivateIP, cmd)
		if err != nil {
			log.Printf("Error executing command on instance %s: %v", instance.InstanceID, err)
			continue
		}
		if output != "" {
			fmt.Printf("---------- Inspect output from %s ----------\n", instance.Name)
			fmt.Println(output)
			found = true
			break
		}
	}

	if !found {
		fmt.Println("Container not found on any instance.")
		return nil
	}

	return nil
}

func followContainerLogs(containerID string) error {
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	found := false
	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}
		cmd := fmt.Sprintf("sudo docker logs -f %s", containerID)
		fmt.Printf("Attempting to follow logs on instance %s (%s)\n", instance.InstanceID, instance.Name)
		// Execute SSH command to follow logs, streaming directly to console
		err := ssh.SSHCommandStream(instance.PrivateIP, cmd)
		if err != nil {
			log.Printf("Error executing command on instance %s: %v", instance.InstanceID, err)
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

func shell(containerID string) error {

	// Fetch EC2 instances for the specified cluster
	instances, err := aws.FetchEC2InstanceData(ActiveConfig.ClusterName, awsProfile)
	if err != nil {
		return fmt.Errorf("error fetching EC2 instance data: %v", err)
	}

	// Flag to indicate if the container was found
	found := false

	// Loop through each EC2 instance
	for _, instance := range instances {
		if instance.PrivateIP == "" {
			continue
		}

		// SSH command to search for the container
		cmd := fmt.Sprintf("sudo docker ps -q --filter id=%s", containerID)
		output, err := ssh.SSHCommand(instance.PrivateIP, cmd)
		if err != nil {
			log.Printf("Error executing command on instance %s: %v", instance.InstanceID, err)
			continue
		}

		// If the container is found on this instance, start an interactive shell session
		if output != "" {
			fmt.Printf("Container %s found on instance %s (%s)\n", containerID, instance.Name, instance.InstanceID)
			// Start an interactive shell session in the container
			err := ssh.SSHInteractiveShell(instance.PrivateIP, fmt.Sprintf("sudo docker exec -it %s /bin/sh", containerID))
			if err != nil {
				log.Printf("Error starting interactive shell session: %v", err)
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
