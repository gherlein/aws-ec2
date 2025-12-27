# EC2 Instance Manager

A CLI tool to create and manage EC2 instances on AWS using CloudFormation, with optional Route53 DNS integration.

## Features

- Creates EC2 instances using CloudFormation
- Automatically fetches SSH public keys from GitHub for passwordless login
- Creates a Linux user matching your GitHub username with sudo access
- Optional Route53 DNS record creation (A record)
- Configurable instance type
- JSON config files for easy stack management

## Prerequisites

- Go 1.21+
- AWS CLI configured or environment variables set:
  - `AWS_REGION`
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`

### Required IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cloudformation:CreateStack",
        "cloudformation:DeleteStack",
        "cloudformation:DescribeStacks",
        "cloudformation:DescribeStackEvents",
        "ec2:*",
        "route53:ListHostedZonesByName",
        "route53:ChangeResourceRecordSets",
        "route53:GetHostedZone"
      ],
      "Resource": "*"
    }
  ]
}
```

## Installation

```bash
make build
```

The binary will be placed in `./bin/ec2`.

## Usage

### 1. Create a config file

Create `<stackname>.json` with your configuration:

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300
}
```

#### Config Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_username` | Yes | - | GitHub username for SSH key fetch |
| `instance_type` | No | `t3.micro` | EC2 instance type |
| `hostname` | No | - | DNS hostname (without domain) |
| `domain` | No | - | Domain name for Route53 lookup |
| `ttl` | No | `300` | DNS record TTL in seconds |

### 2. Create the stack

```bash
./bin/ec2 -c -n mystack
```

This will:
1. Read `mystack.json` for configuration
2. Look up the Route53 hosted zone for the domain (if specified)
3. Create a CloudFormation stack
4. Wait for the EC2 instance to be ready
5. Create a DNS A record (if hostname and domain are specified)
6. Update `mystack.json` with instance details

### 3. Connect via SSH

```bash
ssh gherlein@dev.example.com
# or
ssh gherlein@<public-ip>
```

### 4. Delete the stack

```bash
./bin/ec2 -d -n mystack
```

This will:
1. Read `mystack.json` for DNS cleanup info
2. Delete the Route53 A record (if it was created)
3. Delete the CloudFormation stack
4. Wait for deletion to complete

## Command Reference

```
Usage: ./bin/ec2 [options]

Options:
  -c, --create    Create a new EC2 instance
  -d, --delete    Delete an existing stack
  -n, --name      Stack name (required)

Examples:
  ./bin/ec2 -c -n mystack    Create stack using mystack.json config
  ./bin/ec2 -d -n mystack    Delete stack 'mystack'
```

## Example Workflow

### Create config file

```bash
cat > dev-server.json << 'EOF'
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
EOF
```

### Create the instance

```bash
./bin/ec2 -c -n dev-server
```

Output:
```
Using AWS Region: us-west-2
Stack Name: dev-server
GitHub Username: gherlein
Instance Type: t3.micro
Looking up zone ID for example.com...
Found Zone ID: Z1234567890ABC
Stack creation initiated!
Waiting for stack to complete...
Creating DNS record: dev.example.com -> 54.184.71.168
DNS record created successfully

=== Stack Created Successfully ===
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "stack_name": "dev-server",
  "stack_id": "arn:aws:cloudformation:us-west-2:...",
  "region": "us-west-2",
  "instance_id": "i-0abc123def456",
  "public_ip": "54.184.71.168",
  "security_group": "dev-server-SSHSecurityGroup-xxx",
  "zone_id": "Z1234567890ABC",
  "fqdn": "dev.example.com",
  "ssh_command": "ssh gherlein@dev.example.com"
}

Config updated: dev-server.json
SSH: ssh gherlein@dev.example.com
```

### Connect

```bash
ssh gherlein@dev.example.com
```

### Delete when done

```bash
./bin/ec2 -d -n dev-server
```

## Check Stack Status

```bash
STACK_NAME=dev-server make status
```

## Cleanup Build Artifacts

```bash
make clean
```

## Free Tier Instance Types

The following x86 instance types are free-tier eligible:
- `t3.micro` (default)
- `t3.small`
- `c7i-flex.large`
- `m7i-flex.large`

## Troubleshooting

### "hosted zone not found for domain"

Ensure the domain exists in Route53 as a hosted zone. You can check with:
```bash
aws route53 list-hosted-zones
```

### "instance type is not eligible for Free Tier"

Use a free-tier eligible instance type. See the list above.

### SSH connection refused

Wait 1-2 minutes after stack creation for cloud-init to complete user setup.
