# Cloud-Init Guide

## Overview

This tool supports optional cloud-init configuration to customize EC2 instances beyond user creation. Cloud-init files are YAML templates processed during instance provisioning.

## How It Works

### Architecture

The tool generates multipart MIME UserData with two parts:

1. **User Setup Script** (always included)
   - Creates Linux users from config
   - Installs SSH keys from GitHub
   - Grants sudo access

2. **Cloud-Init Config** (optional)
   - Specified via `cloud_init_file` in config
   - Processed as Go template with instance metadata
   - Installed as cloud-config YAML

### Processing Flow

```
Config File (JSON)
    ├─> users → Shell Script (Part 1)
    └─> cloud_init_file → YAML Template
                            ├─> Template Processing
                            └─> Cloud-Init Config (Part 2)
                                    ↓
                            Base64 Encoded UserData
                                    ↓
                            EC2 Instance Execution
```

### Template Variables

Cloud-init files are Go templates with access to:

| Variable | Type | Example | Description |
|----------|------|---------|-------------|
| `{{.Hostname}}` | string | `www` | Hostname without domain |
| `{{.Domain}}` | string | `example.com` | Domain name |
| `{{.FQDN}}` | string | `www.example.com` | Full domain name |
| `{{.Region}}` | string | `us-east-1` | AWS region |
| `{{.OS}}` | string | `ubuntu-22.04` | Operating system |
| `{{.WorkingDir}}` | string | `/var/www/html` | Working directory for deployments |
| `{{.Packages}}` | []string | `["htop", "tree"]` | Additional packages to install |
| `{{.Users}}` | []User | `[{Username: "admin", GitHubUsername: "gherlein"}]` | User array |

### User Object Structure

```go
type User struct {
    Username       string `json:"username"`
    GitHubUsername string `json:"github_username"`
}
```

Access in templates:
```yaml
{{range .Users}}
  - name: {{.Username}}
    github: {{.GitHubUsername}}
{{end}}
```

## Configuration

### Config File

Add `cloud_init_file`, `working_dir`, and `packages` to your stack config:

```json
{
  "region": "us-east-1",
  "os": "ubuntu-22.04",
  "cloud_init_file": "cloud-init/webserver.yaml",
  "working_dir": "/var/www/html",
  "packages": ["htop", "net-tools", "tree"],
  "users": [
    {"username": "admin", "github_username": "gherlein"}
  ],
  "hostname": "www",
  "domain": "example.com"
}
```

**New Fields:**
- `working_dir` (optional): Base directory for file deployments. Defaults to `/var/www/html` if not specified.
- `packages` (optional): Array of package names to install via apt-get. These are installed in addition to base packages in cloud-init.
- `hostname` (optional): Hostname for DNS. If empty and `domain` is specified, a random 8-character hostname is generated automatically.

### Path Resolution

Cloud-init file paths are resolved relative to the config file:

```
Config: local/myapp/config.json
Cloud-init: cloud-init/webserver.yaml
Resolved: local/myapp/cloud-init/webserver.yaml
```

Absolute paths work too:
```json
{
  "cloud_init_file": "/absolute/path/to/setup.yaml"
}
```

## Examples

### Basic Webserver (Caddy)

**File:** `cloud-init/webserver.yaml`

See the included `cloud-init/webserver.yaml` for a complete Caddy webserver setup that:
- Creates `caddy` system user
- Installs Caddy from official repo
- Configures Caddyfile with HTTPS
- Enables HTTP Basic Authentication (username: `emerging`, password: `emerging2026`)
- Sets up web root at `/var/www/html`
- Adds all users to `www-data` group for deployment access
- Sets `/var/www/html` permissions to 775 (group writable)
- Creates welcome page with server info
- Configures logging
- Adds MOTD banner with credentials

**Config:** `example-webserver.json`

```json
{
  "cloud_init_file": "cloud-init/webserver.yaml",
  "hostname": "www",
  "domain": "example.com"
}
```

**Deploy:**
```bash
./bin/ec2 -c -n webserver
```

**Result:**
- https://www.example.com serves content
- SSH: `ssh admin@www.example.com`
- Logs: `/var/log/caddy/access.log`
- Web files deployed to: `/var/www/html`
- Additional packages installed: `htop`, `net-tools`, `jq`
- User `admin` is in `www-data` group for deployment access

### Database Server

**File:** `cloud-init/postgres.yaml`

```yaml
#cloud-config
hostname: {{.Hostname}}
fqdn: {{.FQDN}}

packages:
  - postgresql-14
  - postgresql-contrib

write_files:
  - path: /etc/postgresql/14/main/pg_hba.conf
    content: |
      # Allow connections from app servers
      host    all    all    10.0.0.0/8    md5
    append: true

runcmd:
  - systemctl enable postgresql
  - systemctl start postgresql
  - sudo -u postgres createdb myapp
```

### Application Server

**File:** `cloud-init/app.yaml`

```yaml
#cloud-config
hostname: {{.Hostname}}
fqdn: {{.FQDN}}

packages:
  - docker.io
  - docker-compose

users:
  - name: app
    system: true
    groups: docker

write_files:
  - path: /opt/app/docker-compose.yml
    content: |
      version: '3.8'
      services:
        web:
          image: myapp:latest
          ports:
            - "8080:8080"
          environment:
            - DATABASE_URL=postgresql://{{.Hostname}}.{{.Domain}}/myapp
    owner: app:app

runcmd:
  - systemctl enable docker
  - systemctl start docker
  - cd /opt/app && docker-compose up -d
```

## Cloud-Init Features

### Packages

Install packages via apt/yum:

```yaml
packages:
  - nginx
  - postgresql-14
  - redis-server
{{range .Packages}}
  - {{.}}
{{end}}
```

The template includes packages from the JSON config automatically. Config packages are appended to the base package list in cloud-init.

**Example Config:**
```json
{
  "packages": ["htop", "tree", "docker.io"]
}
```

**Resulting Package List:**
- Base packages: curl, wget, vim, git, etc.
- Config packages: htop, tree, docker.io

### Users

Create system or regular users:

```yaml
users:
  - name: app
    system: true
    shell: /usr/sbin/nologin
    groups: docker
  - name: deploy
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3...
```

Note: User creation from GitHub is already handled by the shell script. Only add users here if you need additional system accounts.

### Files

Write files before other operations:

```yaml
write_files:
  - path: /etc/myapp/config.yaml
    content: |
      server:
        host: {{.FQDN}}
        port: 8080
    owner: app:app
    permissions: '0644'

  - path: {{.WorkingDir}}/index.html
    content: |
      <!DOCTYPE html>
      <html>
        <head><title>{{.FQDN}}</title></head>
        <body><h1>Welcome to {{.FQDN}}</h1></body>
      </html>
    owner: www-data:www-data
    permissions: '0644'

  - path: /opt/app/startup.sh
    content: |
      #!/bin/bash
      cd /opt/app
      ./myapp --config /etc/myapp/config.yaml
    permissions: '0755'
```

**Using WorkingDir:**
The `{{.WorkingDir}}` template variable allows dynamic file placement based on config:

```json
{
  "working_dir": "/srv/myapp/public"
}
```

Files will be created in `/srv/myapp/public` instead of the default `/var/www/html`.

### Commands

Run commands during boot:

```yaml
runcmd:
  - apt-get update
  - apt-get upgrade -y
  - systemctl enable myapp
  - systemctl start myapp
  - echo "Setup complete" >> /var/log/setup.log
```

Commands run as root. Use `sudo -u user` for specific users:

```yaml
runcmd:
  - sudo -u app /opt/app/setup.sh
```

### Modifying Users Created by Shell Script

Users are created by the shell script before cloud-init runs with:
- **Groups**: `sudo` and `www-data` (pre-configured)
- **Sudo Access**: NOPASSWD:ALL via `/etc/sudoers.d/<username>` (0440 permissions)

Use templates to add users to additional groups:

```yaml
runcmd:
  # Add all users to Docker group
{{range .Users}}
  - usermod -a -G docker {{.Username}}
{{end}}
```

**Note:** Users already have `sudo` and `www-data` group membership from the shell script.

This is useful for:
- Adding users to Docker group
- Adding users to application-specific groups
- Setting up service-specific permissions

### Networking

Configure hostname and hosts file:

```yaml
hostname: {{.Hostname}}
fqdn: {{.FQDN}}
manage_etc_hosts: true
```

This ensures `/etc/hosts` has:
```
127.0.0.1 www.example.com www
```

## Debugging

### Check Cloud-Init Status

SSH into instance and check status:

```bash
# Overall status
cloud-init status

# Detailed output
sudo cat /var/log/cloud-init-output.log

# Cloud-init log
sudo cat /var/log/cloud-init.log

# Check if complete
cloud-init status --wait
```

### Verify UserData

Check what was passed to the instance:

```bash
# View raw UserData
sudo cat /var/lib/cloud/instance/user-data.txt

# View decoded MIME parts
sudo cat /var/lib/cloud/instance/scripts/*
```

### Common Issues

#### "Failed to read cloud-init file"

Path is incorrect. Check:
1. File exists relative to config file
2. File permissions (readable)
3. Path in config is correct

```bash
# From config directory
ls -la cloud-init/webserver.yaml
```

#### "Failed to parse cloud-init template"

Template syntax error. Check:
1. All `{{` have matching `}}`
2. Variable names are correct (case-sensitive)
3. YAML syntax is valid

Test template manually:
```bash
# Install yq for YAML validation
yq eval . cloud-init/webserver.yaml
```

#### Service fails to start

User doesn't exist. Ensure cloud-init creates the user before installing services:

```yaml
# WRONG: Service runs as 'caddy' but user not created
packages:
  - caddy
runcmd:
  - systemctl start caddy

# RIGHT: Create user first
users:
  - name: caddy
    system: true
packages:
  - caddy
runcmd:
  - systemctl start caddy
```

#### Template variables empty

Variable not passed or misspelled:

```yaml
# Check variable name (case-sensitive)
hostname: {{.Hostname}}  # Correct
hostname: {{.hostname}}  # Wrong - empty
hostname: {{.Host}}      # Wrong - empty
```

## Best Practices

### 1. Keep it idempotent

Cloud-init runs once on first boot. Make scripts safe for re-runs:

```yaml
runcmd:
  - mkdir -p /opt/app  # OK: -p flag
  - mkdir /opt/app     # BAD: fails if exists
```

### 2. Use systemd for services

Don't run long-running processes directly:

```yaml
# WRONG
runcmd:
  - /opt/app/server &

# RIGHT
write_files:
  - path: /etc/systemd/system/myapp.service
    content: |
      [Unit]
      Description=My App

      [Service]
      ExecStart=/opt/app/server
      Restart=always

      [Install]
      WantedBy=multi-user.target

runcmd:
  - systemctl daemon-reload
  - systemctl enable myapp
  - systemctl start myapp
```

### 3. Create users before installing services

If a service runs as a specific user, create that user first:

```yaml
users:
  - name: caddy
    system: true

packages:
  - caddy  # Installs after users are created
```

### 4. Use final_message for debugging

Add a completion marker:

```yaml
final_message: "Setup complete. System boot took $UPTIME seconds."
```

Check with:
```bash
sudo cat /var/log/cloud-init.log | grep "Setup complete"
```

### 5. Log everything important

```yaml
runcmd:
  - echo "Starting app deployment" >> /var/log/deploy.log
  - /opt/app/deploy.sh >> /var/log/deploy.log 2>&1
  - echo "Deployment complete" >> /var/log/deploy.log
```

## Reference

### Full Cloud-Init Schema

See official docs: https://cloudinit.readthedocs.io/

Common directives:

| Directive | Purpose |
|-----------|---------|
| `packages` | Install packages |
| `users` | Create users |
| `write_files` | Write files to disk |
| `runcmd` | Run commands |
| `hostname` | Set hostname |
| `fqdn` | Set FQDN |
| `ssh_authorized_keys` | Add SSH keys |
| `apt` | Configure apt |
| `yum_repos` | Configure yum |
| `bootcmd` | Run before any other directives |
| `final_message` | Print when complete |

### Go Template Syntax

Available in cloud-init files:

```yaml
# Variables
{{.Hostname}}

# Conditionals
{{if .Domain}}
server_name: {{.FQDN}}
{{else}}
server_name: {{.Hostname}}
{{end}}

# Range (loops)
{{range .Users}}
- username: {{.Username}}
  github: {{.GitHubUsername}}
{{end}}

# With (scope)
{{with .Users}}
user_count: {{len .}}
{{end}}
```

See: https://pkg.go.dev/text/template

## Migration Guide

### From Manual UserData to Cloud-Init

If you have custom UserData scripts:

**Before:**
```bash
#!/bin/bash
apt-get install -y nginx
systemctl enable nginx
```

**After:**

1. Create `cloud-init/mysetup.yaml`:
```yaml
#cloud-config
packages:
  - nginx

runcmd:
  - systemctl enable nginx
  - systemctl start nginx
```

2. Add to config:
```json
{
  "cloud_init_file": "cloud-init/mysetup.yaml"
}
```

### From Other Tools

**Terraform** - extract user_data to separate file
**CloudFormation** - extract UserData to cloud-init YAML
**Ansible** - convert playbook tasks to cloud-init

## Troubleshooting Checklist

- [ ] Cloud-init file exists and is readable
- [ ] Path in config is relative to config file
- [ ] YAML syntax is valid
- [ ] Template variables are spelled correctly
- [ ] Users are created before services that need them
- [ ] Commands are idempotent
- [ ] Services use systemd units
- [ ] Logs show completion (`cloud-init status`)
- [ ] No errors in `/var/log/cloud-init-output.log`
