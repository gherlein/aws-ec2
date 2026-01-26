# EC2 Instance Manager

A Go CLI tool to create and manage EC2 instances on AWS using CloudFormation, with optional Route53 DNS integration. Designed for quick provisioning of development or temporary instances with automatic SSH key setup from GitHub.

## Features

- **One-command provisioning**: Create a fully configured EC2 instance with a single command
- **DNS-only mode**: Manage Route53 DNS records for any infrastructure (no EC2 required)
- **Flexible configuration**: VM-only, DNS-only, or combined modes via nested config structure
- **GitHub SSH keys**: Automatically fetches your public SSH keys from GitHub
- **User creation**: Creates a Linux user matching your GitHub username with passwordless sudo
- **Route53 DNS**: Optionally creates an A record pointing to your instance
- **CloudFormation**: Uses CloudFormation for reliable, repeatable infrastructure
- **JSON config**: Simple JSON configuration files for each stack
- **Clean teardown**: Deletes DNS records and CloudFormation stack, clears config
- **Basic authentication**: Optional cloud-init templates include HTTP basic auth for web access

## Prerequisites

### Go

Go 1.21 or later is required to build the tool.

### AWS Credentials

Set up AWS credentials using one of these methods:

1. **Environment variables** (recommended for scripts):
   ```bash
   export AWS_REGION=us-west-2
   export AWS_ACCESS_KEY_ID=your-access-key
   export AWS_SECRET_ACCESS_KEY=your-secret-key
   ```

2. **AWS credentials file** (`~/.aws/credentials`):
   ```ini
   [default]
   aws_access_key_id = your-access-key
   aws_secret_access_key = your-secret-key
   ```

3. **AWS config file** (`~/.aws/config`):
   ```ini
   [default]
   region = us-west-2
   ```

### IAM Permissions

Your AWS user/role needs the following permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CloudFormation",
      "Effect": "Allow",
      "Action": [
        "cloudformation:CreateStack",
        "cloudformation:DeleteStack",
        "cloudformation:DescribeStacks",
        "cloudformation:DescribeStackEvents"
      ],
      "Resource": "*"
    },
    {
      "Sid": "EC2",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:CreateSecurityGroup",
        "ec2:DeleteSecurityGroup",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:DescribeSecurityGroups",
        "ec2:CreateTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SSM",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameters"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter/aws/service/ami-amazon-linux-latest/*"
    },
    {
      "Sid": "Route53",
      "Effect": "Allow",
      "Action": [
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
git clone <repository>
cd aws-ec2
make build
```

The binary will be placed in `./bin/ec2`.

### Installing to Your PATH

To install the binary to a directory in your PATH:

```bash
make install
```

This installs to `~/bin` by default. To install to a different location:

```bash
make install INSTALL_DIR=~/aaa
```

Make sure the target directory is in your PATH. To add `~/bin` to your PATH, add this to your `~/.bashrc` or `~/.zshrc`:

```bash
export PATH="$HOME/bin:$PATH"
```

## Quick Start

### 1. Create the stacks directory and copy the example config

```bash
mkdir -p stacks
cp example.json stacks/myserver.json
```

### 2. Edit the config

```bash
vi stacks/myserver.json
```

Set your GitHub username and optionally configure DNS:

```json
{
  "github_username": "your-github-username",
  "instance_type": "t3.micro",
  "hostname": "myserver",
  "domain": "example.com",
  "ttl": 300
}
```

**Tip:** Leave `hostname` empty to auto-generate a random 8-character hostname. This helps avoid Let's Encrypt rate limits during rapid testing:

```json
{
  "github_username": "your-github-username",
  "instance_type": "t3.micro",
  "hostname": "",
  "domain": "example.com",
  "ttl": 300
}
```

The tool will generate something like `a7k3m9xz.example.com` and save it to the config.

### 3. Create the instance

```bash
./bin/ec2 -c -n myserver
```

The tool automatically looks for `stacks/myserver.json` first. If not found, it treats the name as a path.

### 4. Connect via SSH

```bash
ssh your-github-username@myserver.example.com
```

### 5. Access the web interface (if using cloud-init/webserver.yaml)

The webserver is configured with HTTP Basic Authentication:

- **URL**: `https://myserver.example.com`
- **Username**: `emerging`
- **Password**: `emerging2026`

Your browser will prompt for credentials when you access the site.

### 6. Delete when done

```bash
./bin/ec2 -d -n myserver
```

## DNS-Only Mode (New!)

Manage Route53 DNS records for any infrastructure without creating EC2 instances.

### Quick DNS-Only Example

```bash
# Create DNS config for external server
cat > stacks/external.json << 'EOF'
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "app",
    "domain": "example.com",
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
EOF

# Create DNS records
./bin/ec2 -c -n external

# Delete when done
./bin/ec2 -d -n external
```

**Creates:**
- A: `app.example.com → 203.0.113.10`
- A: `example.com → 203.0.113.10`
- CNAME: `www.example.com → app.example.com`

**Use cases:**
- Point domains to DigitalOcean, Linode, Hetzner, etc.
- Manage DNS for existing EC2 instances
- Quick DNS setup for testing
- Load balancer or CDN configurations

**See [DNS_ONLY_GUIDE.md](DNS_ONLY_GUIDE.md) for complete documentation.**

## Configuration Modes

The tool supports three modes via nested configuration structure:

### 1. Full Stack (VM + DNS)

```json
{
  "vm": {
    "users": [{"username": "admin", "github_username": "gherlein"}]
  },
  "dns": {
    "hostname": "app",
    "domain": "example.com"
  }
}
```

Creates EC2 instance and DNS records (DNS uses VM's IP automatically).

### 2. DNS-Only

```json
{
  "dns": {
    "target_ip": "203.0.113.10",
    "domain": "example.com"
  }
}
```

Creates DNS records only (no EC2).

### 3. VM-Only

```json
{
  "vm": {
    "users": [{"username": "admin", "github_username": "gherlein"}]
  }
}
```

Creates EC2 instance only (no DNS, access via IP).

### Legacy Flat Format (Still Supported)

```json
{
  "users": [...],
  "hostname": "dev",
  "domain": "example.com"
}
```

Automatically converted to nested format internally.

## Configuration

Stack configuration files should be stored in the `./stacks/` directory. The tool automatically looks for `stacks/<name>.json` first, then falls back to treating the name as a direct path.

### Config File Structure

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "stack_name": "",
  "stack_id": "",
  "region": "",
  "instance_id": "",
  "public_ip": "",
  "security_group": "",
  "zone_id": "",
  "fqdn": "",
  "ssh_command": ""
}
```

### Input Fields (You Configure)

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_username` | **Yes** | - | Your GitHub username. SSH keys are fetched from `https://github.com/<username>.keys` |
| `instance_type` | No | `t3.micro` | EC2 instance type. See [Free Tier Types](#free-tier-instance-types) |
| `hostname` | No | - | DNS hostname without domain (e.g., `dev`). Required if using DNS |
| `domain` | No | - | Domain name for Route53 (e.g., `example.com`). Required if using DNS |
| `ttl` | No | `300` | DNS record TTL in seconds |

### Output Fields (Auto-Filled)

These fields are empty in a new config and are populated when the stack is created:

| Field | Description |
|-------|-------------|
| `stack_name` | CloudFormation stack name |
| `stack_id` | CloudFormation stack ARN |
| `region` | AWS region where the stack was created |
| `instance_id` | EC2 instance ID (e.g., `i-0abc123def456`) |
| `public_ip` | Public IPv4 address of the instance |
| `security_group` | Security group ID |
| `zone_id` | Route53 hosted zone ID (if DNS configured) |
| `fqdn` | Fully qualified domain name (if DNS configured) |
| `ssh_command` | Ready-to-use SSH command |

When you delete a stack, these output fields are cleared back to empty strings.

## Command Reference

```
Usage: ./bin/ec2 [options]

Options:
  -c, --create    Create a new EC2 instance
  -d, --delete    Delete an existing stack
  -n, --name      Stack name (required)
```

### Create a Stack

```bash
./bin/ec2 -c -n <stackname>
```

This command:
1. Looks for `stacks/<stackname>.json` (or uses the name as a path if not found)
2. Validates required fields (`github_username`)
3. Looks up Route53 hosted zone (if `domain` specified)
4. Creates CloudFormation stack with:
   - EC2 instance with specified instance type
   - Security group allowing SSH (port 22) from anywhere
   - UserData script that creates your user and installs SSH keys
5. Waits for stack creation to complete
6. Creates DNS A record (if `hostname` and `domain` specified)
7. Updates the config file with instance details

### Delete a Stack

```bash
./bin/ec2 -d -n <stackname>
```

This command:
1. Reads the config file for cleanup info
2. Deletes Route53 A record (if it was created)
3. Deletes CloudFormation stack (terminates EC2, deletes security group)
4. Waits for deletion to complete
5. Clears deployment-specific fields in the config file

## Examples

### Basic Usage (No DNS)

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro"
}
```

```bash
./bin/ec2 -c -n devbox
# Connect using IP from output
ssh gherlein@54.184.71.168
```

### With DNS

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

```bash
./bin/ec2 -c -n devbox
# Connect using hostname
ssh gherlein@dev.example.com
```

### Larger Instance

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.large",
  "hostname": "build",
  "domain": "example.com"
}
```

### After Creation

The config file is updated with instance details:

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "stack_name": "devbox",
  "stack_id": "arn:aws:cloudformation:us-west-2:123456789:stack/devbox/abc123",
  "region": "us-west-2",
  "instance_id": "i-0abc123def456789",
  "public_ip": "54.184.71.168",
  "security_group": "devbox-SSHSecurityGroup-XYZ123",
  "zone_id": "Z1234567890ABC",
  "fqdn": "dev.example.com",
  "ssh_command": "ssh gherlein@dev.example.com"
}
```

## Free Tier Instance Types

The following x86 instance types are free-tier eligible (750 hours/month for 12 months):

| Instance Type | vCPUs | Memory | Notes |
|--------------|-------|--------|-------|
| `t3.micro` | 2 | 1 GB | **Default**, general purpose |
| `t3.small` | 2 | 2 GB | More memory |
| `c7i-flex.large` | 2 | 4 GB | Compute optimized |
| `m7i-flex.large` | 2 | 8 GB | General purpose, more memory |

**Note**: Free tier eligibility depends on your AWS account status. Accounts created after a certain date may have different restrictions.

## Makefile Targets

```bash
make build                       # Build the binary to ./bin/ec2
make install                     # Build and install to ~/bin (or specify INSTALL_DIR=path)
make clean                       # Remove the bin directory
make status                      # Check CloudFormation stack events (requires STACK_NAME env var)
```

### Install to Custom Directory

```bash
make install INSTALL_DIR=/usr/local/bin    # Install to /usr/local/bin
make install INSTALL_DIR=~/my-tools        # Install to custom directory
```

### Check Stack Status

```bash
STACK_NAME=myserver make status
```

## How It Works

### Instance Provisioning

1. **AMI Selection**: Uses the latest Amazon Linux 2023 x86_64 AMI via SSM parameter lookup
2. **Security Group**: Creates a security group allowing inbound SSH (port 22) from `0.0.0.0/0`
3. **UserData Script**: Runs on first boot to:
   - Create a Linux user matching your GitHub username
   - Add user to `sudo` and `www-data` groups
   - Grant passwordless sudo access via `/etc/sudoers.d/`
   - Fetch SSH public keys from `https://github.com/<username>.keys`
   - Configure SSH authorized_keys

### DNS Integration

If `hostname` and `domain` are specified:
1. Looks up the Route53 hosted zone ID for the domain
2. Creates an A record: `<hostname>.<domain>` → `<public_ip>`
3. On deletion, removes the A record before deleting the stack

**Random Hostname Generation:**

If `hostname` is empty but `domain` is specified, the tool automatically generates a random 8-character hostname:
- Uses cryptographically secure random generation
- Characters: `a-z` and `0-9` (DNS-safe)
- Saves the generated hostname back to the config file
- Helps avoid Let's Encrypt rate limits during rapid create/delete cycles

Example workflow:
```json
{
  "hostname": "",
  "domain": "example.com"
}
```

Creates: `x3k9m2a7.example.com`

This is useful for:
- Testing and development iterations
- Avoiding Let's Encrypt's 5 certificates per domain per week limit
- Quick throwaway instances

## Troubleshooting

### "hosted zone not found for domain"

The domain must exist as a hosted zone in Route53. Check your hosted zones:

```bash
aws route53 list-hosted-zones --query 'HostedZones[*].[Name,Id]' --output table
```

### "instance type is not eligible for Free Tier"

Your AWS account may have restrictions. Use a free-tier eligible type:
- `t3.micro`
- `t3.small`
- `c7i-flex.large`
- `m7i-flex.large`

### SSH connection refused

The UserData script takes 1-2 minutes to complete after the instance starts. Wait and try again.

Check cloud-init status by connecting via EC2 Instance Connect in the AWS console, then:

```bash
sudo cat /var/log/cloud-init-output.log
```

### Stack creation failed

Check the CloudFormation events:

```bash
STACK_NAME=myserver make status
```

Or in the AWS console: CloudFormation → Stacks → Select stack → Events tab

### Permission denied (publickey)

1. Ensure your GitHub account has public SSH keys: `https://github.com/<username>.keys`
2. Make sure you're using the correct username (matches `github_username` in config)
3. Wait for cloud-init to complete (1-2 minutes after instance starts)

## Security Considerations

- **SSH Access**: The security group allows SSH from `0.0.0.0/0` (anywhere). For production, consider restricting to specific IP ranges.
- **Sudo Access**:
  - Users are added to the `sudo` group
  - Passwordless sudo is configured via `/etc/sudoers.d/<username>` with proper permissions (0440)
  - This provides `ALL=(ALL) NOPASSWD:ALL` access
  - Convenient for development/testing but consider restricting for production
- **Group Memberships**: Users are automatically added to:
  - `sudo` - Full administrative access
  - `www-data` - Web content deployment access
- **Public Keys**: SSH keys are fetched from GitHub over HTTPS. Ensure your GitHub account security is adequate.
- **Basic Authentication** (webserver cloud-init):
  - Default credentials: `emerging` / `emerging2026`
  - Change the password in `cloud-init/webserver.yaml` before deployment
  - For production, use stronger passwords and consider certificate-based auth

## Files

```
.
├── bin/
│   └── ec2              # Compiled binary
├── stacks/              # Stack configuration files (gitignored)
│   └── myserver.json    # Example stack config
├── example.json         # Example configuration template
├── main.go              # Source code
├── go.mod               # Go module definition
├── go.sum               # Go dependencies
├── Makefile             # Build automation
├── plan.md              # Implementation plan
├── .gitignore           # Git ignore file
└── README.md            # This file
```

## License

MIT
