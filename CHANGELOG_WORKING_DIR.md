# Changelog: Working Directory and Package Support

## Summary

Added support for configurable working directory and custom package installation via JSON config.

## Changes

### 1. JSON Configuration Schema

Added two new optional fields to stack config:

```json
{
  "working_dir": "/var/www/html",
  "packages": ["htop", "net-tools", "tree"]
}
```

**Fields:**
- `working_dir` (string, optional): Base directory for file deployments
  - Default: `/var/www/html`
  - Used by cloud-init for file placement
  - Accessible in templates as `{{.WorkingDir}}`

- `packages` (array of strings, optional): Additional packages to install
  - Installed via apt-get during cloud-init
  - Appended to base package list
  - Accessible in templates as `{{.Packages}}`

### 2. Go Code Changes

**File:** `main.go`

**StackConfig struct (line 40-56):**
```go
type StackConfig struct {
    // ... existing fields ...
    WorkingDir     string   `json:"working_dir,omitempty"`
    Packages       []string `json:"packages,omitempty"`
    // ... rest of fields ...
}
```

**CloudInitTemplateData struct (line 636-645):**
```go
type CloudInitTemplateData struct {
    Hostname   string
    Domain     string
    FQDN       string
    Region     string
    OS         string
    WorkingDir string    // NEW
    Packages   []string  // NEW
    Users      []User
}
```

**Template data population (line 1151-1166):**
```go
// Default working directory
workingDir := stackCfg.WorkingDir
if workingDir == "" {
    workingDir = "/var/www/html"
}

templateData := CloudInitTemplateData{
    Hostname:   stackCfg.Hostname,
    Domain:     stackCfg.Domain,
    FQDN:       fqdn,
    Region:     stackCfg.Region,
    OS:         stackCfg.OS,
    WorkingDir: workingDir,  // NEW
    Packages:   stackCfg.Packages,  // NEW
    Users:      stackCfg.Users,
}
```

### 3. Cloud-Init Template Changes

**File:** `cloud-init/webserver.yaml`

**Package installation:**
```yaml
packages:
  - curl
  - wget
  - vim
  - git
  - debian-keyring
  - debian-archive-keyring
  - apt-transport-https
{{range .Packages}}
  - {{.}}
{{end}}
```

**Dynamic working directory usage:**
```yaml
# Caddyfile
root * {{.WorkingDir}}

# Index file
- path: {{.WorkingDir}}/index.html

# Directory creation
- mkdir -p {{.WorkingDir}}
- chown -R www-data:www-data {{.WorkingDir}}
- chmod 775 {{.WorkingDir}}

# MOTD banner
Web root: {{.WorkingDir}}
```

### 4. Example Configurations

**File:** `example-webserver.json`

```json
{
  "region": "us-east-1",
  "os": "ubuntu-22.04",
  "cloud_init_file": "cloud-init/webserver.yaml",
  "working_dir": "/var/www/html",
  "packages": [
    "htop",
    "net-tools",
    "jq"
  ],
  "users": [
    {
      "username": "admin",
      "github_username": "gherlein"
    }
  ],
  "instance_type": "t3.micro",
  "hostname": "www",
  "domain": "example.com",
  "ttl": 300
}
```

**File:** `www/er.json`

```json
{
  "working_dir": "/var/www/html",
  "packages": [
    "htop",
    "net-tools",
    "tree"
  ]
}
```

### 5. Documentation Updates

**File:** `CLOUD_INIT_GUIDE.md`

- Added `WorkingDir` and `Packages` to template variables table
- Updated configuration examples
- Added examples showing package installation
- Added examples showing dynamic file placement with `WorkingDir`
- Updated webserver example results

## Migration

### Existing Configurations

No changes required. If `working_dir` is not specified, defaults to `/var/www/html`.

```json
{
  "cloud_init_file": "cloud-init/webserver.yaml"
}
```

Still works as before with default working directory.

### New Deployments

Recommended to explicitly set working directory:

```json
{
  "cloud_init_file": "cloud-init/webserver.yaml",
  "working_dir": "/var/www/html",
  "packages": ["htop", "tree"]
}
```

### Custom Working Directory

Change deployment location:

```json
{
  "working_dir": "/srv/myapp/public",
  "packages": ["nodejs", "npm"]
}
```

Cloud-init will:
1. Install base packages + nodejs, npm
2. Create `/srv/myapp/public`
3. Set ownership to `www-data:www-data`
4. Configure Caddy to serve from `/srv/myapp/public`
5. Place index.html in `/srv/myapp/public`

## Benefits

1. **Flexibility**: Deploy to any directory structure
2. **Package Management**: Install tools per-instance via config
3. **Template Reusability**: Same cloud-init file works for different directories
4. **Consistency**: Working directory used throughout entire setup
5. **Simplicity**: No need to fork cloud-init templates for different paths

## Use Cases

### Development Server
```json
{
  "working_dir": "/home/dev/public",
  "packages": ["nodejs", "npm", "build-essential"]
}
```

### Production Server
```json
{
  "working_dir": "/var/www/production",
  "packages": ["monitoring-agent", "backup-tool"]
}
```

### Staging Server
```json
{
  "working_dir": "/var/www/staging",
  "packages": ["debug-tools", "profiler"]
}
```

## Testing

Build and test:

```bash
cd ~/herlein/src/er/aws-ec2
make build

# Test with default working_dir
./bin/ec2 -c -n test1

# Test with custom working_dir
echo '{
  "working_dir": "/srv/custom",
  "packages": ["htop"],
  "users": [{"username": "test", "github_username": "gherlein"}],
  "hostname": "test",
  "domain": "example.com"
}' > test-custom.json

./bin/ec2 -c -n test-custom
```

Verify on instance:
```bash
ssh test@test.example.com
ls -la /srv/custom/
dpkg -l | grep htop
```

## Backward Compatibility

✅ Fully backward compatible
- Existing configs work without changes
- `working_dir` defaults to `/var/www/html`
- `packages` defaults to empty array
- No breaking changes to API or behavior
