package aws

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"text/tabwriter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type InstanceData struct {
	InstanceID string
	Name       string
	State      string
	Type       string
	PrivateIP  string
}

// listECSClusters lists all ECS clusters and outputs them in a table format.
func ListECSClusters(awsProfile string) error {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: awsProfile, // Specify the profile name here
		Config: aws.Config{
			Region: aws.String("us-west-2"), // Set your AWS region here
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	svc := ecs.New(sess)
	input := &ecs.ListClustersInput{}
	result, err := svc.ListClusters(input)
	if err != nil {
		return fmt.Errorf("failed to list clusters: %v", err)
	}

	// Extract and sort cluster names from ARNs
	var clusterNames []string
	for _, arn := range result.ClusterArns {
		splitARN := strings.Split(*arn, "/")
		name := splitARN[len(splitARN)-1] // Assumes the cluster name is the last segment of the ARN
		clusterNames = append(clusterNames, name)
	}
	sort.Strings(clusterNames) // Sort the cluster names alphabetically

	// Output the cluster names in a table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Cluster Name\t")
	fmt.Fprintln(w, "-------------\t")
	for _, name := range clusterNames {
		fmt.Fprintf(w, "%s\t\n", name)
	}
	w.Flush()

	return nil
}

func FetchEC2InstanceData(clusterName string, awsProfile string, onlyRunning bool) ([]InstanceData, error) {
	var instances []InstanceData

	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: awsProfile,
		Config: aws.Config{
			Region: aws.String("us-west-2"), // Set your AWS region here
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	ecsSvc := ecs.New(sess)
	ec2Svc := ec2.New(sess)

	ecsParams := &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	}
	ecsResp, err := ecsSvc.ListContainerInstances(ecsParams)
	if err != nil {
		return nil, fmt.Errorf("error listing container instances for cluster %s: %v", clusterName, err)
	}

	if len(ecsResp.ContainerInstanceArns) == 0 {
		log.Println("No container instances found for cluster:", clusterName)
		return nil, nil
	}

	describeParams := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterName),
		ContainerInstances: ecsResp.ContainerInstanceArns,
	}
	describeResp, err := ecsSvc.DescribeContainerInstances(describeParams)
	if err != nil {
		return nil, fmt.Errorf("error describing container instances: %v", err)
	}

	var instanceIds []*string
	for _, instance := range describeResp.ContainerInstances {
		instanceIds = append(instanceIds, instance.Ec2InstanceId)
	}

	ec2Params := &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}
	ec2Resp, err := ec2Svc.DescribeInstances(ec2Params)
	if err != nil {
		return nil, fmt.Errorf("error describing EC2 instances: %v", err)
	}

	for _, reservation := range ec2Resp.Reservations {
		for _, instance := range reservation.Instances {
			instanceName := "Unnamed"
			for _, tag := range instance.Tags {
				if *tag.Key == "Name" {
					instanceName = *tag.Value
					break
				}
			}
			if onlyRunning && *instance.State.Name != "running" {
				continue
			}
			instances = append(instances, InstanceData{
				InstanceID: aws.StringValue(instance.InstanceId),
				Name:       instanceName,
				State:      aws.StringValue(instance.State.Name),
				Type:       aws.StringValue(instance.InstanceType),
				PrivateIP:  aws.StringValue(instance.PrivateIpAddress),
			})
		}
	}

	// Sorting instances by Name
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Name < instances[j].Name
	})

	return instances, nil
}

func DisplayEC2Instances(instances []InstanceData) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(writer, "Instance ID\tName\tState\tType\tPrivate IP") // Print header
	for _, instance := range instances {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			instance.InstanceID,
			instance.Name,
			instance.State,
			instance.Type,
			instance.PrivateIP)
	}
	writer.Flush() // Ensure all buffered operations are applied to the writer
}
