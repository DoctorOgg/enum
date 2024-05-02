# Enum

`enum` is a command-line tool designed to help troubleshoot and manage ECS clusters using EC2 worker nodes. It allows users to enumerate instances, inspect containers, and manage logs directly from the terminal.

## Features

- List all EC2 instances in a specified ECS cluster.
- List all ECS clusters.
- Find running containers by search term.
- Inspect specific containers.
- Follow the logs of a specific container.
- Open an interactive shell session inside a specific container.

## Requirements

you have configured the aws cli, and have setup AWS_PROFILE environment variable.

## Usage

```bash
╰─➤  ./enum -h
This is a tool to help troubleshoot ECS clusters using ec2 worker nodes.

Usage:
  enum [flags]
  enum [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  find        Find running containers by search term
  help        Help about any command
  inspect     Inspect a container by its ID
  list-ec2    List EC2 instances for a cluster
  list-ecs    List ECS clusters
  logs        Follow the logs of a container by its ID
  shell       Start an interactive shell session in a specified container
  version     Print the version number of enum

Flags:
  -c, --cluster string   Name of the ECS cluster (required)
  -h, --help             help for enum

Use "enum [command] --help" for more information about a command.
```
