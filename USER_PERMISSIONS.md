# User Permissions Reference

## Overview

All users created by this tool are configured with full administrative access and web deployment capabilities.

## Default Permissions

### Group Memberships

Every user is automatically added to:

| Group | Purpose | Access Granted |
|-------|---------|----------------|
| `sudo` | Administrative | Full system control via sudo |
| `www-data` | Web deployment | Write access to web directories |

### Sudo Configuration

**Type:** Passwordless sudo
**Scope:** Full system access
**Configuration:** `/etc/sudoers.d/<username>`
**Permissions:** `0440` (read-only, prevents accidental modification)

**Sudoers Content:**
```
username ALL=(ALL) NOPASSWD:ALL
```

This allows users to run any command with sudo without entering a password.

### SSH Access

**Keys Source:** GitHub account (`https://github.com/<github_username>.keys`)
**Location:** `/home/<username>/.ssh/authorized_keys`
**Permissions:** `0600` (read/write for user only)
**Directory Permissions:** `0700` (full access for user only)

### Home Directory

**Location:** `/home/<username>`
**Permissions:** Default `useradd -m` permissions
**Shell:** `/bin/bash`

## Generated User Setup Script

For a user `admin` with GitHub username `gherlein`, the generated script is:

```bash
#!/bin/bash
set -e

# Auto-generated user setup script

# Create user: admin (GitHub: gherlein)
useradd -m -s /bin/bash "admin" || true
usermod -a -G sudo,www-data admin
echo 'admin ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/admin
chmod 0440 /etc/sudoers.d/admin
mkdir -p /home/admin/.ssh
chmod 700 /home/admin/.ssh
curl -s https://github.com/gherlein.keys > /home/admin/.ssh/authorized_keys
chmod 600 /home/admin/.ssh/authorized_keys
chown -R admin:admin /home/admin/.ssh
echo 'User admin created with SSH keys from GitHub (gherlein)'
```

## Verification

### Check User Setup

```bash
# SSH into instance
ssh admin@hostname.example.com

# Verify groups
$ groups
admin sudo www-data

# Verify sudo access
$ sudo -l
User admin may run the following commands on hostname:
    (ALL) NOPASSWD: ALL

# Test sudo (no password prompt)
$ sudo whoami
root

# Verify SSH key
$ cat ~/.ssh/authorized_keys
ssh-rsa AAAAB3NzaC1yc2EA... (your GitHub key)
```

### Check File Permissions

```bash
# Sudoers file
$ ls -la /etc/sudoers.d/admin
-r--r----- 1 root root 39 Jan 12 23:00 /etc/sudoers.d/admin

$ sudo cat /etc/sudoers.d/admin
admin ALL=(ALL) NOPASSWD:ALL

# SSH directory
$ ls -la ~/.ssh
drwx------ admin admin .ssh

$ ls -la ~/.ssh/authorized_keys
-rw------- admin admin authorized_keys

# Web directory (group writable)
$ ls -la /var/www/
drwxrwsr-x www-data www-data html
```

## Web Deployment Access

### How It Works

1. **Directory Ownership:**
   - `/var/www/html` owned by `www-data:www-data`

2. **Directory Permissions:**
   - `2775` = `rwxrwsr-x`
   - `2` = setgid bit (new files inherit www-data group)
   - `7` = owner full access (rwx)
   - `7` = group full access (rwx)
   - `5` = other read+execute only (r-x)

3. **User Group:**
   - Users are in `www-data` group
   - Can read/write/execute in `/var/www/html`

4. **File Creation:**
   - New files inherit `www-data` group (setgid bit)
   - Users can modify each other's files

### Testing Deployment Access

```bash
# As admin user
$ cd /var/www/html
$ echo "Admin's file" > admin-test.html
$ ls -l admin-test.html
-rw-rw-r-- admin www-data admin-test.html

# As another user (dev)
$ cd /var/www/html
$ echo "Dev's file" > dev-test.html
$ ls -l dev-test.html
-rw-rw-r-- dev www-data dev-test.html

# Both can modify each other's files
$ echo "modified" >> admin-test.html  # ✓ Works
$ rm dev-test.html  # ✓ Works
```

## Multi-User Scenarios

### Two Users

**Config:**
```json
{
  "users": [
    {"username": "alice", "github_username": "alice-gh"},
    {"username": "bob", "github_username": "bob-gh"}
  ]
}
```

**Result:**

Both users have:
- Full sudo access without password
- Write access to `/var/www/html`
- Can deploy via `make deploy`
- Can reload Caddy
- Can read/modify each other's files in web directory

**Collaboration:**
```bash
# Alice deploys
alice$ rsync -avz build/ /var/www/html/
alice$ sudo systemctl reload caddy

# Bob can modify
bob$ cd /var/www/html
bob$ vim index.html
bob$ sudo systemctl reload caddy
```

### Team Server

**Config:**
```json
{
  "users": [
    {"username": "lead", "github_username": "team-lead"},
    {"username": "dev1", "github_username": "developer1"},
    {"username": "dev2", "github_username": "developer2"},
    {"username": "ops", "github_username": "operations"}
  ]
}
```

All four users can:
- Deploy independently
- Collaborate on files
- Manage services
- No permission conflicts

## Security Implications

### Full Sudo Access

**Granted:**
```
username ALL=(ALL) NOPASSWD:ALL
```

**This Means:**
- ✅ Can run any command as any user
- ✅ Can modify system files
- ✅ Can install/remove software
- ✅ Can change file permissions
- ✅ Can modify network settings
- ✅ Can stop/start services
- ✅ No password required

**Appropriate For:**
- Development environments
- Personal servers
- Testing/CI instances
- Trusted team members
- Short-lived instances

**NOT Appropriate For:**
- Production servers with sensitive data
- Servers accessible from untrusted networks
- Compliance-regulated environments
- Long-lived infrastructure
- Servers with multiple untrusted users

### Restricting Access

#### Option 1: Limit Sudo Commands

Edit `/etc/sudoers.d/username`:

```
# Limited sudo - only specific commands
username ALL=(ALL) NOPASSWD: /bin/systemctl reload caddy
username ALL=(ALL) NOPASSWD: /bin/systemctl status caddy
username ALL=(ALL) NOPASSWD: /usr/bin/apt-get update
username ALL=(ALL) NOPASSWD: /usr/bin/apt-get upgrade
```

#### Option 2: Require Password

```
# Require password for all sudo
username ALL=(ALL) ALL

# Set password
sudo passwd username
```

#### Option 3: Remove Sudo Access

```
# Remove from sudo group
sudo deluser username sudo

# Remove sudoers file
sudo rm /etc/sudoers.d/username
```

User keeps www-data group for deployment access.

#### Option 4: Use Deployment Keys

For CI/CD, create a deploy-only user:

```json
{
  "users": [
    {"username": "deploy", "github_username": "bot-account"}
  ]
}
```

Then restrict after creation:
```bash
# Remove sudo access
sudo rm /etc/sudoers.d/deploy
sudo deluser deploy sudo

# Deploy user only has www-data group
groups deploy
# Output: deploy www-data
```

## Best Practices

### Development Servers

✅ Use default permissions (sudo + www-data)
- Fast iteration
- No friction
- Easy collaboration

### Production Servers

❌ Don't use default permissions
- Restrict sudo to specific commands
- Use configuration management tools
- Implement audit logging
- Consider read-only deployments

### CI/CD Servers

⚠️ Use deployment-specific users
- Create separate user for automation
- Restrict sudo if possible
- Use SSH key rotation
- Monitor access logs

### Personal Servers

✅ Default permissions are fine
- You trust yourself
- Convenience matters
- Easy to manage

## Comparison with Other Tools

### Traditional Setup

**Manual Steps:**
```bash
# Create user
sudo useradd -m -s /bin/bash alice

# Set password
sudo passwd alice

# Add to sudo group
sudo usermod -a -G sudo alice

# Configure sudoers
sudo visudo -f /etc/sudoers.d/alice

# Add SSH key manually
sudo mkdir -p /home/alice/.ssh
sudo vim /home/alice/.ssh/authorized_keys
sudo chown -R alice:alice /home/alice/.ssh
sudo chmod 700 /home/alice/.ssh
sudo chmod 600 /home/alice/.ssh/authorized_keys

# Add to www-data
sudo usermod -a -G www-data alice
```

**This Tool:**
```bash
# All of the above in one step
./bin/ec2 -c -n myserver
```

### Ansible/Chef/Puppet

Configuration management tools offer:
- ✅ More granular control
- ✅ Idempotent operations
- ✅ Complex policy management

This tool offers:
- ✅ Zero configuration
- ✅ Instant results
- ✅ GitHub integration
- ✅ Perfect for quick instances

## Summary

**Default User Configuration:**

```
User: username
├─ Shell: /bin/bash
├─ Home: /home/username
├─ Groups: sudo, www-data
├─ Sudo: NOPASSWD:ALL
└─ SSH Keys: From GitHub
```

**Capabilities:**

✅ Full system administration
✅ Web content deployment
✅ Service management
✅ Package installation
✅ Configuration changes
✅ File system access
✅ Network management

**Trade-offs:**

✅ **Pros:** Instant productivity, zero friction, team-friendly
⚠️ **Cons:** High privilege level, requires trust, not for untrusted users

The default configuration prioritizes convenience for development and testing. Adjust permissions based on your security requirements.
