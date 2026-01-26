# DNS-Only Mode Guide

## Overview

The EC2 instance manager now supports DNS-only mode, allowing you to manage Route53 DNS records without creating EC2 instances.

## Quick Start

### Create DNS for External Server

```bash
# Create config
cat > stacks/external.json << 'EOF'
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "app",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": true,
    "cname_aliases": ["www", "api"]
  }
}
EOF

# Create DNS records
./bin/ec2 -c -n external

# Verify
dig +short app.example.com
# Output: 203.0.113.10

# Delete when done
./bin/ec2 -d -n external
```

## Configuration Modes

### Mode 1: DNS-Only

**When to use:** Point domain to existing infrastructure (DigitalOcean, existing EC2, load balancer, etc.)

**Config:**
```json
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "app",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
```

**Creates:**
- A: `app.example.com → 203.0.113.10`
- A: `example.com → 203.0.113.10`
- CNAME: `www.example.com → app.example.com`

**No EC2 instance created.**

### Mode 2: VM-Only

**When to use:** Create EC2 instance without DNS, access via IP

**Config:**
```json
{
  "vm": {
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "users": [
      {"username": "admin", "github_username": "gherlein"}
    ]
  }
}
```

**Creates:**
- EC2 instance (t3.micro by default)
- Security group with SSH access

**No DNS records created.**
**Access:** `ssh admin@<IP>`

### Mode 3: Full Stack (VM + DNS)

**When to use:** Create EC2 instance with DNS records

**Config:**
```json
{
  "vm": {
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "users": [
      {"username": "admin", "github_username": "gherlein"}
    ],
    "cloud_init_file": "cloud-init/webserver.yaml"
  },
  "dns": {
    "hostname": "app",
    "domain": "example.com",
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
```

**Creates:**
- EC2 instance
- DNS records pointing to EC2's public IP

**Note:** `dns.target_ip` is automatically set to VM's public IP

## DNS-Only Examples

### Example 1: Point Domain to DigitalOcean Droplet

```json
{
  "dns": {
    "target_ip": "167.99.123.45",
    "hostname": "web",
    "domain": "mysite.com",
    "ttl": 300
  }
}
```

**Result:**
- A: `web.mysite.com → 167.99.123.45`

### Example 2: Apex Domain + WWW

```json
{
  "dns": {
    "target_ip": "203.0.113.50",
    "hostname": "www",
    "domain": "mysite.com",
    "ttl": 300,
    "is_apex_domain": true
  }
}
```

**Result:**
- A: `www.mysite.com → 203.0.113.50`
- A: `mysite.com → 203.0.113.50`

### Example 3: Multiple Aliases

```json
{
  "dns": {
    "target_ip": "198.51.100.10",
    "hostname": "main",
    "domain": "mysite.com",
    "cname_aliases": ["www", "api", "cdn", "static"]
  }
}
```

**Result:**
- A: `main.mysite.com → 198.51.100.10`
- CNAME: `www.mysite.com → main.mysite.com`
- CNAME: `api.mysite.com → main.mysite.com`
- CNAME: `cdn.mysite.com → main.mysite.com`
- CNAME: `static.mysite.com → main.mysite.com`

### Example 4: Random Hostname Testing

```json
{
  "dns": {
    "target_ip": "203.0.113.99",
    "hostname": "",
    "domain": "test.example.com",
    "ttl": 300
  }
}
```

**Creates:**
- Random hostname like `k3m9x2a7.test.example.com → 203.0.113.99`
- Hostname saved to config file

**Use case:** Quick testing without hostname management

## Common Workflows

### Workflow 1: DNS for Existing EC2 Instance

```bash
# Get existing EC2 instance IP
INSTANCE_IP=$(aws ec2 describe-instances \
  --instance-ids i-0abc123def456 \
  --query 'Reservations[0].Instances[0].PublicIpAddress' \
  --output text)

# Create DNS config
cat > stacks/existing.json << EOF
{
  "dns": {
    "target_ip": "$INSTANCE_IP",
    "hostname": "legacy",
    "domain": "example.com"
  }
}
EOF

# Create DNS
./bin/ec2 -c -n existing

# Update DNS if IP changes
# Edit target_ip in stacks/existing.json
./bin/ec2 -d -n existing
./bin/ec2 -c -n existing
```

### Workflow 2: Point Domain to Load Balancer

```bash
# Get ALB DNS name (not IP - use CNAME or ALIAS in Route53 console for this)
# For IP-based (like NLB with static IP):

NLB_IP="52.1.2.3"

cat > stacks/lb.json << EOF
{
  "dns": {
    "target_ip": "$NLB_IP",
    "hostname": "lb",
    "domain": "example.com",
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
EOF

./bin/ec2 -c -n lb
```

### Workflow 3: Rapid Testing with Random Hostnames

```bash
# Test iteration 1
cat > stacks/test.json << 'EOF'
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "",
    "domain": "test.example.com"
  }
}
EOF

./bin/ec2 -c -n test
# Generated: a3k5m7.test.example.com
# Test...
./bin/ec2 -d -n test

# Test iteration 2
./bin/ec2 -c -n test
# Generated: x9p2q4.test.example.com (different!)
# No conflicts, no rate limits
```

### Workflow 4: Migrate from Other Cloud Provider

```bash
# Setup DNS while migrating from DigitalOcean to AWS

# Step 1: Point DNS to DO droplet
cat > stacks/migrate.json << 'EOF'
{
  "dns": {
    "target_ip": "167.99.123.45",
    "hostname": "app",
    "domain": "example.com",
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
EOF

./bin/ec2 -c -n migrate

# Step 2: Create AWS instance
# Add VM section to config
{
  "vm": { ... },
  "dns": { ... }
}

# Step 3: Deploy to AWS, test

# Step 4: Update DNS to point to AWS
# dns.target_ip will auto-use VM's IP
./bin/ec2 -d -n migrate
./bin/ec2 -c -n migrate

# DNS now points to AWS instance
```

## Configuration Reference

### DNS Section Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `target_ip` | string | Yes* | - | IP address for DNS records (*not required if VM section present) |
| `hostname` | string | No | random | Hostname (empty = generate random 8-char string) |
| `domain` | string | Yes | - | Domain name (must exist in Route53) |
| `ttl` | number | No | 300 | DNS record TTL in seconds |
| `is_apex_domain` | boolean | No | false | Create A record for apex domain |
| `cname_aliases` | array | No | [] | CNAME aliases to create |

### VM Section Fields

See main README for complete VM configuration options.

**Key fields:**
- `region` - AWS region
- `os` - Operating system
- `instance_type` - EC2 instance type
- `users` - Users to create
- `cloud_init_file` - Cloud-init template
- `packages` - Additional packages

## Validation Rules

### DNS-Only Mode

**Required:**
- `dns.target_ip` must be specified
- `dns.domain` must be specified

**Optional:**
- `dns.hostname` (defaults to random if empty)
- All other DNS fields

**Example:**
```json
{
  "dns": {
    "target_ip": "203.0.113.10",
    "domain": "example.com"
  }
}
```

Minimal valid DNS-only config.

### VM + DNS Mode

**Required:**
- `vm.users` must have at least one user
- `dns.domain` must be specified

**Optional:**
- `dns.target_ip` (automatically uses VM's public IP)
- All other fields

**Example:**
```json
{
  "vm": {
    "users": [{"username": "admin", "github_username": "gherlein"}]
  },
  "dns": {
    "domain": "example.com"
  }
}
```

Minimal valid full stack config (random hostname generated, defaults applied).

## Output Fields

### DNS Section Output

After creating DNS records, the config is updated with:

```json
{
  "dns": {
    // Input fields...
    "zone_id": "Z0333201...",
    "fqdn": "app.example.com",
    "dns_records": [
      {"name": "app.example.com", "type": "A", "value": "203.0.113.10", "ttl": 300},
      {"name": "www.example.com", "type": "CNAME", "value": "app.example.com", "ttl": 300}
    ]
  }
}
```

### VM Section Output

After creating VM, the config is updated with:

```json
{
  "vm": {
    // Input fields...
    "stack_name": "mystack",
    "stack_id": "arn:aws:cloudformation:...",
    "instance_id": "i-0abc123...",
    "public_ip": "54.184.71.168",
    "security_group": "sg-0acf5e...",
    "ami_id": "ami-0030e43..."
  }
}
```

## Updating Existing Resources

### Update DNS Target IP

```bash
# Edit config file
vim stacks/mystack.json
# Change dns.target_ip to new value

# Delete old records
./bin/ec2 -d -n mystack

# Create new records
./bin/ec2 -c -n mystack
```

### Add DNS to Existing VM-Only Stack

```bash
# Current config (VM only)
{
  "vm": { ... }
}

# Add DNS section
{
  "vm": { ...existing VM config... },
  "dns": {
    "hostname": "app",
    "domain": "example.com"
  }
}

# Run create again - detects existing VM, only creates DNS
./bin/ec2 -c -n mystack
```

## Limitations

### Current Limitations

1. **No ALIAS records**: Only A and CNAME records supported
   - For ALB/CloudFront, use AWS console to create ALIAS records
   - This tool uses standard A/CNAME which work for IP-based targets

2. **No health checks**: DNS records are static
   - No automatic failover
   - No health-based routing

3. **Single IP per record**: No multi-value or weighted routing
   - Each A record points to one IP
   - For load balancing, use actual load balancer

4. **No advanced routing**: No geolocation, latency-based, or failover routing

### Future Enhancements

Potential additions:
- ALIAS record support for AWS resources
- Health check configuration
- Weighted routing policies
- Geolocation routing
- Multi-value answers

## Troubleshooting

### DNS Records Not Created

**Error:** "dns.target_ip is required when vm section is not present"

**Cause:** DNS-only mode requires explicit IP

**Fix:**
```json
{
  "dns": {
    "target_ip": "203.0.113.10",  // Add this
    "domain": "example.com"
  }
}
```

### DNS Not Resolving

**Check Route53:**
```bash
aws route53 list-resource-record-sets \
  --hosted-zone-id Z03332011VT7SOCHIV083 \
  --query "ResourceRecordSets[?Name=='app.example.com.']"
```

**Check propagation:**
```bash
# Query AWS nameserver directly
dig @ns-97.awsdns-12.com app.example.com

# Query Google DNS
dig @8.8.8.8 app.example.com

# Flush local DNS cache (macOS)
sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder
```

### Wrong IP in DNS

**Check config:**
```bash
cat stacks/mystack.json | jq .dns.target_ip
```

**Update:**
```bash
# Edit config, change target_ip
vim stacks/mystack.json

# Recreate DNS
./bin/ec2 -d -n mystack
./bin/ec2 -c -n mystack
```

## Migration from Flat Format

### Old Format (Still Works)

```json
{
  "users": [{"username": "admin", "github_username": "gherlein"}],
  "hostname": "app",
  "domain": "example.com"
}
```

Automatically converted to:

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

### Migration Steps

1. **Identify config type:**
   - Has `users` or `instance_type`? → Add `vm` section
   - Has `hostname` or `domain`? → Add `dns` section

2. **Restructure:**
   - Move VM fields into `vm` object
   - Move DNS fields into `dns` object

3. **Test:**
   ```bash
   ./bin/ec2 -c -n test
   ./bin/ec2 -d -n test
   ```

**Note:** Migration is optional. Flat format still works and is automatically converted.

## Comparison with AWS CLI

### Using This Tool

```bash
# One command
./bin/ec2 -c -n mystack

# Creates A record, CNAME records, apex record
# Saves all details to JSON
```

### Using AWS CLI Directly

```bash
# Multiple commands needed
aws route53 change-resource-record-sets --hosted-zone-id Z123... --change-batch '{
  "Changes": [
    {
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "app.example.com",
        "Type": "A",
        "TTL": 300,
        "ResourceRecords": [{"Value": "203.0.113.10"}]
      }
    }
  ]
}'

# Repeat for each CNAME...
# Repeat for apex domain...
# Manually track zone ID, record details...
```

**This tool handles:**
- Zone ID lookup
- Multiple record creation
- Automatic cleanup on delete
- Config file management
- Random hostname generation

## Use Cases

### 1. Development on External VPS

**Scenario:** Using Linode/DigitalOcean for cost savings

```json
{
  "dns": {
    "target_ip": "170.187.xxx.xxx",
    "hostname": "dev",
    "domain": "mycompany.com"
  }
}
```

### 2. Staging Environment on Hetzner

**Scenario:** Staging on cheaper European provider

```json
{
  "dns": {
    "target_ip": "95.217.xxx.xxx",
    "hostname": "staging",
    "domain": "mycompany.com",
    "cname_aliases": ["staging-api", "staging-cdn"]
  }
}
```

### 3. Point to On-Premise Server

**Scenario:** Hybrid cloud setup

```json
{
  "dns": {
    "target_ip": "203.0.113.100",
    "hostname": "onprem",
    "domain": "mycompany.com"
  }
}
```

### 4. Testing SSL Certificates

**Scenario:** Need real domain for Let's Encrypt testing

```json
{
  "dns": {
    "target_ip": "127.0.0.1",
    "hostname": "",
    "domain": "test.example.com"
  }
}
```

Generates random hostname, lets you test SSL locally with SSH tunnel.

### 5. Multi-Region Setup

**Scenario:** Different regions for different services

```bash
# US-EAST region (AWS)
{
  "vm": {"region": "us-east-1", ...},
  "dns": {"hostname": "us", "domain": "example.com"}
}

# EU region (DigitalOcean)
{
  "dns": {
    "target_ip": "167.99.xxx.xxx",
    "hostname": "eu",
    "domain": "example.com"
  }
}

# Asia region (Vultr)
{
  "dns": {
    "target_ip": "45.76.xxx.xxx",
    "hostname": "asia",
    "domain": "example.com"
  }
}
```

## Best Practices

### 1. Use Descriptive Hostnames

```json
// Good
"hostname": "app-production"
"hostname": "api-staging"

// Avoid
"hostname": "server1"
"hostname": "test"
```

### 2. Set Appropriate TTL

```json
// Production (longer TTL, better caching)
"ttl": 3600

// Development (shorter TTL, faster updates)
"ttl": 60

// Default (balanced)
"ttl": 300
```

### 3. Use Apex + WWW Pattern

```json
{
  "dns": {
    "hostname": "www",
    "domain": "example.com",
    "is_apex_domain": true
  }
}
```

Both `www.example.com` and `example.com` work.

### 4. Group Related Services with CNAMEs

```json
{
  "dns": {
    "hostname": "main",
    "cname_aliases": ["api", "cdn", "static", "assets"]
  }
}
```

Change one IP, all aliases follow.

## Summary

**DNS-Only Mode Enables:**

✅ Manage DNS for any infrastructure
✅ Point domains to external servers
✅ Quick DNS setup without EC2
✅ Random hostnames for testing
✅ Same tool for all DNS needs
✅ Automatic cleanup on delete

**Commands:**

```bash
# Create DNS
./bin/ec2 -c -n mystack

# Delete DNS
./bin/ec2 -d -n mystack

# Same commands work for VM-only, DNS-only, or full stack
```

The tool automatically detects mode from config structure.
