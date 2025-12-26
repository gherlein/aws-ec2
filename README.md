# EC2 Instance Manager

A simple CLI tool to create and manage low-cost x86 EC2 instances on AWS using CloudFormation.

## Features

- Creates the lowest-cost free-tier eligible x86 EC2 instance (t3.micro)
- Automatically fetches SSH public keys from GitHub for passwordless login
- Creates a Linux user matching your GitHub username with sudo access
- Manages instances via CloudFormation stacks
- Outputs stack info to JSON files for easy reference

## Prerequisites

- Go 1.21+
- AWS CLI configured or environment variables set:
  - `AWS_REGION`
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`

## Installation

```bash
make build
```

The binary will be placed in `./bin/ec2`.

## Usage

### Create an instance

```bash
./bin/ec2 -c -n <stack-name> <github-username>
```

Options:
- `-c, --create` - Create a new EC2 instance
- `-n, --name` - Stack name (default: "ec2-instance")

Example:
```bash
./bin/ec2 -c -n my-dev-box gherlein
```

This will:
1. Create a CloudFormation stack named `my-dev-box`
2. Launch a t3.micro EC2 instance
3. Create a user `gherlein` with sudo access
4. Download SSH keys from `https://github.com/gherlein.keys`
5. Write stack info to `my-dev-box.json`

### Delete an instance

```bash
./bin/ec2 -d -n <stack-name>
```

Options:
- `-d, --delete` - Delete an existing stack
- `-n, --name` - Stack name to delete

Example:
```bash
./bin/ec2 -d -n my-dev-box
```

This will delete the CloudFormation stack and remove the JSON file.

### Check stack status

```bash
STACK_NAME=my-dev-box make status
```

## Output

After creation, a JSON file is written with stack details:

```json
{
  "stack_name": "my-dev-box",
  "stack_id": "arn:aws:cloudformation:us-west-2:...",
  "region": "us-west-2",
  "github_username": "gherlein",
  "instance_id": "i-0abc123def456",
  "instance_type": "t3.micro",
  "public_ip": "54.184.71.168",
  "security_group": "my-dev-box-SSHSecurityGroup-xxx",
  "ssh_command": "ssh gherlein@54.184.71.168"
}
```

## Connecting

After the instance is created, connect via SSH:

```bash
ssh <github-username>@<public-ip>
```

Or use the command from the JSON output:
```bash
jq -r '.ssh_command' my-dev-box.json | sh
```

## Cleanup

```bash
make clean
```
