# Implementation Summary: Multi-User & DNS Enhancements

## Overview

This document provides a comprehensive implementation guide for two major enhancements to the EC2 instance manager:

1. **Multi-User Support**: Create multiple users on each instance with different local and GitHub usernames
2. **DNS Enhancements**: Support for CNAME aliases and apex domain records

## Implementation Order

Implement in this order to ensure proper dependency handling:

1. Multi-User Support (modifies core user creation)
2. DNS Enhancements (builds on existing DNS infrastructure)
3. Integration Testing (validates both features working together)

## Phase 1: Multi-User Support

### Files to Modify

#### main.go

**1. Add User struct** (after line 19):
```go
type User struct {
	Username       string `json:"username"`
	GitHubUsername string `json:"github_username"`
}
```

**2. Update StackConfig struct** (line 21-39):
```go
type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername string   `json:"github_username,omitempty"` // DEPRECATED but supported
	Users          []User   `json:"users,omitempty"`           // NEW
	InstanceType   string   `json:"instance_type,omitempty"`
	Hostname       string   `json:"hostname,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	TTL            int      `json:"ttl,omitempty"`

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
```

**3. Update CloudFormation template** (line 41-121):
```go
const cloudFormationTemplate = `
AWSTemplateFormatVersion: '2010-09-09'
Description: EC2 instance with SSH access

Parameters:
  LatestAmiId:
    Type: AWS::SSM::Parameter::Value<AWS::EC2::Image::Id>
    Default: /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64
  Users:
    Type: String
    Description: Comma-separated list of username:github_username pairs
  InstanceType:
    Type: String
    Description: EC2 instance type
    Default: t3.micro

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
      ImageId: !Ref LatestAmiId
      SecurityGroupIds:
        - !GetAtt SSHSecurityGroup.GroupId
      UserData:
        Fn::Base64: !Sub |
          #!/bin/bash
          set -e

          # Users parameter format: username1:github1,username2:github2
          USERS="${Users}"

          # Split by comma and process each user
          IFS=',' read -ra USER_ARRAY <<< "$USERS"
          for user_spec in "$${USER_ARRAY[@]}"; do
            # Split by colon to get username and github_username
            IFS=':' read -r USERNAME GITHUB_USER <<< "$user_spec"

            echo "Creating user: $USERNAME (GitHub: $GITHUB_USER)"

            # Create user with sudo access
            useradd -m -s /bin/bash "$USERNAME"
            echo "$USERNAME ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/$USERNAME"

            # Setup SSH directory
            SSH_DIR="/home/$USERNAME/.ssh"
            AUTH_KEYS="$SSH_DIR/authorized_keys"

            mkdir -p "$SSH_DIR"
            chmod 700 "$SSH_DIR"

            # Download public keys from GitHub
            curl -s "https://github.com/$GITHUB_USER.keys" > "$AUTH_KEYS"

            # Set correct permissions
            chmod 600 "$AUTH_KEYS"
            chown -R "$USERNAME:$USERNAME" "$SSH_DIR"

            echo "User $USERNAME created with SSH keys from GitHub ($GITHUB_USER)"
          done
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
```

**4. Add validation functions** (before createStack function):
```go
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

		// Validate username format (basic check)
		if !isValidLinuxUsername(user.Username) {
			return fmt.Errorf("invalid username format: %s (must be lowercase alphanumeric, start with letter)", user.Username)
		}
	}

	return nil
}

func isValidLinuxUsername(username string) bool {
	// Basic validation: alphanumeric plus underscore/hyphen, start with letter
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

func encodeUsers(users []User) string {
	var parts []string
	for _, user := range users {
		parts = append(parts, fmt.Sprintf("%s:%s", user.Username, user.GitHubUsername))
	}
	return strings.Join(parts, ",")
}
```

**5. Update createStack function** (line 307):

Add validation after reading config:
```go
// Validate user configuration (also normalizes legacy github_username)
if err := validateUserConfig(stackCfg); err != nil {
	log.Fatalf("Invalid user configuration: %v", err)
}
```

Update output display:
```go
fmt.Printf("Users to create: %d\n", len(stackCfg.Users))
for _, user := range stackCfg.Users {
	fmt.Printf("  - %s (GitHub: %s)\n", user.Username, user.GitHubUsername)
}
```

Update CloudFormation parameters:
```go
usersParam := encodeUsers(stackCfg.Users)

input := &cloudformation.CreateStackInput{
	StackName:    &stackName,
	TemplateBody: aws.String(cloudFormationTemplate),
	Parameters: []types.Parameter{
		{
			ParameterKey:   aws.String("Users"),
			ParameterValue: aws.String(usersParam),
		},
		{
			ParameterKey:   aws.String("InstanceType"),
			ParameterValue: aws.String(stackCfg.InstanceType),
		},
	},
	// ... rest of input
}
```

Update SSH command generation:
```go
if stackCfg.FQDN != "" {
	stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.FQDN)
} else {
	stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.PublicIP)
}
```

## Phase 2: DNS Enhancements

### Files to Modify

#### main.go

**1. Add DNSRecord struct** (after User struct):
```go
type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`   // "A" or "CNAME"
	Value string `json:"value"`  // IP address for A, FQDN for CNAME
	TTL   int    `json:"ttl"`
}
```

**2. Update StackConfig struct** (add to existing):
```go
type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername string   `json:"github_username,omitempty"`
	Users          []User   `json:"users,omitempty"`
	InstanceType   string   `json:"instance_type,omitempty"`
	Hostname       string   `json:"hostname,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	TTL            int      `json:"ttl,omitempty"`
	IsApexDomain   bool     `json:"is_apex_domain,omitempty"`      // NEW
	CNAMEAliases   []string `json:"cname_aliases,omitempty"`       // NEW

	// Output fields (program fills in)
	StackName     string      `json:"stack_name,omitempty"`
	StackID       string      `json:"stack_id,omitempty"`
	Region        string      `json:"region,omitempty"`
	InstanceID    string      `json:"instance_id,omitempty"`
	PublicIP      string      `json:"public_ip,omitempty"`
	SecurityGroup string      `json:"security_group,omitempty"`
	ZoneID        string      `json:"zone_id,omitempty"`
	FQDN          string      `json:"fqdn,omitempty"`
	SSHCommand    string      `json:"ssh_command,omitempty"`
	DNSRecords    []DNSRecord `json:"dns_records,omitempty"`        // NEW
}
```

**3. Add DNS validation** (before createStack):
```go
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
```

**4. Replace createDNSRecord with new functions**:
```go
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

func createDNSRecords(ctx context.Context, r53Client *route53.Client, cfg *StackConfig) ([]DNSRecord, error) {
	var createdRecords []DNSRecord

	if cfg.Domain == "" {
		return createdRecords, nil // No DNS configuration
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
				// Cleanup on failure
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
			// Cleanup on failure
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

func deleteCreatedRecords(ctx context.Context, r53Client *route53.Client, zoneID string, records []DNSRecord) {
	for _, record := range records {
		if record.Type == "A" {
			deleteARecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		} else if record.Type == "CNAME" {
			deleteCNAMERecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		}
	}
}
```

**5. Update createStack function**:

Add DNS validation:
```go
// Validate DNS configuration
if err := validateDNSConfig(stackCfg); err != nil {
	log.Fatalf("Invalid DNS configuration: %v", err)
}
```

Replace DNS record creation:
```go
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
```

**6. Update deleteStack function**:

Replace DNS record deletion:
```go
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
```

## Testing Strategy

### Phase 1: Multi-User Testing

1. **Test legacy format**:
   ```bash
   cp example-legacy.json stacks/test1.json
   ./bin/ec2 -c -n test1
   ```
   Verify single user created with old format.

2. **Test single user new format**:
   ```bash
   cp example-single-user.json stacks/test2.json
   ./bin/ec2 -c -n test2
   ```
   Verify single user created with new format.

3. **Test multiple users**:
   ```bash
   cp example-no-dns.json stacks/test3.json
   ./bin/ec2 -c -n test3
   ```
   Verify multiple users created, each can SSH.

4. **Test validation errors**:
   - Empty username
   - Duplicate usernames
   - Invalid username format
   - No users specified

### Phase 2: DNS Testing

1. **Test CNAME only**:
   ```bash
   cp example.json stacks/test4.json
   # Edit to remove is_apex_domain
   ./bin/ec2 -c -n test4
   ```
   Verify A record and CNAMEs created.

2. **Test apex domain**:
   ```bash
   cp example-apex-domain.json stacks/test5.json
   ./bin/ec2 -c -n test5
   ```
   Verify apex A record created.

3. **Test full featured**:
   ```bash
   cp example-full-featured.json stacks/test6.json
   ./bin/ec2 -c -n test6
   ```
   Verify all DNS records and all users created.

4. **Test cleanup**:
   ```bash
   ./bin/ec2 -d -n test6
   ```
   Verify all DNS records deleted.

### Integration Testing

Test combinations:
- Multiple users + CNAMEs
- Multiple users + apex domain
- Multiple users + CNAMEs + apex domain
- Verify SSH works for all users via all DNS names

## Build and Deployment

1. **Build the binary**:
   ```bash
   make build
   ```

2. **Test locally**:
   ```bash
   ./bin/ec2 -c -n test-stack
   ```

3. **Verify deployment**:
   - Check CloudFormation stack status
   - Check Route53 records
   - Test SSH access
   - Check cloud-init logs on instance

4. **Clean up**:
   ```bash
   ./bin/ec2 -d -n test-stack
   ```

## Rollback Plan

If issues occur during implementation:

1. **Revert to git commit before changes**:
   ```bash
   git checkout <previous-commit>
   ```

2. **Keep existing stack configs compatible**:
   - All changes are backward compatible
   - Existing configs continue to work

3. **Manual cleanup if needed**:
   - Delete CloudFormation stacks via AWS console
   - Delete DNS records via Route53 console

## Documentation Updates

Update these files:
- [x] example.json - Updated with all new fields
- [ ] README.md - Add multi-user and DNS enhancement sections
- [x] EXAMPLES.md - Created with comprehensive examples
- [x] DNS_ENHANCEMENT_PLAN.md - Detailed DNS implementation plan
- [x] MULTI_USER_PLAN.md - Detailed multi-user implementation plan
- [x] IMPLEMENTATION_SUMMARY.md - This file

## Success Criteria

Implementation is complete when:
- [ ] All multi-user validation tests pass
- [ ] All DNS enhancement tests pass
- [ ] Integration tests pass
- [ ] Backward compatibility verified
- [ ] Documentation updated
- [ ] Example files created and tested
- [ ] README updated with new features
- [ ] No regression in existing functionality

## Estimated Implementation Time

- Multi-User Support: 2-3 hours
- DNS Enhancements: 2-3 hours
- Testing: 2 hours
- Documentation: 1 hour
- **Total: 7-9 hours**

## Future Enhancements

Consider for future versions:
1. Per-user sudo configuration
2. Per-user groups
3. Additional record types (MX, TXT, SRV)
4. IPv6 support (AAAA records)
5. Health check integration
6. Multiple SSH key sources
7. User management commands (add/remove users without recreating stack)
