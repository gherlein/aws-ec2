package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

const cloudFormationTemplate = `
AWSTemplateFormatVersion: '2010-09-09'
Description: Lowest cost x86 EC2 instance (t2.nano) with SSH access

Parameters:
  LatestAmiId:
    Type: AWS::SSM::Parameter::Value<AWS::EC2::Image::Id>
    Default: /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64
  GitHubUsername:
    Type: String
    Description: GitHub username to fetch SSH public keys from

Resources:
  SSHSecurityGroup:
    Type: AWS::EC2::SecurityGroup
    Properties:
      GroupDescription: Allow SSH inbound traffic
      SecurityGroupIngress:
        - IpProtocol: tcp
          FromPort: 22
          ToPort: 22
          CidrIp: 0.0.0.0/0
      Tags:
        - Key: Name
          Value: SSHAccess

  EC2Instance:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: t3.micro
      ImageId: !Ref LatestAmiId
      SecurityGroupIds:
        - !GetAtt SSHSecurityGroup.GroupId
      UserData:
        Fn::Base64: !Sub |
          #!/bin/bash
          set -e

          GITHUB_USER="${GitHubUsername}"

          # Create user with sudo access
          useradd -m -s /bin/bash $GITHUB_USER
          echo "$GITHUB_USER ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/$GITHUB_USER

          # Setup SSH directory
          SSH_DIR="/home/$GITHUB_USER/.ssh"
          AUTH_KEYS="$SSH_DIR/authorized_keys"

          mkdir -p $SSH_DIR
          chmod 700 $SSH_DIR

          # Download public keys from GitHub
          curl -s "https://github.com/${GitHubUsername}.keys" > $AUTH_KEYS

          # Set correct permissions
          chmod 600 $AUTH_KEYS
          chown -R $GITHUB_USER:$GITHUB_USER $SSH_DIR

          echo "User $GITHUB_USER created with SSH keys from GitHub"
      Tags:
        - Key: Name
          Value: LowestCostX86Instance

Outputs:
  InstanceId:
    Description: Instance ID
    Value: !Ref EC2Instance
  PublicIP:
    Description: Public IP Address
    Value: !GetAtt EC2Instance.PublicIp
  SSHCommand:
    Description: SSH command to connect
    Value: !Sub "ssh ${GitHubUsername}@${EC2Instance.PublicIp}"
  InstanceType:
    Description: Instance Type
    Value: t3.micro
  SecurityGroupId:
    Description: Security Group ID
    Value: !Ref SSHSecurityGroup
`

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <github-username>", os.Args[0])
	}
	githubUsername := os.Args[1]

	ctx := context.Background()

	// Load AWS config from environment (AWS_REGION, credentials)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", cfg.Region)
	fmt.Printf("GitHub Username: %s\n", githubUsername)

	client := cloudformation.NewFromConfig(cfg)

	stackName := "lowest-cost-x86-instance"

	// Create the CloudFormation stack
	input := &cloudformation.CreateStackInput{
		StackName:    &stackName,
		TemplateBody: stringPtr(cloudFormationTemplate),
		Parameters: []types.Parameter{
			{
				ParameterKey:   stringPtr("GitHubUsername"),
				ParameterValue: stringPtr(githubUsername),
			},
		},
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
		},
		Tags: []types.Tag{
			{
				Key:   stringPtr("Purpose"),
				Value: stringPtr("LowestCostX86"),
			},
		},
	}

	result, err := client.CreateStack(ctx, input)
	if err != nil {
		log.Fatalf("failed to create stack: %v", err)
	}

	fmt.Printf("Stack creation initiated!\n")
	fmt.Printf("Stack ID: %s\n", *result.StackId)
	fmt.Printf("Waiting for stack to complete...\n")

	// Wait for stack creation to complete
	waiter := cloudformation.NewStackCreateCompleteWaiter(client)
	err = waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 10*time.Minute)
	if err != nil {
		log.Fatalf("failed waiting for stack: %v", err)
	}

	// Get stack outputs
	describeOutput, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		log.Fatalf("failed to describe stack: %v", err)
	}

	fmt.Printf("\n=== Stack Created Successfully ===\n")
	for _, output := range describeOutput.Stacks[0].Outputs {
		fmt.Printf("%s: %s\n", *output.OutputKey, *output.OutputValue)
	}
}

func stringPtr(s string) *string {
	return &s
}
