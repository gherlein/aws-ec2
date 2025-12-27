# Plan: JSON Configuration File Support

## Overview

Refactor the CLI to use a JSON configuration file for all stack parameters. The user provides only the stack name on the command line, and the program reads `<stackname>.json` for configuration.

## Configuration File Format

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300
}
```

### Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_username` | Yes | - | GitHub username for SSH key fetch |
| `instance_type` | No | `t3.micro` | EC2 instance type |
| `hostname` | No | - | DNS hostname (without domain) |
| `domain` | No | - | Domain name for Route53 lookup |
| `ttl` | No | `300` | DNS record TTL in seconds |

## CLI Changes

### New Usage

```bash
# Create - reads mystack.json for config
./bin/ec2 -c -n mystack

# Delete - reads mystack.json to know what to clean up
./bin/ec2 -d -n mystack
```

### Workflow

#### Create

1. Parse `-n <stackname>` from command line
2. Read `<stackname>.json` config file
3. Validate required fields
4. If `domain` is specified:
   - Call Route53 `ListHostedZonesByName` to find zone ID
   - Validate zone exists
5. Create CloudFormation stack with instance type from config
6. Wait for stack completion
7. Get public IP from stack outputs
8. If `hostname` and `domain` specified:
   - Create/update Route53 A record: `<hostname>.<domain>` → public IP
9. Update `<stackname>.json` with output fields:
   - `stack_id`
   - `instance_id`
   - `public_ip`
   - `security_group`
   - `zone_id` (if DNS configured)
   - `fqdn` (if DNS configured)
   - `ssh_command`

#### Delete

1. Parse `-n <stackname>` from command line
2. Read `<stackname>.json` to get `zone_id` and `fqdn`
3. If DNS was configured:
   - Delete Route53 A record
4. Delete CloudFormation stack
5. Wait for stack deletion
6. Optionally: remove or mark config file as deleted

## Implementation Steps

### Step 1: Define Config Structs

```go
type StackConfig struct {
    // Input fields (user provides)
    GitHubUsername string `json:"github_username"`
    InstanceType   string `json:"instance_type,omitempty"`
    Hostname       string `json:"hostname,omitempty"`
    Domain         string `json:"domain,omitempty"`
    TTL            int    `json:"ttl,omitempty"`

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

### Step 2: Add Route53 Functions

```go
func lookupZoneID(domain string) (string, error)
func createDNSRecord(zoneID, fqdn, ip string, ttl int) error
func deleteDNSRecord(zoneID, fqdn, ip string) error
```

### Step 3: Update CloudFormation Template

- Make `InstanceType` a parameter instead of hardcoded
- Pass instance type from config

### Step 4: Update Main Logic

1. Remove `github_username` positional argument
2. Add config file reading
3. Add config file writing (merge input + output)
4. Add Route53 integration
5. Update delete to read config for cleanup

### Step 5: Update Makefile

Add `go.mod` dependency for Route53:
```
github.com/aws/aws-sdk-go-v2/service/route53
```

## Required IAM Permissions

Existing:
- `cloudformation:CreateStack`
- `cloudformation:DeleteStack`
- `cloudformation:DescribeStacks`
- `ec2:*` (for CloudFormation)

New:
- `route53:ListHostedZonesByName`
- `route53:ChangeResourceRecordSets`
- `route53:GetHostedZone`

## Example Workflow

### 1. User creates config file

```bash
cat > mystack.json << 'EOF'
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
EOF
```

### 2. User creates stack

```bash
./bin/ec2 -c -n mystack
```

Output:
```
Using AWS Region: us-west-2
Stack Name: mystack
Reading config from mystack.json...
GitHub Username: gherlein
Instance Type: t3.micro
Looking up zone ID for example.com...
Found Zone ID: Z1234567890ABC
Stack creation initiated!
Waiting for stack to complete...

=== Stack Created Successfully ===
Creating DNS record: dev.example.com -> 54.184.71.168
DNS record created successfully

Config updated: mystack.json
SSH: ssh gherlein@dev.example.com
```

### 3. Config file after creation

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "stack_name": "mystack",
  "stack_id": "arn:aws:cloudformation:us-west-2:...",
  "region": "us-west-2",
  "instance_id": "i-0abc123def456",
  "public_ip": "54.184.71.168",
  "security_group": "mystack-SSHSecurityGroup-xxx",
  "zone_id": "Z1234567890ABC",
  "fqdn": "dev.example.com",
  "ssh_command": "ssh gherlein@dev.example.com"
}
```

### 4. User deletes stack

```bash
./bin/ec2 -d -n mystack
```

Output:
```
Using AWS Region: us-west-2
Reading config from mystack.json...
Deleting DNS record: dev.example.com...
DNS record deleted
Deleting stack: mystack...
Stack deleted successfully
```

## Error Handling

- Config file not found → error with example config
- Missing required field → error listing missing fields
- Domain not found in Route53 → error with available zones
- Instance type not free-tier eligible → warning, proceed anyway
- DNS creation fails → warning, stack still created

## Future Enhancements

- Support multiple hostnames per stack
- Support AAAA records for IPv6
- Add `--dry-run` flag to preview changes
- Add `ec2 status -n <stack>` command
- Add `ec2 ssh -n <stack>` command to auto-connect
- Add `ec2 init -n <stack>` to generate template config file
