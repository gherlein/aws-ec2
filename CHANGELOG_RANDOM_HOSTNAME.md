# Changelog: Random Hostname & User Permissions

## Summary

Added automatic random hostname generation and improved user permissions setup.

## Changes

### 1. Random Hostname Generation

**Feature:** Automatically generate random 8-character hostname when not specified in config.

**File:** `main.go` (lines 189-203, 1052-1061)

**Implementation:**

```go
func generateRandomHostname() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 8
	result := make([]byte, length)

	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Fatalf("failed to generate random hostname: %v", err)
		}
		result[i] = charset[num.Int64()]
	}

	return string(result)
}
```

**Trigger Logic:**

```go
// Generate random hostname if not specified (helps avoid Let's Encrypt rate limits)
if stackCfg.Hostname == "" && stackCfg.Domain != "" {
	stackCfg.Hostname = generateRandomHostname()
	fmt.Printf("Generated random hostname: %s\n", stackCfg.Hostname)

	// Save the generated hostname back to config
	if err := writeConfig(configFile, stackCfg); err != nil {
		log.Fatalf("failed to save generated hostname to config: %v", err)
	}
}
```

**Behavior:**
- Only triggers if `hostname` is empty AND `domain` is specified
- Uses cryptographically secure random number generator (`crypto/rand`)
- Generates 8 characters from `a-z` and `0-9` (DNS-safe charset)
- Saves generated hostname back to JSON config file
- Printed to console: `Generated random hostname: x3k9m2a7`

**Example Config:**

Before deployment:
```json
{
  "hostname": "",
  "domain": "example.com"
}
```

After deployment:
```json
{
  "hostname": "x3k9m2a7",
  "domain": "example.com"
}
```

**Use Cases:**
1. **Rapid Testing**: Create/delete cycles without hostname conflicts
2. **Let's Encrypt Rate Limits**: Avoid the 5 certificates per domain per week limit
3. **Throwaway Instances**: Quick testing without managing hostnames
4. **CI/CD Pipelines**: Automated testing environments

**Let's Encrypt Rate Limits:**

Without random hostnames:
- Creating `test.example.com` multiple times hits rate limits
- Limit: 5 duplicate certificates per week per domain

With random hostnames:
- Each hostname is unique: `a3k5m7.example.com`, `x9p2q4.example.com`
- No conflicts, no rate limit issues
- Can create/delete rapidly for testing

### 2. Enhanced User Permissions

**Feature:** Add all created users to both `sudo` and `www-data` groups with proper NOPASSWD sudo configuration.

**File:** `main.go` (lines 656-661)

**Changes:**

**Before:**
```go
script.WriteString(fmt.Sprintf("usermod -a -G www-data %s\n", user.Username))
script.WriteString(fmt.Sprintf("echo %q ' ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/%s\n", user.Username, user.Username))
```

**After:**
```go
script.WriteString(fmt.Sprintf("usermod -a -G sudo,www-data %s\n", user.Username))
script.WriteString(fmt.Sprintf("echo '%s ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/%s\n", user.Username, user.Username))
script.WriteString(fmt.Sprintf("chmod 0440 /etc/sudoers.d/%s\n", user.Username))
```

**Improvements:**

1. **Sudo Group Membership:**
   - Users now added to `sudo` group (in addition to `www-data`)
   - Provides standard Linux sudo group membership
   - Consistent with Ubuntu/Debian conventions

2. **Fixed Sudoers Permissions:**
   - Added `chmod 0440` to sudoers file
   - Prevents accidental modification
   - Follows security best practices
   - Silences sudo warnings about file permissions

3. **Simplified String Formatting:**
   - Changed from `%q` (quoted) to `%s` (plain)
   - Removed extra quotes that could cause issues
   - Cleaner sudoers file output

**Generated Script:**

```bash
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

**Result:**

```bash
# Check user groups
$ groups
admin sudo www-data

# Verify sudo access
$ sudo -l
User admin may run the following commands on hostname:
    (ALL) NOPASSWD: ALL

# Check sudoers file permissions
$ ls -la /etc/sudoers.d/admin
-r--r----- 1 root root 39 Jan 12 23:00 /etc/sudoers.d/admin
```

**Permissions Breakdown:**

| Group | Purpose | Access |
|-------|---------|--------|
| `sudo` | Administrative access | Full system control via sudo |
| `www-data` | Web deployment | Write access to `/var/www/html` |

### 3. Documentation Updates

**Files Updated:**
- `README.md`
- `example-random-hostname.json` (new)

**README Changes:**

1. **Quick Start Section** (lines 138-150):
   - Added tip about random hostname generation
   - Example config with empty hostname
   - Explanation of generated output

2. **How It Works Section** (lines 368-373):
   - Updated user creation details
   - Added `sudo` and `www-data` group mentions
   - Clarified sudoers configuration

3. **DNS Integration Section** (lines 382-403):
   - Added "Random Hostname Generation" subsection
   - Detailed explanation of feature
   - Use cases and benefits
   - Let's Encrypt rate limit explanation

4. **Security Considerations** (lines 449-460):
   - Expanded sudo access details
   - Added group memberships section
   - Clarified sudoers file permissions
   - Noted production considerations

**New Example File:**

`example-random-hostname.json` - Demonstrates random hostname feature with:
- Empty hostname field
- Domain specified
- Cloud-init and packages configured
- Ready to use for testing

## Usage Examples

### Random Hostname Deployment

```bash
# Create config with empty hostname
cat > stacks/test.json << 'EOF'
{
  "hostname": "",
  "domain": "example.com",
  "users": [{"username": "admin", "github_username": "gherlein"}],
  "instance_type": "t3.micro"
}
EOF

# Deploy - hostname generated automatically
./bin/ec2 -c -n test
# Output: Generated random hostname: k3m9x2a7

# Config now contains:
cat stacks/test.json
{
  "hostname": "k3m9x2a7",
  "domain": "example.com",
  ...
}

# Connect via generated hostname
ssh admin@k3m9x2a7.example.com

# Delete when done
./bin/ec2 -d -n test
```

### Rapid Testing Workflow

```bash
# Test deployment 1
./bin/ec2 -c -n test
# Generated: a7k3m9.example.com
./bin/ec2 -d -n test

# Test deployment 2
./bin/ec2 -c -n test
# Generated: x9p2q4.example.com
./bin/ec2 -d -n test

# Test deployment 3
./bin/ec2 -c -n test
# Generated: m5n8r2.example.com
./bin/ec2 -d -n test

# No rate limit issues! Each hostname is unique
```

### User Permissions Verification

```bash
# SSH into instance
ssh admin@k3m9x2a7.example.com

# Check groups
$ groups
admin sudo www-data

# Test sudo access
$ sudo whoami
root

# Test web deployment
$ echo "test" > /var/www/html/test.txt
$ curl http://localhost/test.txt
test

# Check sudoers file
$ cat /etc/sudoers.d/admin
admin ALL=(ALL) NOPASSWD:ALL

$ ls -la /etc/sudoers.d/admin
-r--r----- 1 root root 39 Jan 12 23:00 /etc/sudoers.d/admin
```

## Migration

### Existing Deployments

No impact on existing stacks. Changes only affect new deployments.

### Existing Configs

Configs with explicit hostnames continue to work:

```json
{
  "hostname": "myserver",
  "domain": "example.com"
}
```

To use random hostnames, change to:

```json
{
  "hostname": "",
  "domain": "example.com"
}
```

### User Permissions

Newly created instances will have users with:
- ✅ `sudo` group membership
- ✅ `www-data` group membership
- ✅ Properly configured sudoers files (0440 permissions)

Existing instances are not affected. To update existing users:

```bash
ssh user@server
sudo usermod -a -G sudo $USER
sudo chmod 0440 /etc/sudoers.d/$USER
```

## Testing

Build and test:

```bash
cd ~/herlein/src/er/aws-ec2
make build

# Test random hostname generation
echo '{"hostname":"","domain":"example.com","users":[{"username":"test","github_username":"gherlein"}]}' > test-random.json
./bin/ec2 -c -n test-random

# Verify hostname was generated and saved
cat stacks/test-random.json | jq .hostname
# Output: "x3k9m2a7" (or similar)

# Verify user permissions on instance
ssh test@<generated-hostname>.example.com "groups && sudo -l"
# Output:
# test sudo www-data
# User test may run the following commands:
#     (ALL) NOPASSWD: ALL

# Clean up
./bin/ec2 -d -n test-random
```

## Security Considerations

### Random Hostnames

**Security:** Cryptographically secure random generation using `crypto/rand`
- Not predictable
- Suitable for production use
- DNS-safe characters only

**Privacy:** Hostnames are not obfuscation:
- Still publicly visible in DNS
- Not a security mechanism
- Use for rate limit avoidance, not hiding

### Sudo Access

**Development/Testing:** Passwordless sudo is convenient
- Quick iteration
- No password prompts
- Simplified automation

**Production:** Consider restrictions:
- Specific sudo commands only
- Require password for sudo
- Use configuration management (Ansible, Chef, Puppet)
- Implement audit logging

**Example Restricted Sudo:**

```bash
# /etc/sudoers.d/admin-restricted
admin ALL=(ALL) PASSWD: ALL
admin ALL=(ALL) NOPASSWD: /bin/systemctl reload caddy
admin ALL=(ALL) NOPASSWD: /bin/systemctl status caddy
```

## Benefits

### Random Hostnames

1. **No Rate Limits**: Unique hostname per deployment
2. **Clean Testing**: No hostname conflicts
3. **Automation-Friendly**: Works in CI/CD pipelines
4. **Zero Configuration**: Automatic with empty hostname

### Enhanced User Permissions

1. **Standard Compliance**: Follows Ubuntu/Debian conventions
2. **Web Deployment**: Users can deploy without additional setup
3. **No Warnings**: Proper file permissions prevent sudo warnings
4. **Clear Intent**: Explicit group memberships

## Backward Compatibility

✅ Fully backward compatible
- Existing configs work unchanged
- Empty hostname is opt-in feature
- User creation enhanced but compatible
- No breaking changes

## Summary

Two key improvements:

**1. Random Hostname Generation**
- Empty hostname → automatic 8-character generation
- Saves back to config file
- Avoids Let's Encrypt rate limits
- Perfect for rapid testing

**2. Enhanced User Permissions**
- Users added to `sudo` + `www-data` groups
- Proper sudoers file permissions (0440)
- Passwordless sudo configured correctly
- Ready for deployment immediately

Both features work together for seamless testing and deployment workflows.
