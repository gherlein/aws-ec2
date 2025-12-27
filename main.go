package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername  string `json:"github_username"`
	InstanceType    string `json:"instance_type,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	Domain          string `json:"domain,omitempty"`
	TTL             int    `json:"ttl,omitempty"`
	CloudInitScript string `json:"cloudinit_script,omitempty"`
	Ports           string `json:"ports,omitempty"`     // Comma-separated list of ports (e.g., "22,80,443")
	BaseImage       string `json:"base_image,omitempty"` // SSM parameter path or AMI ID

	// Output fields (program fills in)
	StackName     string `json:"stack_name,omitempty"`
	StackID       string `json:"stack_id,omitempty"`
	Region        string `json:"region,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	PublicIP      string `json:"public_ip,omitempty"`
	SecurityGroup string `json:"security_group,omitempty"`
	ZoneID        string `json:"zone_id,omitempty"`
	FQDN          string `json:"fqdn,omitempty"`
	SSHCommand    string `json:"ssh_command,omitempty"`
}

// Default AMI - Amazon Linux 2023 x86_64
const defaultBaseImage = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"

const cloudFormationTemplateHeader = `
AWSTemplateFormatVersion: '2010-09-09'
Description: EC2 instance with SSH access

Parameters:
  AmiId:
    Type: String
    Description: AMI ID for the EC2 instance
  GitHubUsername:
    Type: String
    Description: GitHub username to fetch SSH public keys from
  InstanceType:
    Type: String
    Description: EC2 instance type
    Default: t3.micro

Resources:
  SecurityGroup:
    Type: AWS::EC2::SecurityGroup
    Properties:
      GroupDescription: Security group with configured ports
      SecurityGroupIngress:
`

const cloudFormationTemplateFooter = `
      Tags:
        - Key: Name
          Value: !Sub "${AWS::StackName}-sg"

  EC2Instance:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref InstanceType
      ImageId: !Ref AmiId
      SecurityGroupIds:
        - !GetAtt SecurityGroup.GroupId
      UserData: %s
      Tags:
        - Key: Name
          Value: !Ref AWS::StackName

Outputs:
  InstanceId:
    Description: Instance ID
    Value: !Ref EC2Instance
  PublicIP:
    Description: Public IP Address
    Value: !GetAtt EC2Instance.PublicIp
  InstanceType:
    Description: Instance Type
    Value: !Ref InstanceType
  SecurityGroupId:
    Description: Security Group ID
    Value: !Ref SecurityGroup
`

const defaultCloudInitTemplate = `#!/bin/bash
set -e

GITHUB_USER="%s"

# Create user with sudo access
useradd -m -s /bin/bash $GITHUB_USER
echo "$GITHUB_USER ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/$GITHUB_USER

# Setup SSH directory
SSH_DIR="/home/$GITHUB_USER/.ssh"
AUTH_KEYS="$SSH_DIR/authorized_keys"

mkdir -p $SSH_DIR
chmod 700 $SSH_DIR

# Download public keys from GitHub
curl -s "https://github.com/$GITHUB_USER.keys" > $AUTH_KEYS

# Set correct permissions
chmod 600 $AUTH_KEYS
chown -R $GITHUB_USER:$GITHUB_USER $SSH_DIR

echo "User $GITHUB_USER created with SSH keys from GitHub"
`

func main() {
	createCmd := flag.Bool("create", false, "Create a new EC2 instance")
	createShort := flag.Bool("c", false, "Create a new EC2 instance (shorthand)")
	deleteCmd := flag.Bool("delete", false, "Delete an existing stack")
	deleteShort := flag.Bool("d", false, "Delete an existing stack (shorthand)")
	stackName := flag.String("name", "", "Stack name (required)")
	stackNameShort := flag.String("n", "", "Stack name (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -c -n mystack    Create stack using stacks/mystack.json\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -d -n mystack    Delete stack 'mystack'\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThe tool looks for stacks/<name>.json first, then treats name as a path.\n")
		fmt.Fprintf(os.Stderr, "\nConfig file format (stacks/mystack.json):\n")
		fmt.Fprintf(os.Stderr, `  {
    "github_username": "gherlein",
    "instance_type": "t3.micro",
    "hostname": "dev",
    "domain": "example.com",
    "ttl": 300
  }
`)
	}

	flag.Parse()

	doCreate := *createCmd || *createShort
	doDelete := *deleteCmd || *deleteShort

	name := *stackName
	if *stackNameShort != "" {
		name = *stackNameShort
	}

	if name == "" {
		log.Fatal("Stack name required (-n <name>)")
	}

	if !doCreate && !doDelete {
		flag.Usage()
		os.Exit(1)
	}

	if doCreate && doDelete {
		log.Fatal("Cannot specify both --create and --delete")
	}

	if doCreate {
		createStack(name)
	} else if doDelete {
		deleteStack(name)
	}
}

func resolveConfigPath(stackName string) string {
	// First, check if ./stacks/<stackName>.json exists
	stacksPath := fmt.Sprintf("stacks/%s.json", stackName)
	if _, err := os.Stat(stacksPath); err == nil {
		return stacksPath
	}

	// Otherwise, treat stackName as a path (with or without .json)
	if strings.HasSuffix(stackName, ".json") {
		return stackName
	}
	return fmt.Sprintf("%s.json", stackName)
}

func readConfig(stackName string) (*StackConfig, string, error) {
	filename := resolveConfigPath(stackName)
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, filename, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var cfg StackConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, filename, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.InstanceType == "" {
		cfg.InstanceType = "t3.micro"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 300
	}

	return &cfg, filename, nil
}

func writeConfig(filename string, cfg *StackConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(filename, data, 0644)
}

func resolveAmiId(ctx context.Context, ssmClient *ssm.Client, baseImage string) (string, error) {
	// If it starts with "ami-", it's already an AMI ID
	if strings.HasPrefix(baseImage, "ami-") {
		return baseImage, nil
	}

	// Otherwise, treat it as an SSM parameter path
	input := &ssm.GetParameterInput{
		Name: aws.String(baseImage),
	}

	result, err := ssmClient.GetParameter(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get SSM parameter %s: %w", baseImage, err)
	}

	return *result.Parameter.Value, nil
}

func generateCloudFormationTemplate(ports string, userDataScript string) string {
	// Parse ports, default to just SSH (22)
	portList := []string{"22"}
	if ports != "" {
		portList = []string{}
		for _, p := range strings.Split(ports, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				portList = append(portList, p)
			}
		}
	}

	// Generate security group ingress rules
	var ingressRules strings.Builder
	for _, port := range portList {
		ingressRules.WriteString(fmt.Sprintf(`        - IpProtocol: tcp
          FromPort: %s
          ToPort: %s
          CidrIp: 0.0.0.0/0
`, port, port))
	}

	// Embed UserData directly in template (base64 encoded)
	userDataBase64 := base64.StdEncoding.EncodeToString([]byte(userDataScript))
	footer := fmt.Sprintf(cloudFormationTemplateFooter, userDataBase64)

	return cloudFormationTemplateHeader + ingressRules.String() + footer
}

func getUserDataScript(cfg *StackConfig, configFile string) (string, error) {
	var script string

	if cfg.CloudInitScript != "" {
		// Look for the cloudinit script relative to the config file
		configDir := filepath.Dir(configFile)
		scriptPath := filepath.Join(configDir, cfg.CloudInitScript)

		// Also check stacks/ directory
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			scriptPath = filepath.Join("stacks", cfg.CloudInitScript)
		}

		data, err := os.ReadFile(scriptPath)
		if err != nil {
			return "", fmt.Errorf("failed to read cloudinit script %s: %w", cfg.CloudInitScript, err)
		}
		script = string(data)
		fmt.Printf("Using custom cloudinit script: %s\n", scriptPath)
	} else {
		// Use default template
		script = fmt.Sprintf(defaultCloudInitTemplate, cfg.GitHubUsername)
	}

	return script, nil
}

func lookupZoneID(ctx context.Context, r53Client *route53.Client, domain string) (string, error) {
	// Ensure domain ends with a dot for Route53
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(domain),
	}

	result, err := r53Client.ListHostedZonesByName(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to list hosted zones: %w", err)
	}

	for _, zone := range result.HostedZones {
		if *zone.Name == domain {
			// Zone ID format: /hostedzone/Z1234567890ABC
			zoneID := strings.TrimPrefix(*zone.Id, "/hostedzone/")
			return zoneID, nil
		}
	}

	return "", fmt.Errorf("hosted zone not found for domain: %s", domain)
}

func createDNSRecord(ctx context.Context, r53Client *route53.Client, zoneID, fqdn, ip string, ttl int) error {
	if !strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(fqdn),
						Type: r53types.RRTypeA,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(ip)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func deleteDNSRecord(ctx context.Context, r53Client *route53.Client, zoneID, fqdn, ip string, ttl int) error {
	if !strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(fqdn),
						Type: r53types.RRTypeA,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(ip)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func createStack(stackName string) {
	ctx := context.Background()

	// Read config
	stackCfg, configFile, err := readConfig(stackName)
	if err != nil {
		log.Fatalf("Error: %v\n\nCreate a config file stacks/%s.json with:\n%s", err, stackName, `{
  "github_username": "your-github-username",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}`)
	}

	if stackCfg.GitHubUsername == "" {
		log.Fatal("github_username is required in config file")
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", awsCfg.Region)
	fmt.Printf("Config File: %s\n", configFile)
	fmt.Printf("Stack Name: %s\n", stackName)
	fmt.Printf("GitHub Username: %s\n", stackCfg.GitHubUsername)
	fmt.Printf("Instance Type: %s\n", stackCfg.InstanceType)

	cfClient := cloudformation.NewFromConfig(awsCfg)
	r53Client := route53.NewFromConfig(awsCfg)
	ssmClient := ssm.NewFromConfig(awsCfg)

	// Resolve base image
	baseImage := stackCfg.BaseImage
	if baseImage == "" {
		baseImage = defaultBaseImage
	}
	amiId, err := resolveAmiId(ctx, ssmClient, baseImage)
	if err != nil {
		log.Fatalf("failed to resolve AMI: %v", err)
	}
	fmt.Printf("Base Image: %s\n", baseImage)
	fmt.Printf("AMI ID: %s\n", amiId)

	// Get UserData script (custom or default)
	userDataScript, err := getUserDataScript(stackCfg, configFile)
	if err != nil {
		log.Fatalf("failed to get user data script: %v", err)
	}

	// Lookup zone ID if domain is specified
	var zoneID string
	var fqdn string
	if stackCfg.Domain != "" && stackCfg.Hostname != "" {
		fmt.Printf("Looking up zone ID for %s...\n", stackCfg.Domain)
		zoneID, err = lookupZoneID(ctx, r53Client, stackCfg.Domain)
		if err != nil {
			log.Fatalf("failed to lookup zone ID: %v", err)
		}
		fmt.Printf("Found Zone ID: %s\n", zoneID)
		fqdn = fmt.Sprintf("%s.%s", stackCfg.Hostname, stackCfg.Domain)
	}

	// Generate CloudFormation template with configured ports and embedded UserData
	cfTemplate := generateCloudFormationTemplate(stackCfg.Ports, userDataScript)
	if stackCfg.Ports != "" {
		fmt.Printf("Ports: %s\n", stackCfg.Ports)
	} else {
		fmt.Printf("Ports: 22 (default)\n")
	}

	// Create CloudFormation stack
	input := &cloudformation.CreateStackInput{
		StackName:    &stackName,
		TemplateBody: aws.String(cfTemplate),
		Parameters: []types.Parameter{
			{
				ParameterKey:   aws.String("AmiId"),
				ParameterValue: aws.String(amiId),
			},
			{
				ParameterKey:   aws.String("GitHubUsername"),
				ParameterValue: aws.String(stackCfg.GitHubUsername),
			},
			{
				ParameterKey:   aws.String("InstanceType"),
				ParameterValue: aws.String(stackCfg.InstanceType),
			},
		},
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
		},
		Tags: []types.Tag{
			{
				Key:   aws.String("Purpose"),
				Value: aws.String("EC2Instance"),
			},
		},
	}

	result, err := cfClient.CreateStack(ctx, input)
	if err != nil {
		log.Fatalf("failed to create stack: %v", err)
	}

	fmt.Printf("Stack creation initiated!\n")
	fmt.Printf("Stack ID: %s\n", *result.StackId)
	fmt.Printf("Waiting for stack to complete...\n")

	waiter := cloudformation.NewStackCreateCompleteWaiter(cfClient)
	err = waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 10*time.Minute)
	if err != nil {
		log.Fatalf("failed waiting for stack: %v", err)
	}

	// Get stack outputs
	describeOutput, err := cfClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		log.Fatalf("failed to describe stack: %v", err)
	}

	// Update config with outputs
	stackCfg.StackName = stackName
	stackCfg.StackID = *result.StackId
	stackCfg.Region = awsCfg.Region

	for _, output := range describeOutput.Stacks[0].Outputs {
		switch *output.OutputKey {
		case "InstanceId":
			stackCfg.InstanceID = *output.OutputValue
		case "InstanceType":
			stackCfg.InstanceType = *output.OutputValue
		case "PublicIP":
			stackCfg.PublicIP = *output.OutputValue
		case "SecurityGroupId":
			stackCfg.SecurityGroup = *output.OutputValue
		}
	}

	// Create DNS record if configured
	if zoneID != "" && fqdn != "" {
		fmt.Printf("Creating DNS record: %s -> %s\n", fqdn, stackCfg.PublicIP)
		err = createDNSRecord(ctx, r53Client, zoneID, fqdn, stackCfg.PublicIP, stackCfg.TTL)
		if err != nil {
			log.Printf("Warning: failed to create DNS record: %v", err)
		} else {
			fmt.Println("DNS record created successfully")
			stackCfg.ZoneID = zoneID
			stackCfg.FQDN = fqdn
			stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.GitHubUsername, fqdn)
		}
	} else {
		stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.GitHubUsername, stackCfg.PublicIP)
	}

	// Write updated config
	if err := writeConfig(configFile, stackCfg); err != nil {
		log.Printf("Warning: failed to write config: %v", err)
	}

	fmt.Printf("\n=== Stack Created Successfully ===\n")
	jsonData, _ := json.MarshalIndent(stackCfg, "", "  ")
	fmt.Println(string(jsonData))
	fmt.Printf("\nConfig updated: %s\n", configFile)
	fmt.Printf("SSH: %s\n", stackCfg.SSHCommand)
}

func deleteStack(stackName string) {
	ctx := context.Background()

	// Try to read config for DNS cleanup
	stackCfg, configFile, err := readConfig(stackName)
	if err != nil {
		fmt.Printf("Warning: could not read config file: %v\n", err)
		stackCfg = nil
		configFile = ""
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", awsCfg.Region)
	fmt.Printf("Deleting Stack: %s\n", stackName)

	cfClient := cloudformation.NewFromConfig(awsCfg)

	// Delete DNS record if it was configured
	if stackCfg != nil && stackCfg.ZoneID != "" && stackCfg.FQDN != "" && stackCfg.PublicIP != "" {
		fmt.Printf("Deleting DNS record: %s\n", stackCfg.FQDN)
		r53Client := route53.NewFromConfig(awsCfg)
		err = deleteDNSRecord(ctx, r53Client, stackCfg.ZoneID, stackCfg.FQDN, stackCfg.PublicIP, stackCfg.TTL)
		if err != nil {
			log.Printf("Warning: failed to delete DNS record: %v", err)
		} else {
			fmt.Println("DNS record deleted")
		}
	}

	// Delete CloudFormation stack
	_, err = cfClient.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: &stackName,
	})
	if err != nil {
		log.Fatalf("failed to delete stack: %v", err)
	}

	fmt.Println("Stack deletion initiated, waiting for completion...")

	waiter := cloudformation.NewStackDeleteCompleteWaiter(cfClient)
	err = waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 10*time.Minute)
	if err != nil {
		log.Fatalf("failed waiting for stack deletion: %v", err)
	}

	// Clear output fields in config file
	if stackCfg != nil && configFile != "" {
		stackCfg.StackName = ""
		stackCfg.StackID = ""
		stackCfg.Region = ""
		stackCfg.InstanceID = ""
		stackCfg.PublicIP = ""
		stackCfg.SecurityGroup = ""
		stackCfg.ZoneID = ""
		stackCfg.FQDN = ""
		stackCfg.SSHCommand = ""
		if err := writeConfig(configFile, stackCfg); err != nil {
			log.Printf("Warning: failed to update config file: %v", err)
		} else {
			fmt.Printf("Config cleared: %s\n", configFile)
		}
	}

	fmt.Println("Stack deleted successfully")
}
