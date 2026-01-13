# Configuration Examples

This directory contains several example configuration files demonstrating different use cases for the EC2 instance manager.

## Example Files

### example.json
**Default example with multiple users and all DNS features**

Shows the complete feature set:
- Multiple users with different local and GitHub usernames
- Primary hostname A record
- CNAME aliases
- Apex domain support

```json
{
  "users": [
    {"username": "admin", "github_username": "your-github-username"},
    {"username": "developer", "github_username": "another-github-username"}
  ],
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "is_apex_domain": false,
  "cname_aliases": ["api", "staging"]
}
```

**DNS Records Created:**
- A: `dev.example.com -> <IP>`
- CNAME: `api.example.com -> dev.example.com`
- CNAME: `staging.example.com -> dev.example.com`

**Users Created:**
- `admin` (keys from `your-github-username`)
- `developer` (keys from `another-github-username`)

**Access:**
```bash
ssh admin@dev.example.com
ssh admin@api.example.com
ssh developer@staging.example.com
```

---

### example-legacy.json
**Backward compatible single-user configuration**

Uses the original `github_username` field (deprecated but supported):

```json
{
  "github_username": "your-github-username",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

**DNS Records Created:**
- A: `dev.example.com -> <IP>`

**Users Created:**
- `your-github-username` (keys from `your-github-username`)

**Access:**
```bash
ssh your-github-username@dev.example.com
```

**Migration Note:** This format still works but consider migrating to the `users` array for consistency and flexibility.

---

### example-single-user.json
**Single user with new format**

Demonstrates the new `users` array with a single user:

```json
{
  "users": [
    {"username": "admin", "github_username": "your-github-username"}
  ],
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

**DNS Records Created:**
- A: `dev.example.com -> <IP>`

**Users Created:**
- `admin` (keys from `your-github-username`)

**Access:**
```bash
ssh admin@dev.example.com
```

**Use Case:** When you want a different local username than your GitHub username (e.g., `admin` instead of your GitHub handle).

---

### example-apex-domain.json
**Apex domain configuration**

Makes the instance respond to both `www.example.com` and `example.com`:

```json
{
  "users": [
    {"username": "admin", "github_username": "your-github-username"}
  ],
  "instance_type": "t3.micro",
  "hostname": "www",
  "domain": "example.com",
  "is_apex_domain": true
}
```

**DNS Records Created:**
- A: `www.example.com -> <IP>`
- A: `example.com -> <IP>`

**Users Created:**
- `admin` (keys from `your-github-username`)

**Access:**
```bash
ssh admin@www.example.com
ssh admin@example.com
```

**Use Case:** Web servers where you want both the www subdomain and bare domain to resolve to the same instance.

---

### example-full-featured.json
**All features enabled**

Demonstrates the complete feature set with multiple users, CNAME aliases, and apex domain:

```json
{
  "users": [
    {"username": "greg", "github_username": "gherlein"},
    {"username": "alice", "github_username": "alice-dev"},
    {"username": "bob", "github_username": "bob-admin"}
  ],
  "instance_type": "t3.micro",
  "hostname": "app",
  "domain": "example.com",
  "is_apex_domain": true,
  "cname_aliases": ["api", "staging", "web"]
}
```

**DNS Records Created:**
- A: `app.example.com -> <IP>`
- CNAME: `api.example.com -> app.example.com`
- CNAME: `staging.example.com -> app.example.com`
- CNAME: `web.example.com -> app.example.com`
- A: `example.com -> <IP>`

**Users Created:**
- `greg` (keys from `gherlein`)
- `alice` (keys from `alice-dev`)
- `bob` (keys from `bob-admin`)

**Access (all combinations work):**
```bash
# Any user can access via any DNS name
ssh greg@app.example.com
ssh alice@api.example.com
ssh bob@staging.example.com
ssh greg@web.example.com
ssh alice@example.com
```

**Use Case:** Development team server with multiple team members and multiple DNS aliases for different purposes (API testing, staging environment, etc.).

---

### example-no-dns.json
**Multiple users without DNS**

Creates an instance with multiple users but no DNS configuration:

```json
{
  "users": [
    {"username": "admin", "github_username": "your-github-username"},
    {"username": "developer", "github_username": "dev-github-username"}
  ],
  "instance_type": "t3.micro"
}
```

**DNS Records Created:** None (access via IP only)

**Users Created:**
- `admin` (keys from `your-github-username`)
- `developer` (keys from `dev-github-username`)

**Access:**
```bash
# After creation, use the public IP from the output
ssh admin@54.184.71.168
ssh developer@54.184.71.168
```

**Use Case:** Quick temporary instances where DNS isn't needed, or when you don't have a Route53 hosted zone configured.

---

## Quick Start

1. **Choose an example** that matches your use case
2. **Copy it to the stacks directory**:
   ```bash
   cp example-single-user.json stacks/myserver.json
   ```
3. **Edit with your values**:
   ```bash
   vi stacks/myserver.json
   ```
4. **Create the stack**:
   ```bash
   ./bin/ec2 -c -n myserver
   ```

## Configuration Field Reference

### Input Fields (You Configure)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `users` | array | Conditional | - | Array of user objects to create |
| `github_username` | string | Conditional | - | Legacy single-user format (deprecated) |
| `instance_type` | string | No | `t3.micro` | EC2 instance type |
| `hostname` | string | No | - | DNS hostname without domain |
| `domain` | string | No | - | Domain name for Route53 |
| `ttl` | number | No | `300` | DNS record TTL in seconds |
| `is_apex_domain` | boolean | No | `false` | Create A record for bare domain |
| `cname_aliases` | array | No | `[]` | Additional CNAME aliases |

**Note:** Either `users` OR `github_username` must be specified.

### User Object Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | Linux username to create on the host |
| `github_username` | string | Yes | GitHub username to fetch SSH keys from |

### Output Fields (Auto-Filled)

| Field | Description |
|-------|-------------|
| `stack_name` | CloudFormation stack name |
| `stack_id` | CloudFormation stack ARN |
| `region` | AWS region |
| `instance_id` | EC2 instance ID |
| `public_ip` | Public IPv4 address |
| `security_group` | Security group ID |
| `zone_id` | Route53 hosted zone ID |
| `fqdn` | Fully qualified domain name |
| `ssh_command` | Ready-to-use SSH command |
| `dns_records` | Array of created DNS records |

## Common Patterns

### Pattern 1: Development Server for Team
Use `example-full-featured.json` as a starting point. Create multiple users for team members and use CNAMEs for different purposes (api, staging, admin).

### Pattern 2: Production Web Server
Use `example-apex-domain.json` to ensure both `www.domain.com` and `domain.com` work.

### Pattern 3: Personal Dev Box
Use `example-single-user.json` with your preferred username and GitHub account.

### Pattern 4: Quick Temporary Instance
Use `example-no-dns.json` for fast provisioning without DNS setup.

### Pattern 5: Random Hostname for Testing
Use `example-random-hostname.json` with empty hostname to avoid Let's Encrypt rate limits:
```json
{
  "hostname": "",
  "domain": "example.com"
}
```
The tool generates a unique 8-character hostname like `k3m9x2a7.example.com` automatically. Perfect for rapid create/delete testing cycles.

### Pattern 6: Multi-Environment Server
Use CNAMEs to create logical names:
```json
{
  "hostname": "server",
  "cname_aliases": ["dev", "staging", "test", "demo"]
}
```

## DNS Configuration Explained

### CNAME Aliases
CNAMEs create additional hostnames that point to your primary hostname:
```
Primary:  app.example.com -> 54.184.71.168 (A record)
Alias 1:  api.example.com -> app.example.com (CNAME)
Alias 2:  web.example.com -> app.example.com (CNAME)
```

All three names resolve to the same IP, but CNAMEs are easier to update if you recreate the instance.

### Apex Domain
The apex domain is the bare domain without any subdomain:
```
Subdomain: www.example.com
Apex:      example.com
```

Setting `is_apex_domain: true` creates an A record for the apex so both work.

**Important:** You cannot use a CNAME for the apex domain (DNS limitation). The tool automatically uses an A record.

### DNS Record Cleanup
All created DNS records are tracked in the `dns_records` output field. When you delete the stack, all records are automatically cleaned up.

## Validation Rules

The tool validates your configuration before creating resources:

1. **At least one user required**: Must specify either `users` or `github_username`
2. **Unique usernames**: No duplicate usernames in the `users` array
3. **Valid username format**: Must be lowercase alphanumeric, start with letter
4. **DNS requires domain**: If using `hostname`, `cname_aliases`, or `is_apex_domain`, you must specify `domain`
5. **CNAMEs require hostname**: Cannot use `cname_aliases` without a primary `hostname`
6. **No empty values**: Usernames and GitHub usernames cannot be empty strings

## Troubleshooting

### "at least one user required"
You forgot to specify either `github_username` or `users` in your config.

**Fix:** Add a user:
```json
{
  "users": [{"username": "admin", "github_username": "your-github-username"}]
}
```

### "invalid username format"
Linux usernames must be lowercase alphanumeric, start with a letter.

**Bad:** `Admin`, `1user`, `user@host`
**Good:** `admin`, `user1`, `my-user`

### "cname_aliases requires both hostname and domain"
You specified CNAME aliases but didn't provide the primary hostname.

**Fix:** Add both fields:
```json
{
  "hostname": "app",
  "domain": "example.com",
  "cname_aliases": ["api"]
}
```

### "hosted zone not found for domain"
The domain doesn't exist in your Route53 hosted zones.

**Fix:** Create the hosted zone in Route53 first, or omit DNS fields to use IP-only access.

### SSH connection times out
The instance might still be starting up. Wait 1-2 minutes for cloud-init to complete user creation.

Check cloud-init logs via EC2 Instance Connect:
```bash
sudo cat /var/log/cloud-init-output.log
```

## Cost Considerations

### Free Tier Eligible
These instance types are free tier eligible (750 hours/month for 12 months):
- `t3.micro` (default) - 2 vCPU, 1 GB RAM
- `t3.small` - 2 vCPU, 2 GB RAM

### DNS Costs
Route53 charges:
- $0.50 per hosted zone per month
- $0.40 per million queries (first billion queries)

For development, DNS costs are typically under $1/month.

### Instance Costs
After free tier or for larger instances:
- `t3.micro`: ~$0.01/hour ($7.50/month)
- `t3.small`: ~$0.02/hour ($15/month)
- `t3.medium`: ~$0.04/hour ($30/month)

Remember to delete stacks when not in use:
```bash
./bin/ec2 -d -n myserver
```
