# Multi-User Support Plan

## Overview

Enhance the EC2 instance manager to support creating multiple users on the host, each with their own username and GitHub account for SSH key provisioning.

## Requirements Analysis

### Current Behavior

Currently, the tool creates a single user:
- Username matches the GitHub username
- SSH keys pulled from `https://github.com/{github_username}.keys`
- User gets passwordless sudo access

### New Behavior

Support creating multiple users:
- Each user has a local username (can differ from GitHub username)
- Each user's SSH keys pulled from their respective GitHub account
- All users get passwordless sudo access
- Backward compatible with single-user configuration

### Use Cases

**Use Case 1: Multiple Team Members**
```json
{
  "users": [
    {"username": "greg", "github_username": "gherlein"},
    {"username": "alice", "github_username": "alice-dev"},
    {"username": "bob", "github_username": "bob-admin"}
  ]
}
```

**Use Case 2: Same User, Different Name**
```json
{
  "users": [
    {"username": "admin", "github_username": "gherlein"}
  ]
}
```

**Use Case 3: Backward Compatibility (Legacy)**
```json
{
  "github_username": "gherlein"
}
```
Creates single user named `gherlein` (existing behavior).

## Configuration Schema Design

### New Structure

```json
{
  "users": [
    {
      "username": "greg",
      "github_username": "gherlein"
    },
    {
      "username": "alice",
      "github_username": "alice-dev"
    }
  ]
}
```

### Backward Compatibility

**Legacy field**: `github_username` (string)
**New field**: `users` (array of user objects)

**Resolution rules**:
1. If `users` is present and non-empty: Use `users` array (ignore `github_username`)
2. If `users` is empty/missing and `github_username` is present: Create single user
3. If both are missing: Error (at least one user required)

### User Object Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | Linux username to create on the host |
| `github_username` | string | Yes | GitHub username to fetch SSH keys from |

### Validation Rules

1. **At least one user required**: Either `github_username` OR `users` must be specified
2. **Unique usernames**: No duplicate usernames in the `users` array
3. **Valid username format**: Must match Linux username requirements (alphanumeric, underscore, hyphen; start with letter)
4. **Non-empty values**: Both `username` and `github_username` must be non-empty strings

## Implementation Design

### Data Structure Changes

**Go struct updates**:

```go
type User struct {
	Username       string `json:"username"`
	GitHubUsername string `json:"github_username"`
}

type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername string   `json:"github_username,omitempty"` // DEPRECATED but supported
	Users          []User   `json:"users,omitempty"`           // NEW
	InstanceType   string   `json:"instance_type,omitempty"`
	Hostname       string   `json:"hostname,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	TTL            int      `json:"ttl,omitempty"`
	IsApexDomain   bool     `json:"is_apex_domain,omitempty"`
	CNAMEAliases   []string `json:"cname_aliases,omitempty"`

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
	DNSRecords    []DNSRecord `json:"dns_records,omitempty"`
}
```

### Validation Function

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
			return fmt.Errorf("invalid username format: %s (must be alphanumeric, start with letter)", user.Username)
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
```

### CloudFormation Template Changes

Update the CloudFormation template to accept a list of users and create them all:

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

### User String Encoding

Convert the `Users` array to a comma-separated string for CloudFormation:

```go
func encodeUsers(users []User) string {
	var parts []string
	for _, user := range users {
		parts = append(parts, fmt.Sprintf("%s:%s", user.Username, user.GitHubUsername))
	}
	return strings.Join(parts, ",")
}
```

### Update createStack Function

Modify the `createStack()` function to validate and encode users:

```go
func createStack(stackName string) {
	ctx := context.Background()

	// Read config
	stackCfg, configFile, err := readConfig(stackName)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Validate user configuration (also normalizes legacy github_username)
	if err := validateUserConfig(stackCfg); err != nil {
		log.Fatalf("Invalid user configuration: %v", err)
	}

	// Validate DNS configuration
	if err := validateDNSConfig(stackCfg); err != nil {
		log.Fatalf("Invalid DNS configuration: %v", err)
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", awsCfg.Region)
	fmt.Printf("Config File: %s\n", configFile)
	fmt.Printf("Stack Name: %s\n", stackName)
	fmt.Printf("Users to create: %d\n", len(stackCfg.Users))
	for _, user := range stackCfg.Users {
		fmt.Printf("  - %s (GitHub: %s)\n", user.Username, user.GitHubUsername)
	}
	fmt.Printf("Instance Type: %s\n", stackCfg.InstanceType)

	cfClient := cloudformation.NewFromConfig(awsCfg)
	r53Client := route53.NewFromConfig(awsCfg)

	// Lookup zone ID if domain is specified
	var zoneID string
	if stackCfg.Domain != "" && stackCfg.Hostname != "" {
		fmt.Printf("Looking up zone ID for %s...\n", stackCfg.Domain)
		zoneID, err = lookupZoneID(ctx, r53Client, stackCfg.Domain)
		if err != nil {
			log.Fatalf("failed to lookup zone ID: %v", err)
		}
		fmt.Printf("Found Zone ID: %s\n", zoneID)
	}

	// Encode users for CloudFormation parameter
	usersParam := encodeUsers(stackCfg.Users)

	// Create CloudFormation stack
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

	// ... rest of stack creation logic ...

	// Update SSH command to use first user
	if stackCfg.FQDN != "" {
		stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.FQDN)
	} else {
		stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.Users[0].Username, stackCfg.PublicIP)
	}

	// ... rest of function ...
}
```

## Backward Compatibility

### Migration Path

**Existing configs continue to work unchanged**:

Old format:
```json
{
  "github_username": "gherlein"
}
```

Behavior: Creates user `gherlein` with keys from GitHub user `gherlein` (same as before).

**New format provides more flexibility**:
```json
{
  "users": [
    {"username": "admin", "github_username": "gherlein"}
  ]
}
```

Behavior: Creates user `admin` with keys from GitHub user `gherlein`.

### Config File Handling

- If config has `github_username` but not `users`, the validation function converts it to `users` array internally
- Config file is NOT rewritten with the conversion (maintains user's original format)
- Output fields remain the same

## Testing Plan

### Test Case 1: Single User (New Format)
```json
{
  "users": [
    {"username": "greg", "github_username": "gherlein"}
  ],
  "instance_type": "t3.micro"
}
```

Expected:
- Creates user `greg` with SSH keys from `gherlein`
- SSH command: `ssh greg@<IP>`

### Test Case 2: Multiple Users
```json
{
  "users": [
    {"username": "greg", "github_username": "gherlein"},
    {"username": "alice", "github_username": "alice-dev"},
    {"username": "bob", "github_username": "bob-admin"}
  ],
  "instance_type": "t3.micro"
}
```

Expected:
- Creates 3 users: `greg`, `alice`, `bob`
- Each user has sudo access
- Each user has SSH keys from their respective GitHub accounts
- SSH command uses first user: `ssh greg@<IP>`

### Test Case 3: Legacy Format (Backward Compatibility)
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro"
}
```

Expected:
- Creates user `gherlein` with SSH keys from `gherlein` (unchanged behavior)
- SSH command: `ssh gherlein@<IP>`

### Test Case 4: Validation Errors

**Duplicate usernames**:
```json
{
  "users": [
    {"username": "admin", "github_username": "user1"},
    {"username": "admin", "github_username": "user2"}
  ]
}
```
Expected: Error "duplicate username: admin"

**Empty username**:
```json
{
  "users": [
    {"username": "", "github_username": "gherlein"}
  ]
}
```
Expected: Error "user[0]: username cannot be empty"

**Invalid username format**:
```json
{
  "users": [
    {"username": "Admin", "github_username": "gherlein"}
  ]
}
```
Expected: Error "invalid username format: Admin (must be alphanumeric, start with letter)"

### Test Case 5: No Users Specified
```json
{
  "instance_type": "t3.micro"
}
```
Expected: Error "at least one user required: specify 'github_username' or 'users'"

## Documentation Updates

### README.md Updates

Update the configuration table:

```markdown
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_username` | Conditional | - | Your GitHub username (legacy, use `users` instead) |
| `users` | Conditional | - | Array of user objects to create on the host |

**Note**: Either `github_username` OR `users` must be specified.

#### User Object Fields

| Field | Required | Description |
|-------|----------|-------------|
| `username` | Yes | Linux username to create on the host |
| `github_username` | Yes | GitHub username to fetch SSH keys from |
```

Add examples:

```markdown
### Multiple Users

Create instance with multiple team members:

```json
{
  "users": [
    {"username": "greg", "github_username": "gherlein"},
    {"username": "alice", "github_username": "alice-dev"},
    {"username": "bob", "github_username": "bob-admin"}
  ],
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

All users get:
- Passwordless sudo access
- SSH keys from their GitHub accounts
- Home directory with proper permissions

Connect as any user:
```bash
ssh greg@dev.example.com
ssh alice@dev.example.com
ssh bob@dev.example.com
```

### Different Local and GitHub Usernames

Create user `admin` but pull keys from GitHub user `gherlein`:

```json
{
  "users": [
    {"username": "admin", "github_username": "gherlein"}
  ],
  "instance_type": "t3.micro"
}
```
```

## Implementation Checklist

- [ ] Add `User` struct to main.go
- [ ] Update `StackConfig` struct with `Users` field
- [ ] Implement `validateUserConfig()` function
- [ ] Implement `isValidLinuxUsername()` helper
- [ ] Implement `encodeUsers()` helper
- [ ] Update CloudFormation template with multi-user UserData script
- [ ] Update `createStack()` to validate and encode users
- [ ] Update `createStack()` to use first user in SSH command
- [ ] Update `example.json` with new format
- [ ] Update README.md with multi-user examples
- [ ] Test backward compatibility with `github_username`
- [ ] Test single user with new format
- [ ] Test multiple users
- [ ] Test validation errors

## Security Considerations

1. **All users get sudo**: Currently all users receive passwordless sudo access
   - This matches the existing single-user behavior
   - Future enhancement: could add optional `sudo` boolean per user

2. **SSH key validation**: No validation of GitHub usernames or SSH keys
   - Same as existing behavior
   - Invalid GitHub usernames result in empty `authorized_keys` (SSH login fails)

3. **Username injection**: Username validation prevents shell injection
   - Only alphanumeric, underscore, hyphen allowed
   - Must start with lowercase letter

## Edge Cases

1. **GitHub user with no keys**: Empty `authorized_keys` file, SSH login impossible for that user
2. **Very long users list**: CloudFormation parameter limit is 4096 characters
   - Each user entry is ~30-50 characters
   - Practical limit: ~80-130 users (far more than typical use case)
3. **User creation failure**: UserData script uses `set -e`, so first failure stops all user creation
   - CloudFormation stack creation will still succeed (UserData is asynchronous)
   - Cloud-init logs will show the error

## Future Enhancements

Potential future additions:
1. Per-user sudo configuration: `{"username": "alice", "github_username": "alice-dev", "sudo": false}`
2. Per-user groups: `{"username": "alice", "github_username": "alice-dev", "groups": ["docker", "wheel"]}`
3. Per-user shell: `{"username": "alice", "github_username": "alice-dev", "shell": "/bin/zsh"}`
4. Additional SSH key sources: Support direct SSH key input alongside GitHub
