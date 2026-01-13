# Features Summary

Complete reference for all EC2 instance manager features.

## User Creation & Permissions

### Automatic Setup

Every user specified in the config is automatically configured with:

**Group Memberships:**
- `sudo` - Administrative access
- `www-data` - Web content deployment access

**Sudo Access:**
- Type: Passwordless (`NOPASSWD:ALL`)
- Configuration: `/etc/sudoers.d/<username>`
- Permissions: `0440` (read-only, root-owned)
- Scope: Full system access

**SSH Access:**
- Keys fetched from: `https://github.com/<github_username>.keys`
- Location: `/home/<username>/.ssh/authorized_keys`
- Permissions: `0600`
- Home directory: `/home/<username>` (created with `-m` flag)

**Shell:**
- Default: `/bin/bash`
- Interactive login enabled

### Example User Setup

**Config:**
```json
{
  "users": [
    {"username": "admin", "github_username": "gherlein"},
    {"username": "luca", "github_username": "lherlein"}
  ]
}
```

**Result on Instance:**

```bash
# User: admin
groups admin
# Output: admin sudo www-data

sudo -l -U admin
# Output: (ALL) NOPASSWD: ALL

# User: luca
groups luca
# Output: luca sudo www-data

# Both can deploy
admin$ echo "test" > /var/www/html/test.txt  # ✓ Works
luca$ echo "test" > /var/www/html/test.txt   # ✓ Works

# Both can sudo
admin$ sudo systemctl status caddy  # ✓ No password
luca$ sudo systemctl reload caddy   # ✓ No password
```

## Random Hostname Generation

### Feature

Automatically generate unique, random hostnames to avoid DNS and certificate conflicts.

### Trigger

When `hostname` is empty AND `domain` is specified:

```json
{
  "hostname": "",
  "domain": "example.com"
}
```

### Behavior

1. Generates 8-character random string
2. Characters: `a-z` and `0-9` (DNS-safe)
3. Uses cryptographically secure RNG (`crypto/rand`)
4. Saves generated hostname to config file
5. Prints to console: `Generated random hostname: k3m9x2a7`

### Example

**Before Creation:**
```json
{
  "hostname": "",
  "domain": "example.com",
  "users": [{"username": "test", "github_username": "gherlein"}]
}
```

**After Creation:**
```json
{
  "hostname": "k3m9x2a7",
  "domain": "example.com",
  "users": [{"username": "test", "github_username": "gherlein"}],
  "fqdn": "k3m9x2a7.example.com",
  "public_ip": "54.184.71.168",
  ...
}
```

**Access:**
```bash
ssh test@k3m9x2a7.example.com
```

### Use Cases

**1. Rapid Testing**

```bash
# Test iteration 1
./bin/ec2 -c -n test
# Generated: a7k3m9.example.com
# Test...
./bin/ec2 -d -n test

# Test iteration 2
./bin/ec2 -c -n test
# Generated: x9p2q4.example.com (different!)
# Test...
./bin/ec2 -d -n test

# No conflicts, no rate limits
```

**2. Let's Encrypt Rate Limit Avoidance**

Let's Encrypt limits:
- 5 certificates per exact domain per week
- 50 certificates per registered domain per week

Without random hostnames:
```bash
# Create test.example.com (cert 1)
# Delete test.example.com
# Create test.example.com (cert 2)
# ...
# Create test.example.com (cert 5)
# ❌ RATE LIMITED for rest of week
```

With random hostnames:
```bash
# Create a3k5m7.example.com (cert 1)
# Create x9p2q4.example.com (cert 2)
# Create m7n8r3.example.com (cert 3)
# ...
# Create k3m9x2.example.com (cert 50)
# ✓ Still within limit (50 per registered domain)
```

**3. CI/CD Testing**

```bash
# Each PR gets unique hostname
PR-123: pr123-k3m9.example.com
PR-124: pr124-x9p2.example.com
PR-125: pr125-m7n8.example.com
```

**4. Throwaway Instances**

```bash
# Quick tests without managing hostnames
./bin/ec2 -c -n quicktest
# Generated: random hostname
# Use and discard
./bin/ec2 -d -n quicktest
```

## Working Directory Configuration

### Feature

Configurable base directory for web content deployment.

### Configuration

```json
{
  "working_dir": "/var/www/html"
}
```

**Default:** `/var/www/html` if not specified

### Behavior

- Directory created during instance setup
- Ownership: `www-data:www-data`
- Permissions: `2775` (setgid + group writable)
- Caddy serves from this directory
- All users can deploy files (via www-data group)

### Template Usage

Available in cloud-init templates as `{{.WorkingDir}}`:

```yaml
write_files:
  - path: {{.WorkingDir}}/index.html
    content: |
      <html>...</html>

runcmd:
  - mkdir -p {{.WorkingDir}}
  - chown www-data:www-data {{.WorkingDir}}
```

## Custom Package Installation

### Feature

Install additional packages beyond base set.

### Configuration

```json
{
  "packages": ["htop", "tree", "docker.io"]
}
```

### Behavior

Packages are appended to base package list in cloud-init:

**Base packages:**
- curl, wget, vim, git, debian-keyring, debian-archive-keyring, apt-transport-https

**Config packages:**
- Added via template: `{{range .Packages}}`

**Result:**
All packages installed via `apt-get install` during instance boot.

### Example

**Config:**
```json
{
  "packages": ["htop", "net-tools", "tree"]
}
```

**Installed on Instance:**
```bash
dpkg -l | grep -E "htop|net-tools|tree"
# All three packages installed
```

## DNS Features

### Standard A Record

```json
{
  "hostname": "dev",
  "domain": "example.com"
}
```

Creates: `dev.example.com -> <IP>`

### CNAME Aliases

```json
{
  "hostname": "app",
  "domain": "example.com",
  "cname_aliases": ["api", "staging"]
}
```

Creates:
- A: `app.example.com -> <IP>`
- CNAME: `api.example.com -> app.example.com`
- CNAME: `staging.example.com -> app.example.com`

### Apex Domain

```json
{
  "hostname": "www",
  "domain": "example.com",
  "is_apex_domain": true
}
```

Creates:
- A: `www.example.com -> <IP>`
- A: `example.com -> <IP>`

### All Features Combined

```json
{
  "hostname": "",
  "domain": "example.com",
  "is_apex_domain": true,
  "cname_aliases": ["www", "api"]
}
```

Creates (with generated hostname `k3m9x2a7`):
- A: `k3m9x2a7.example.com -> <IP>`
- CNAME: `www.example.com -> k3m9x2a7.example.com`
- CNAME: `api.example.com -> k3m9x2a7.example.com`
- A: `example.com -> <IP>`

## Cloud-Init Integration

### Feature

Optional cloud-init YAML for custom instance configuration.

### Configuration

```json
{
  "cloud_init_file": "cloud-init/webserver.yaml"
}
```

Path is relative to config file location.

### Available Templates

**Webserver:** `cloud-init/webserver.yaml`
- Installs Caddy
- Configures HTTPS
- Sets up web root
- Creates welcome page
- Adds MOTD banner

**Template Variables:**
- `{{.Hostname}}` - Hostname (may be random)
- `{{.Domain}}` - Domain name
- `{{.FQDN}}` - Full domain (hostname.domain)
- `{{.Region}}` - AWS region
- `{{.OS}}` - Operating system
- `{{.WorkingDir}}` - Working directory
- `{{.Packages}}` - Package array
- `{{.Users}}` - User array

## Complete Configuration Example

```json
{
  "region": "us-east-1",
  "os": "ubuntu-22.04",
  "cloud_init_file": "cloud-init/webserver.yaml",
  "working_dir": "/var/www/html",
  "packages": ["htop", "tree", "net-tools"],
  "users": [
    {"username": "admin", "github_username": "gherlein"},
    {"username": "dev", "github_username": "lherlein"}
  ],
  "instance_type": "t3.micro",
  "hostname": "",
  "domain": "example.com",
  "ttl": 300,
  "is_apex_domain": true,
  "cname_aliases": ["www", "api"],
  "vpc_id": "vpc-123456",
  "subnet_id": "subnet-789012"
}
```

**What This Creates:**

**Instance:**
- Type: t3.micro
- OS: Ubuntu 22.04
- Region: us-east-1
- VPC: vpc-123456
- Subnet: subnet-789012

**Users:**
- `admin` (GitHub: gherlein) - groups: sudo, www-data
- `dev` (GitHub: lherlein) - groups: sudo, www-data
- Both have: NOPASSWD:ALL sudo access

**Software:**
- Caddy webserver
- htop, tree, net-tools (custom packages)
- curl, wget, vim, git (base packages)

**Web Setup:**
- Working dir: `/var/www/html` (2775 permissions)
- Caddy serves from: `/var/www/html`
- HTTPS automatic via Let's Encrypt

**DNS (with random hostname `k3m9x2a7`):**
- A: `k3m9x2a7.example.com -> 54.184.71.168`
- CNAME: `www.example.com -> k3m9x2a7.example.com`
- CNAME: `api.example.com -> k3m9x2a7.example.com`
- A: `example.com -> 54.184.71.168`

**Access:**
```bash
# All these work
ssh admin@k3m9x2a7.example.com
ssh admin@www.example.com
ssh admin@api.example.com
ssh admin@example.com

ssh dev@www.example.com
ssh dev@api.example.com
```

**Deployment:**
```bash
make deploy CONFIG=myconfig.json
# Deploys to /var/www/html
# Changes immediately available
# Site live at https://www.example.com
```

## Quick Reference

### Common Tasks

```bash
# Create instance with random hostname
./bin/ec2 -c -n mystack

# Create instance with specific hostname
# (edit hostname in stacks/mystack.json first)
./bin/ec2 -c -n mystack

# Delete instance
./bin/ec2 -d -n mystack

# Deploy website
make deploy CONFIG=er.json

# Preview deployment
make deploy-dry-run CONFIG=er.json
```

### User Permissions Cheat Sheet

```bash
# Check groups
groups
# Output: username sudo www-data

# Check sudo access
sudo -l
# Output: (ALL) NOPASSWD: ALL

# Deploy files (no sudo needed)
echo "content" > /var/www/html/file.html

# Check Caddy status (if needed)
sudo systemctl status caddy
```

### File Permissions Cheat Sheet

```bash
# Web root
ls -la /var/www/
# drwxrwsr-x www-data www-data html

# User home
ls -la /home/admin/
# drwxr-xr-x admin admin .

# Sudoers
ls -la /etc/sudoers.d/admin
# -r--r----- root root admin
```

## Feature Matrix

| Feature | Config Field | Default | Required |
|---------|-------------|---------|----------|
| Random hostname | `hostname: ""` | None | No (only if domain specified) |
| Custom hostname | `hostname: "dev"` | None | No |
| Domain | `domain: "example.com"` | None | No (can use IP) |
| Working directory | `working_dir: "/var/www/html"` | `/var/www/html` | No |
| Custom packages | `packages: ["htop"]` | `[]` | No |
| Cloud-init | `cloud_init_file: "path.yaml"` | None | No |
| Multi-user | `users: [{...}, {...}]` | None | Yes |
| Apex domain | `is_apex_domain: true` | `false` | No |
| CNAME aliases | `cname_aliases: ["www"]` | `[]` | No |
| Instance type | `instance_type: "t3.micro"` | `t3.micro` | No |
| Operating system | `os: "ubuntu-22.04"` | `ubuntu-22.04` | No |
| Region | `region: "us-east-1"` | `us-east-1` | No |
| VPC | `vpc_id: "vpc-xxx"` | Auto-discover | No |
| Subnet | `subnet_id: "subnet-xxx"` | Auto-discover | No |

## Configuration Examples by Use Case

### Development Server (Random Hostname)

```json
{
  "hostname": "",
  "domain": "dev.company.com",
  "users": [{"username": "dev", "github_username": "yourname"}],
  "packages": ["docker.io", "build-essential"]
}
```

### Production Web Server (Fixed Hostname)

```json
{
  "hostname": "www",
  "domain": "company.com",
  "is_apex_domain": true,
  "cloud_init_file": "cloud-init/webserver.yaml",
  "users": [{"username": "deploy", "github_username": "deploybot"}],
  "instance_type": "t3.small"
}
```

### Team Development Server

```json
{
  "hostname": "team",
  "domain": "dev.company.com",
  "users": [
    {"username": "alice", "github_username": "alice"},
    {"username": "bob", "github_username": "bob"},
    {"username": "charlie", "github_username": "charlie"}
  ],
  "packages": ["vim", "tmux", "htop"],
  "cname_aliases": ["dev", "staging"]
}
```

### Minimal Configuration

```json
{
  "users": [{"username": "admin", "github_username": "yourname"}]
}
```

Creates instance with:
- No DNS (IP access only)
- Default instance type (t3.micro)
- Default OS (ubuntu-22.04)
- Default region (us-east-1)
- Auto-discovered VPC/subnet

## All Available Config Fields

### Input Fields

| Field | Type | Example | Description |
|-------|------|---------|-------------|
| `region` | string | `"us-east-1"` | AWS region |
| `os` | string | `"ubuntu-22.04"` | Operating system |
| `cloud_init_file` | string | `"cloud-init/webserver.yaml"` | Path to cloud-init template |
| `working_dir` | string | `"/var/www/html"` | Base directory for deployments |
| `packages` | array | `["htop", "tree"]` | Additional packages to install |
| `users` | array | `[{username, github_username}]` | Users to create |
| `instance_type` | string | `"t3.micro"` | EC2 instance type |
| `hostname` | string | `"dev"` or `""` | Hostname (empty = random) |
| `domain` | string | `"example.com"` | Domain name |
| `ttl` | number | `300` | DNS record TTL (seconds) |
| `is_apex_domain` | boolean | `true` | Create apex A record |
| `cname_aliases` | array | `["www", "api"]` | CNAME aliases to create |
| `vpc_id` | string | `"vpc-123456"` | VPC ID (auto-discovered if empty) |
| `subnet_id` | string | `"subnet-789"` | Subnet ID (auto-discovered if empty) |

### Output Fields (Auto-Filled)

| Field | Type | Example | Description |
|-------|------|---------|-------------|
| `stack_name` | string | `"mystack"` | CloudFormation stack name |
| `stack_id` | string | `"arn:aws:..."` | CloudFormation stack ARN |
| `ami_id` | string | `"ami-0030e43..."` | AMI ID used |
| `instance_id` | string | `"i-0abc123..."` | EC2 instance ID |
| `public_ip` | string | `"54.184.71.168"` | Public IPv4 address |
| `security_group` | string | `"sg-0acf5e..."` | Security group ID |
| `zone_id` | string | `"Z0333201..."` | Route53 zone ID |
| `fqdn` | string | `"dev.example.com"` | Fully qualified domain |
| `ssh_command` | string | `"ssh user@host"` | Ready SSH command |
| `dns_records` | array | `[{name, type, value, ttl}]` | Created DNS records |

## Deployment Workflow

### Complete Workflow

```bash
# 1. Create config with random hostname
cat > stacks/test.json << 'EOF'
{
  "hostname": "",
  "domain": "example.com",
  "cloud_init_file": "cloud-init/webserver.yaml",
  "users": [{"username": "admin", "github_username": "gherlein"}]
}
EOF

# 2. Create instance
./bin/ec2 -c -n test
# Output:
#   Generated random hostname: k3m9x2a7
#   Stack created
#   DNS: k3m9x2a7.example.com

# 3. Wait for cloud-init (1-2 minutes)
# Check status: cloud-init status

# 4. Deploy website
cd ../www
make deploy CONFIG=../aws-ec2/stacks/test.json
# Deploys to /var/www/html
# Reloads Caddy automatically

# 5. Verify
curl https://k3m9x2a7.example.com
# Site is live!

# 6. Clean up when done
cd ../aws-ec2
./bin/ec2 -d -n test
# Deletes DNS and terminates instance
```

## Troubleshooting

### Random Hostname Issues

**Hostname not generated:**
- Check `domain` is specified in config
- Check `hostname` is empty string (not missing)

**Hostname not saved to config:**
- Check file permissions (must be writable)
- Check for file system errors

### Permission Issues

**Can't write to /var/www/html:**
```bash
# Check group membership
groups
# Should show: username sudo www-data

# If missing, add manually
sudo usermod -a -G www-data $USER
# Log out and back in
```

**Sudo prompts for password:**
```bash
# Check sudoers file exists
ls -la /etc/sudoers.d/$USER

# Check content
sudo cat /etc/sudoers.d/$USER
# Should show: username ALL=(ALL) NOPASSWD:ALL

# Check permissions
ls -la /etc/sudoers.d/$USER
# Should show: -r--r----- root root
```

### Deployment Fails

**rsync permission denied:**
- User not in www-data group
- Working directory wrong permissions
- User not logged in since group add

**Caddy reload fails:**
- Sudoers not configured (but should be automatic)
- Caddy not installed
- Config syntax error

## Summary

All features are designed to work together for rapid development and deployment:

1. ✅ **Random hostnames** - No conflicts, no rate limits
2. ✅ **Auto sudo/www-data groups** - Deploy immediately
3. ✅ **Configurable working dir** - Flexible deployment targets
4. ✅ **Custom packages** - Install exactly what you need
5. ✅ **Cloud-init templates** - Customize instance setup
6. ✅ **Instant file serving** - Caddy serves changes immediately
7. ✅ **Multi-user support** - Team collaboration ready

The tool handles all the complexity so you can focus on building and deploying.
