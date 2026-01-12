package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type User struct {
	Username       string `json:"username"`
	GitHubUsername string `json:"github_username"`
}

type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername string   `json:"github_username,omitempty"`
	Users          []User   `json:"users,omitempty"`
	InstanceType   string   `json:"instance_type,omitempty"`
	OS             string   `json:"os,omitempty"`
	CloudInitFile  string   `json:"cloud_init_file,omitempty"`
	Hostname       string   `json:"hostname,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	TTL            int      `json:"ttl,omitempty"`
	IsApexDomain   bool     `json:"is_apex_domain,omitempty"`
	CNAMEAliases   []string `json:"cname_aliases,omitempty"`

	// Output fields (program fills in)
	StackName     string      `json:"stack_name,omitempty"`
	StackID       string      `json:"stack_id,omitempty"`
	Region        string      `json:"region,omitempty"`
	AMIID         string      `json:"ami_id,omitempty"`
	InstanceID    string      `json:"instance_id,omitempty"`
	PublicIP      string      `json:"public_ip,omitempty"`
	SecurityGroup string      `json:"security_group,omitempty"`
	ZoneID        string      `json:"zone_id,omitempty"`
	FQDN          string      `json:"fqdn,omitempty"`
	SSHCommand    string      `json:"ssh_command,omitempty"`
	DNSRecords    []DNSRecord `json:"dns_records,omitempty"`
}

var osSSMPaths = map[string]string{
	"amazon-linux-2023": "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64",
	"amazon-linux-2":    "/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2",
	"ubuntu-24.04":      "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp2/ami-id",
	"ubuntu-22.04":      "/aws/service/canonical/ubuntu/server/22.04/stable/current/amd64/hvm/ebs-gp2/ami-id",
	"ubuntu-20.04":      "/aws/service/canonical/ubuntu/server/20.04/stable/current/amd64/hvm/ebs-gp2/ami-id",
	"debian-12":         "/aws/service/debian/release/12/latest/amd64",
	"debian-11":         "/aws/service/debian/release/11/latest/amd64",
}

const cloudFormationTemplate = `
AWSTemplateFormatVersion: '2010-09-09'
Description: EC2 instance with SSH access

Parameters:
  ImageId:
    Type: String
    Description: AMI ID for the EC2 instance
  InstanceType:
    Type: String
    Description: EC2 instance type
    Default: t3.micro
  UserData:
    Type: String
    Description: Base64 encoded UserData script

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
      InstanceType: !Ref InstanceType
      ImageId: !Ref ImageId
      SecurityGroupIds:
        - !GetAtt SSHSecurityGroup.GroupId
      UserData: !Ref UserData
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
    Value: !Ref SSHSecurityGroup
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
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "cloud_init_file": "cloud-init/setup.yaml",
    "users": [{"username": "admin", "github_username": "gherlein"}],
    "instance_type": "t3.micro",
    "hostname": "dev",
    "domain": "example.com",
    "ttl": 300
  }
`)
		fmt.Fprintf(os.Stderr, "\nSupported OS values:\n")
		fmt.Fprintf(os.Stderr, "  amazon-linux-2023, amazon-linux-2, ubuntu-24.04, ubuntu-22.04,\n")
		fmt.Fprintf(os.Stderr, "  ubuntu-20.04, debian-12, debian-11\n")
	}

	flag.Parse()

	doCreate := *createCmd || *createShort
	doDelete := *deleteCmd || *deleteShort

	name := *stackName
	if *stackNameShort != "" {
		name = *stackNameShort
	}

	// If no -n flag, check for positional argument (config file path)
	if name == "" && flag.NArg() > 0 {
		configPath := flag.Arg(0)
		// Extract stack name from filename (remove path and .json extension)
		name = strings.TrimSuffix(configPath, ".json")
		if lastSlash := strings.LastIndex(name, "/"); lastSlash >= 0 {
			name = name[lastSlash+1:]
		}
	}

	if name == "" {
		log.Fatal("Stack name required: use -n <name> or provide a config file path")
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
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.OS == "" {
		cfg.OS = "amazon-linux-2023"
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

func lookupAMI(ctx context.Context, ssmClient *ssm.Client, osName string) (string, error) {
	ssmPath, ok := osSSMPaths[osName]
	if !ok {
		var supported []string
		for k := range osSSMPaths {
			supported = append(supported, k)
		}
		return "", fmt.Errorf("unsupported OS %q, supported: %v", osName, supported)
	}

	result, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(ssmPath),
	})
	if err != nil {
		return "", fmt.Errorf("failed to lookup AMI for %s: %w", osName, err)
	}

	return *result.Parameter.Value, nil
}

func generateUserSetupScript(users []User) string {
	var script strings.Builder
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")
	script.WriteString("# Auto-generated user setup script\n")

	for _, user := range users {
		script.WriteString(fmt.Sprintf("\n# Create user: %s (GitHub: %s)\n", user.Username, user.GitHubUsername))
		script.WriteString(fmt.Sprintf("useradd -m -s /bin/bash %q || true\n", user.Username))
		script.WriteString(fmt.Sprintf("echo %q ' ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/%s\n", user.Username, user.Username))
		script.WriteString(fmt.Sprintf("mkdir -p /home/%s/.ssh\n", user.Username))
		script.WriteString(fmt.Sprintf("chmod 700 /home/%s/.ssh\n", user.Username))
		script.WriteString(fmt.Sprintf("curl -s https://github.com/%s.keys > /home/%s/.ssh/authorized_keys\n", user.GitHubUsername, user.Username))
		script.WriteString(fmt.Sprintf("chmod 600 /home/%s/.ssh/authorized_keys\n", user.Username))
		script.WriteString(fmt.Sprintf("chown -R %s:%s /home/%s/.ssh\n", user.Username, user.Username, user.Username))
		script.WriteString(fmt.Sprintf("echo 'User %s created with SSH keys from GitHub (%s)'\n", user.Username, user.GitHubUsername))
	}

	return script.String()
}

type CloudInitTemplateData struct {
	Hostname string
	Domain   string
	FQDN     string
	Region   string
	OS       string
	Users    []User
}

func processCloudInitTemplate(templatePath string, data CloudInitTemplateData) (string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read cloud-init file: %w", err)
	}

	tmpl, err := template.New("cloud-init").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse cloud-init template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute cloud-init template: %w", err)
	}

	return buf.String(), nil
}

func generateMultipartUserData(userScript string, cloudInitContent string) string {
	boundary := "MIMEBOUNDARY"
	var buf bytes.Buffer

	buf.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\n")
	buf.WriteString("MIME-Version: 1.0\n\n")

	// Part 1: Shell script for user setup
	buf.WriteString("--" + boundary + "\n")
	buf.WriteString("Content-Type: text/x-shellscript; charset=\"utf-8\"\n")
	buf.WriteString("Content-Disposition: attachment; filename=\"setup-users.sh\"\n\n")
	buf.WriteString(userScript)
	buf.WriteString("\n")

	// Part 2: Cloud-init config (if provided)
	if cloudInitContent != "" {
		buf.WriteString("--" + boundary + "\n")
		buf.WriteString("Content-Type: text/cloud-config; charset=\"utf-8\"\n")
		buf.WriteString("Content-Disposition: attachment; filename=\"cloud-config.yaml\"\n\n")
		buf.WriteString(cloudInitContent)
		buf.WriteString("\n")
	}

	buf.WriteString("--" + boundary + "--\n")

	return base64.StdEncoding.EncodeToString(buf.Bytes())
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

func validateUserConfig(cfg *StackConfig) error {
	// Normalize: convert legacy github_username to users array
	if len(cfg.Users) == 0 && cfg.GitHubUsername != "" {
		cfg.Users = []User{
			{
				Username:       cfg.GitHubUsername,
				GitHubUsername: cfg.GitHubUsername,
			},
		}
	}

	// Require at least one user
	if len(cfg.Users) == 0 {
		return fmt.Errorf("at least one user required: specify 'github_username' or 'users'")
	}

	// Validate each user
	seen := make(map[string]bool)
	for i, user := range cfg.Users {
		if user.Username == "" {
			return fmt.Errorf("user[%d]: username cannot be empty", i)
		}
		if user.GitHubUsername == "" {
			return fmt.Errorf("user[%d]: github_username cannot be empty", i)
		}

		// Check for duplicate usernames
		if seen[user.Username] {
			return fmt.Errorf("duplicate username: %s", user.Username)
		}
		seen[user.Username] = true

		// Validate username format
		if !isValidLinuxUsername(user.Username) {
			return fmt.Errorf("invalid username format: %s (must be lowercase alphanumeric, start with letter)", user.Username)
		}
	}

	return nil
}

func isValidLinuxUsername(username string) bool {
	if len(username) == 0 || len(username) > 32 {
		return false
	}

	// Must start with lowercase letter
	if username[0] < 'a' || username[0] > 'z' {
		return false
	}

	// Rest can be alphanumeric, underscore, or hyphen
	for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return false
		}
	}

	return true
}

func validateDNSConfig(cfg *StackConfig) error {
	// Validate CNAME aliases
	if len(cfg.CNAMEAliases) > 0 {
		if cfg.Hostname == "" || cfg.Domain == "" {
			return fmt.Errorf("cname_aliases requires both hostname and domain")
		}

		// Check for duplicates and empty strings
		seen := make(map[string]bool)
		for _, alias := range cfg.CNAMEAliases {
			if alias == "" {
				return fmt.Errorf("cname_aliases cannot contain empty strings")
			}
			if alias == cfg.Hostname {
				return fmt.Errorf("cname_aliases cannot duplicate primary hostname: %s", alias)
			}
			if seen[alias] {
				return fmt.Errorf("duplicate cname_alias: %s", alias)
			}
			seen[alias] = true
		}
	}

	// Validate apex domain
	if cfg.IsApexDomain && cfg.Domain == "" {
		return fmt.Errorf("is_apex_domain requires domain to be specified")
	}

	return nil
}

func encodeUsers(users []User) string {
	var parts []string
	for _, user := range users {
		parts = append(parts, fmt.Sprintf("%s:%s", user.Username, user.GitHubUsername))
	}
	return strings.Join(parts, ",")
}

func createARecord(ctx context.Context, r53Client *route53.Client, zoneID, name, ip string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
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

func createCNAMERecord(ctx context.Context, r53Client *route53.Client, zoneID, name, target string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	if !strings.HasSuffix(target, ".") {
		target = target + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
						Type: r53types.RRTypeCname,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(target)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func deleteARecord(ctx context.Context, r53Client *route53.Client, zoneID, name, ip string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
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

func deleteCNAMERecord(ctx context.Context, r53Client *route53.Client, zoneID, name, target string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	if !strings.HasSuffix(target, ".") {
		target = target + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionDelete,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
						Type: r53types.RRTypeCname,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(target)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func deleteCreatedRecords(ctx context.Context, r53Client *route53.Client, zoneID string, records []DNSRecord) {
	for _, record := range records {
		if record.Type == "A" {
			deleteARecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		} else if record.Type == "CNAME" {
			deleteCNAMERecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		}
	}
}

func createDNSRecords(ctx context.Context, r53Client *route53.Client, cfg *StackConfig) ([]DNSRecord, error) {
	var createdRecords []DNSRecord

	if cfg.Domain == "" {
		return createdRecords, nil
	}

	// 1. Create primary A record (hostname.domain -> IP)
	if cfg.Hostname != "" {
		fqdn := fmt.Sprintf("%s.%s", cfg.Hostname, cfg.Domain)
		err := createARecord(ctx, r53Client, cfg.ZoneID, fqdn, cfg.PublicIP, cfg.TTL)
		if err != nil {
			return createdRecords, fmt.Errorf("failed to create primary A record: %w", err)
		}
		createdRecords = append(createdRecords, DNSRecord{
			Name:  fqdn,
			Type:  "A",
			Value: cfg.PublicIP,
			TTL:   cfg.TTL,
		})
	}

	// 2. Create CNAME records (alias.domain -> hostname.domain)
	if cfg.Hostname != "" && len(cfg.CNAMEAliases) > 0 {
		targetFQDN := fmt.Sprintf("%s.%s", cfg.Hostname, cfg.Domain)
		for _, alias := range cfg.CNAMEAliases {
			aliasFQDN := fmt.Sprintf("%s.%s", alias, cfg.Domain)
			err := createCNAMERecord(ctx, r53Client, cfg.ZoneID, aliasFQDN, targetFQDN, cfg.TTL)
			if err != nil {
				deleteCreatedRecords(ctx, r53Client, cfg.ZoneID, createdRecords)
				return nil, fmt.Errorf("failed to create CNAME %s: %w", aliasFQDN, err)
			}
			createdRecords = append(createdRecords, DNSRecord{
				Name:  aliasFQDN,
				Type:  "CNAME",
				Value: targetFQDN,
				TTL:   cfg.TTL,
			})
		}
	}

	// 3. Create apex A record (domain -> IP)
	if cfg.IsApexDomain {
		err := createARecord(ctx, r53Client, cfg.ZoneID, cfg.Domain, cfg.PublicIP, cfg.TTL)
		if err != nil {
			deleteCreatedRecords(ctx, r53Client, cfg.ZoneID, createdRecords)
			return nil, fmt.Errorf("failed to create apex A record: %w", err)
		}
		createdRecords = append(createdRecords, DNSRecord{
			Name:  cfg.Domain,
			Type:  "A",
			Value: cfg.PublicIP,
			TTL:   cfg.TTL,
		})
	}

	return createdRecords, nil
}

func createStack(stackName string) {
	ctx := context.Background()

	// Read config
	stackCfg, configFile, err := readConfig(stackName)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Validate user configuration
	if err := validateUserConfig(stackCfg); err != nil {
		log.Fatalf("Invalid user configuration: %v", err)
	}

	// Validate DNS configuration
	if err := validateDNSConfig(stackCfg); err != nil {
		log.Fatalf("Invalid DNS configuration: %v", err)
	}

	// Load AWS config with region from JSON config
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(stackCfg.Region))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", stackCfg.Region)
	fmt.Printf("Config File: %s\n", configFile)
	fmt.Printf("Stack Name: %s\n", stackName)
	fmt.Printf("OS: %s\n", stackCfg.OS)
	fmt.Printf("Users to create: %d\n", len(stackCfg.Users))
	for _, user := range stackCfg.Users {
		fmt.Printf("  - %s (GitHub: %s)\n", user.Username, user.GitHubUsername)
	}
	fmt.Printf("Instance Type: %s\n", stackCfg.InstanceType)

	cfClient := cloudformation.NewFromConfig(awsCfg)
	r53Client := route53.NewFromConfig(awsCfg)
	ssmClient := ssm.NewFromConfig(awsCfg)

	// Lookup AMI ID from SSM
	fmt.Printf("Looking up AMI for %s...\n", stackCfg.OS)
	amiID, err := lookupAMI(ctx, ssmClient, stackCfg.OS)
	if err != nil {
		log.Fatalf("failed to lookup AMI: %v", err)
	}
	fmt.Printf("Found AMI: %s\n", amiID)
	stackCfg.AMIID = amiID

	// Lookup zone ID if domain is specified
	var zoneID string
	if stackCfg.Domain != "" {
		fmt.Printf("Looking up zone ID for %s...\n", stackCfg.Domain)
		zoneID, err = lookupZoneID(ctx, r53Client, stackCfg.Domain)
		if err != nil {
			log.Fatalf("failed to lookup zone ID: %v", err)
		}
		fmt.Printf("Found Zone ID: %s\n", zoneID)
	}

	// Generate UserData
	userScript := generateUserSetupScript(stackCfg.Users)

	var cloudInitContent string
	if stackCfg.CloudInitFile != "" {
		// Resolve path relative to config file
		cloudInitPath := stackCfg.CloudInitFile
		if !filepath.IsAbs(cloudInitPath) {
			configDir := filepath.Dir(configFile)
			cloudInitPath = filepath.Join(configDir, cloudInitPath)
		}

		fmt.Printf("Processing cloud-init file: %s\n", cloudInitPath)

		// Calculate FQDN for template
		fqdn := ""
		if stackCfg.Hostname != "" && stackCfg.Domain != "" {
			fqdn = fmt.Sprintf("%s.%s", stackCfg.Hostname, stackCfg.Domain)
		}

		templateData := CloudInitTemplateData{
			Hostname: stackCfg.Hostname,
			Domain:   stackCfg.Domain,
			FQDN:     fqdn,
			Region:   stackCfg.Region,
			OS:       stackCfg.OS,
			Users:    stackCfg.Users,
		}

		cloudInitContent, err = processCloudInitTemplate(cloudInitPath, templateData)
		if err != nil {
			log.Fatalf("failed to process cloud-init: %v", err)
		}
	}

	userData := generateMultipartUserData(userScript, cloudInitContent)

	// Create CloudFormation stack
	input := &cloudformation.CreateStackInput{
		StackName:    &stackName,
		TemplateBody: aws.String(cloudFormationTemplate),
		Parameters: []types.Parameter{
			{
				ParameterKey:   aws.String("ImageId"),
				ParameterValue: aws.String(amiID),
			},
			{
				ParameterKey:   aws.String("InstanceType"),
				ParameterValue: aws.String(stackCfg.InstanceType),
			},
			{
				ParameterKey:   aws.String("UserData"),
				ParameterValue: aws.String(userData),
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

	// Create DNS records if configured
	if zoneID != "" {
		fmt.Println("Creating DNS records...")
		stackCfg.ZoneID = zoneID
		dnsRecords, err := createDNSRecords(ctx, r53Client, stackCfg)
		if err != nil {
			log.Printf("Warning: failed to create DNS records: %v", err)
		} else {
			fmt.Printf("Created %d DNS record(s) successfully\n", len(dnsRecords))
			stackCfg.DNSRecords = dnsRecords

			// Set FQDN to primary hostname or apex if no hostname
			if stackCfg.Hostname != "" {
				stackCfg.FQDN = fmt.Sprintf("%s.%s", stackCfg.Hostname, stackCfg.Domain)
			} else if stackCfg.IsApexDomain {
				stackCfg.FQDN = stackCfg.Domain
			}
		}
	}

	// Set SSH command
	if stackCfg.FQDN != "" {
		stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.FQDN)
	} else {
		stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.PublicIP)
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

	// Determine region (from config or default)
	region := "us-east-1"
	if stackCfg != nil && stackCfg.Region != "" {
		region = stackCfg.Region
	}

	// Load AWS config with region from JSON config
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", region)
	fmt.Printf("Deleting Stack: %s\n", stackName)

	cfClient := cloudformation.NewFromConfig(awsCfg)

	// Delete DNS records if they were configured
	if stackCfg != nil && stackCfg.ZoneID != "" && len(stackCfg.DNSRecords) > 0 {
		fmt.Printf("Deleting %d DNS record(s)...\n", len(stackCfg.DNSRecords))
		r53Client := route53.NewFromConfig(awsCfg)

		for _, record := range stackCfg.DNSRecords {
			fmt.Printf("  Deleting %s record: %s -> %s\n", record.Type, record.Name, record.Value)

			var err error
			if record.Type == "A" {
				err = deleteARecord(ctx, r53Client, stackCfg.ZoneID, record.Name, record.Value, record.TTL)
			} else if record.Type == "CNAME" {
				err = deleteCNAMERecord(ctx, r53Client, stackCfg.ZoneID, record.Name, record.Value, record.TTL)
			}

			if err != nil {
				log.Printf("Warning: failed to delete DNS record %s: %v", record.Name, err)
			}
		}
		fmt.Println("DNS records deleted")
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
		stackCfg.DNSRecords = []DNSRecord{}
		if err := writeConfig(configFile, stackCfg); err != nil {
			log.Printf("Warning: failed to update config file: %v", err)
		} else {
			fmt.Printf("Config cleared: %s\n", configFile)
		}
	}

	fmt.Println("Stack deleted successfully")
}
