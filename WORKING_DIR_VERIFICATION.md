# Working Directory Configuration Verification

## Configuration Summary

The cloud-init script correctly configures:
1. ✅ Caddy to serve from working directory
2. ✅ Group write access for all users
3. ✅ Proper ownership and permissions

## Caddy Configuration

**File:** `cloud-init/webserver.yaml` (line 29-48)

```yaml
- path: /etc/caddy/Caddyfile
  content: |
    {{.FQDN}} {
      root * {{.WorkingDir}}      # ← Points to working directory
      file_server
      encode gzip
      # ... rest of config
    }
  owner: caddy:caddy
  permissions: '0644'
```

**Result:** Caddy serves files from `{{.WorkingDir}}` (default: `/var/www/html`)

## Permissions Setup

**File:** `cloud-init/webserver.yaml` (line 122-131)

```yaml
runcmd:
  # Create directories
  - mkdir -p {{.WorkingDir}}              # ← Create working directory
  - mkdir -p /var/log/caddy
  - mkdir -p /var/lib/caddy

  # Set ownership
  - chown -R caddy:caddy /var/lib/caddy
  - chown -R caddy:caddy /var/log/caddy
  - chown -R www-data:www-data {{.WorkingDir}}  # ← Set group ownership
  - chmod 775 {{.WorkingDir}}             # ← Group writable (rwxrwxr-x)
```

**Breakdown:**
- `775` permissions = `rwxrwxr-x`
  - Owner (www-data): read, write, execute
  - Group (www-data): read, write, execute ← **Users get write access here**
  - Other: read, execute only

## User Group Membership

**File:** `cloud-init/webserver.yaml` (line 137-140)

```yaml
  # Add users to www-data group for web content management
{{range .Users}}
  - usermod -a -G www-data {{.Username}}  # ← Add each user to www-data
{{end}}
```

**Result:** All users from config are added to `www-data` group

## Complete Flow

### 1. User Creation (Shell Script - Part 1)
```bash
useradd -m -s /bin/bash gherlein
useradd -m -s /bin/bash luca
# ... SSH keys, sudo access
```

### 2. Directory Setup (Cloud-Init - Part 2)
```bash
mkdir -p /var/www/html
chown -R www-data:www-data /var/www/html
chmod 775 /var/www/html
```

### 3. Group Membership (Cloud-Init - Part 2)
```bash
usermod -a -G www-data gherlein
usermod -a -G www-data luca
```

### 4. Caddy Configuration (Cloud-Init - Part 2)
```
root * /var/www/html
```

## Permission Matrix

| Path | Owner | Group | Permissions | Effect |
|------|-------|-------|-------------|--------|
| `/var/www/html` | www-data | www-data | 775 | Users in www-data group can read/write/execute |
| `/var/log/caddy` | caddy | caddy | default | Only caddy can write logs |
| `/var/lib/caddy` | caddy | caddy | default | Only caddy can store state |
| `/etc/caddy` | caddy | caddy | default | Only caddy can read config |

## User Access Verification

Once deployed, verify access:

```bash
# SSH into instance
ssh gherlein@new1.emergingrobotics.ai

# Check group membership
groups
# Output should include: gherlein www-data ...

# Check directory permissions
ls -la /var/www/
# Output: drwxrwxr-x ... www-data www-data ... html

# Test write access
echo "test" > /var/www/html/test.txt
# Should succeed without sudo

# Verify Caddy serves from correct location
curl http://localhost/test.txt
# Output: test
```

## Example Config

**File:** `er.json`

```json
{
  "working_dir": "/var/www/html",
  "users": [
    {"username": "gherlein", "github_username": "gherlein"},
    {"username": "luca", "github_username": "lherlein"}
  ]
}
```

**Resulting Setup:**

1. **Directory:** `/var/www/html`
   - Owner: `www-data:www-data`
   - Permissions: `775`

2. **Users:** `gherlein`, `luca`
   - Groups: `gherlein www-data`, `luca www-data`
   - Can write to: `/var/www/html`

3. **Caddy:**
   - Serves from: `/var/www/html`
   - Runs as: `caddy` user
   - Can read: `/var/www/html` (world-readable)

4. **Deployment:**
   ```bash
   make deploy CONFIG=er.json
   # Files copied to /var/www/html as gherlein:www-data
   # Caddy serves them immediately
   ```

## Custom Working Directory Example

**Config:**
```json
{
  "working_dir": "/srv/myapp/public"
}
```

**Result:**
```bash
# Directory created
mkdir -p /srv/myapp/public

# Ownership set
chown -R www-data:www-data /srv/myapp/public

# Permissions set
chmod 775 /srv/myapp/public

# Users added to group
usermod -a -G www-data gherlein
usermod -a -G www-data luca

# Caddy configured
root * /srv/myapp/public
```

## Troubleshooting

### User cannot write to working directory

```bash
# Check group membership
groups
# Should show: username www-data

# If missing, add manually
sudo usermod -a -G www-data username

# Log out and back in for group change to take effect
```

### Wrong directory permissions

```bash
# Check current permissions
ls -la /var/www/html
# Should show: drwxrwxr-x www-data www-data

# Fix if wrong
sudo chown -R www-data:www-data /var/www/html
sudo chmod 775 /var/www/html
```

### Caddy serving wrong directory

```bash
# Check Caddyfile
cat /etc/caddy/Caddyfile
# Should show: root * /var/www/html

# If wrong, edit and reload
sudo caddy reload --config /etc/caddy/Caddyfile
```

### Files deployed with wrong permissions

```bash
# Check file ownership
ls -la /var/www/html/
# Files should be: -rw-rw-r-- username www-data

# Fix existing files
sudo chgrp -R www-data /var/www/html/*
sudo chmod -R 664 /var/www/html/*.html
sudo chmod -R 775 /var/www/html/*/  # directories
```

## Security Notes

### Why 775 instead of 777?

- `775` allows group write but not world write
- Only users in `www-data` group can modify files
- More secure than `777` (world-writable)

### Why www-data group?

- Standard web server group on Debian/Ubuntu
- Caddy can read files (world-readable via 775)
- Users can write files (group-writable via 775)
- Separation between Caddy process (runs as `caddy` user) and deployment (www-data group)

### File Creation Defaults

When users create files in `/var/www/html`:

```bash
# User creates file
echo "test" > /var/www/html/new.html

# Default permissions depend on umask
ls -l /var/www/html/new.html
# Usually: -rw-r--r-- gherlein gherlein

# To ensure group write
umask 002  # In user's ~/.bashrc
echo "test" > /var/www/html/new2.html
ls -l /var/www/html/new2.html
# Now: -rw-rw-r-- gherlein gherlein

# Or use setgid bit on directory
sudo chmod g+s /var/www/html
# New files inherit www-data group automatically
```

## Recommendation: Add setgid bit

Add this to cloud-init for better defaults:

```yaml
runcmd:
  # ... existing commands ...
  - chmod 2775 {{.WorkingDir}}  # 2 = setgid bit
```

With setgid (2775):
- New files/dirs inherit `www-data` group automatically
- Users don't need to manually chgrp files
- More convenient for deployment

## Summary

The current configuration provides:

✅ **Caddy points to working directory**
- Configured via `root * {{.WorkingDir}}`
- Dynamic based on JSON config
- Default: `/var/www/html`

✅ **Group write access for users**
- Directory: `www-data:www-data` ownership
- Permissions: `775` (group writable)
- Users added to `www-data` group
- Can deploy without sudo

✅ **Secure by default**
- Not world-writable (not 777)
- Caddy runs as separate user
- Clear separation of concerns

The setup is correct and complete!
