# DNS-Only Mode Implementation Plan

## Overview

Extend the EC2 instance manager to support DNS-only operations without creating EC2 instances. This allows using the same tool for:
1. Full stack: EC2 + DNS (current functionality)
2. DNS only: Manage Route53 records for existing infrastructure

## Use Cases

### DNS-Only Scenarios

1. **External Servers**: Point domain to non-AWS infrastructure
   - DigitalOcean droplet
   - On-premise server
   - Other cloud provider VM

2. **Existing EC2**: Manage DNS for manually created instances
   - Pre-existing EC2 instances
   - Instances created via AWS console
   - Instances from other tools

3. **Load Balancers**: Point domains to AWS load balancers
   - Application Load Balancer
   - Network Load Balancer
   - Classic Load Balancer

4. **CDN/Services**: Configure DNS for third-party services
   - CloudFront distribution
   - External CDN
   - SaaS platform

## Current Architecture

### Existing JSON Structure (Flat)

```json
{
  "region": "us-east-1",
  "os": "ubuntu-22.04",
  "instance_type": "t3.micro",
  "users": [...],
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "is_apex_domain": true,
  "cname_aliases": ["www", "api"],
  "vpc_id": "vpc-123",
  "subnet_id": "subnet-456"
}
```

**Issues:**
- EC2 and DNS settings mixed together
- No way to skip EC2 creation
- No way to specify IP for existing infrastructure
- Can't manage DNS independently

## Proposed Architecture

### Option A: Nested Structure with Optional Sections

```json
{
  "vm": {
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "instance_type": "t3.micro",
    "cloud_init_file": "cloud-init/webserver.yaml",
    "working_dir": "/var/www/html",
    "packages": ["htop", "tree"],
    "users": [
      {"username": "admin", "github_username": "gherlein"}
    ],
    "vpc_id": "vpc-896fc4ec",
    "subnet_id": "subnet-38d1c410"
  },
  "dns": {
    "hostname": "dev",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": true,
    "cname_aliases": ["www", "api"],
    "target_ip": ""
  }
}
```

**Behavior:**
- If `vm` section present → Create EC2, use its IP for DNS
- If `vm` section absent → DNS only, use `target_ip`
- If `dns` section absent → EC2 only, no DNS

### Option B: Mode Field

```json
{
  "mode": "dns-only",
  "target_ip": "203.0.113.10",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "is_apex_domain": true,
  "cname_aliases": ["www"]
}
```

**Modes:**
- `full` - Create EC2 + DNS (default)
- `dns-only` - DNS records only
- `vm-only` - EC2 only, no DNS

**Issues:**
- Mode field is extra complexity
- Still have mixed EC2/DNS fields

### Option C: Separate Config Types (File Extension Based)

**ec2-config.json** - Full EC2 + DNS:
```json
{
  "region": "us-east-1",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

**dns-config.json** - DNS only:
```json
{
  "target_ip": "203.0.113.10",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,
  "is_apex_domain": true,
  "cname_aliases": ["www"]
}
```

**Issues:**
- Need different file naming convention
- Tool needs to detect config type
- Less flexible

## Recommended Approach: Option A (Nested Structure)

### Why Nested Structure?

✅ **Clear Separation**: VM and DNS concerns separated
✅ **Flexible**: Any combination of VM/DNS
✅ **Backward Compatible**: Can support flat structure during migration
✅ **Self-Documenting**: Config clearly shows what will be created
✅ **Extensible**: Easy to add new sections (e.g., "storage", "monitoring")

## Detailed Design

### Configuration Schema

```go
type Config struct {
    VM  *VMConfig  `json:"vm,omitempty"`
    DNS *DNSConfig `json:"dns,omitempty"`
}

type VMConfig struct {
    Region        string   `json:"region,omitempty"`
    OS            string   `json:"os,omitempty"`
    InstanceType  string   `json:"instance_type,omitempty"`
    CloudInitFile string   `json:"cloud_init_file,omitempty"`
    WorkingDir    string   `json:"working_dir,omitempty"`
    Packages      []string `json:"packages,omitempty"`
    Users         []User   `json:"users,omitempty"`
    VpcID         string   `json:"vpc_id,omitempty"`
    SubnetID      string   `json:"subnet_id,omitempty"`

    // Output fields
    StackName     string `json:"stack_name,omitempty"`
    StackID       string `json:"stack_id,omitempty"`
    InstanceID    string `json:"instance_id,omitempty"`
    PublicIP      string `json:"public_ip,omitempty"`
    SecurityGroup string `json:"security_group,omitempty"`
    AMIID         string `json:"ami_id,omitempty"`
}

type DNSConfig struct {
    Hostname      string   `json:"hostname,omitempty"`
    Domain        string   `json:"domain,omitempty"`
    TTL           int      `json:"ttl,omitempty"`
    IsApexDomain  bool     `json:"is_apex_domain,omitempty"`
    CNAMEAliases  []string `json:"cname_aliases,omitempty"`
    TargetIP      string   `json:"target_ip,omitempty"`

    // Output fields
    ZoneID     string      `json:"zone_id,omitempty"`
    FQDN       string      `json:"fqdn,omitempty"`
    DNSRecords []DNSRecord `json:"dns_records,omitempty"`
}
```

### Execution Logic

```go
func createStack(stackName string) {
    config := readConfig(stackName)

    var publicIP string

    // 1. Create VM if specified
    if config.VM != nil {
        publicIP = createEC2Instance(config.VM)
        config.VM.PublicIP = publicIP
    }

    // 2. Create DNS if specified
    if config.DNS != nil {
        // Use VM IP if created, otherwise use target_ip
        if publicIP != "" {
            config.DNS.TargetIP = publicIP
        }

        if config.DNS.TargetIP == "" {
            log.Fatal("DNS config requires target_ip when vm section is not present")
        }

        createDNSRecords(config.DNS)
    }

    saveConfig(stackName, config)
}

func deleteStack(stackName string) {
    config := readConfig(stackName)

    // 1. Delete DNS if specified
    if config.DNS != nil {
        deleteDNSRecords(config.DNS)
    }

    // 2. Delete VM if specified
    if config.VM != nil {
        deleteEC2Instance(config.VM)
    }

    saveConfig(stackName, config)
}
```

### Validation Rules

```go
func validateConfig(config *Config) error {
    // At least one section required
    if config.VM == nil && config.DNS == nil {
        return errors.New("config must have vm and/or dns section")
    }

    // DNS validation
    if config.DNS != nil {
        if config.DNS.Domain == "" {
            return errors.New("dns.domain is required")
        }

        // If no VM, target_ip is required
        if config.VM == nil && config.DNS.TargetIP == "" {
            return errors.New("dns.target_ip is required when vm section is not present")
        }

        // Generate random hostname if empty
        if config.DNS.Hostname == "" {
            config.DNS.Hostname = generateRandomHostname()
        }
    }

    // VM validation
    if config.VM != nil {
        if len(config.VM.Users) == 0 {
            return errors.New("vm.users is required")
        }
    }

    return nil
}
```

## Configuration Examples

### Example 1: DNS-Only for External Server

**File:** `stacks/external.json`

```json
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
```

**Creates:**
- A: `app.example.com → 203.0.113.10`
- A: `example.com → 203.0.113.10`
- CNAME: `www.example.com → app.example.com`
- CNAME: `api.example.com → app.example.com`

**No EC2 instance created.**

**Commands:**
```bash
# Create DNS records
./bin/ec2 -c -n external

# Delete DNS records
./bin/ec2 -d -n external
```

### Example 2: DNS-Only with Random Hostname

**File:** `stacks/temp-dns.json`

```json
{
  "dns": {
    "target_ip": "198.51.100.5",
    "hostname": "",
    "domain": "example.com",
    "ttl": 300
  }
}
```

**Creates:**
- A: `<random>.example.com → 198.51.100.5`
- Random hostname saved to config

**Use case:** Point temporary DNS to external server for testing.

### Example 3: Full Stack (EC2 + DNS)

**File:** `stacks/full.json`

```json
{
  "vm": {
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "instance_type": "t3.micro",
    "users": [
      {"username": "admin", "github_username": "gherlein"}
    ],
    "cloud_init_file": "cloud-init/webserver.yaml",
    "packages": ["htop"]
  },
  "dns": {
    "hostname": "app",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
```

**Creates:**
- EC2 instance (t3.micro, Ubuntu 22.04)
- A: `app.example.com → <EC2 IP>`
- A: `example.com → <EC2 IP>`
- CNAME: `www.example.com → app.example.com`

**Note:** `dns.target_ip` is automatically set to the EC2 instance's public IP.

### Example 4: EC2 Only (No DNS)

**File:** `stacks/vm-only.json`

```json
{
  "vm": {
    "region": "us-east-1",
    "os": "ubuntu-22.04",
    "instance_type": "t3.micro",
    "users": [
      {"username": "admin", "github_username": "gherlein"}
    ]
  }
}
```

**Creates:**
- EC2 instance only
- No DNS records
- Access via IP: `ssh admin@<IP>`

### Example 5: Update DNS for Existing VM

**File:** `stacks/update-dns.json`

```json
{
  "dns": {
    "target_ip": "54.184.71.168",
    "hostname": "dev",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": false,
    "cname_aliases": []
  }
}
```

**Use case:** Point DNS to existing EC2 instance created manually or by other tools.

## Migration Strategy

### Phase 1: Support Both Formats

Add logic to detect config format:

```go
func readConfig(stackName string) (*Config, string, error) {
    // Read JSON
    data := readFile(filename)

    // Try nested format first
    var nestedConfig Config
    if err := json.Unmarshal(data, &nestedConfig); err == nil {
        if nestedConfig.VM != nil || nestedConfig.DNS != nil {
            return &nestedConfig, filename, nil
        }
    }

    // Fall back to flat format (backward compatibility)
    var flatConfig StackConfig
    if err := json.Unmarshal(data, &flatConfig); err != nil {
        return nil, "", err
    }

    // Convert flat to nested
    return convertFlatToNested(&flatConfig), filename, nil
}

func convertFlatToNested(flat *StackConfig) *Config {
    nested := &Config{}

    // VM section if EC2 fields present
    if len(flat.Users) > 0 || flat.InstanceType != "" {
        nested.VM = &VMConfig{
            Region: flat.Region,
            OS: flat.OS,
            InstanceType: flat.InstanceType,
            // ... copy all VM fields
        }
    }

    // DNS section if DNS fields present
    if flat.Domain != "" {
        nested.DNS = &DNSConfig{
            Hostname: flat.Hostname,
            Domain: flat.Domain,
            TTL: flat.TTL,
            // ... copy all DNS fields
        }
    }

    return nested
}
```

### Phase 2: Deprecate Flat Format

1. Add warning when flat format detected
2. Update all examples to nested format
3. Document migration path
4. Keep flat format working (don't break existing configs)

### Phase 3: (Optional) Remove Flat Format

After sufficient deprecation period, remove flat format support.

## Implementation Steps

### Step 1: Define New Structs

**File:** `main.go`

```go
// Top-level config
type Config struct {
    VM  *VMConfig  `json:"vm,omitempty"`
    DNS *DNSConfig `json:"dns,omitempty"`
}

// VM configuration
type VMConfig struct {
    Region        string   `json:"region,omitempty"`
    OS            string   `json:"os,omitempty"`
    InstanceType  string   `json:"instance_type,omitempty"`
    CloudInitFile string   `json:"cloud_init_file,omitempty"`
    WorkingDir    string   `json:"working_dir,omitempty"`
    Packages      []string `json:"packages,omitempty"`
    Users         []User   `json:"users,omitempty"`
    VpcID         string   `json:"vpc_id,omitempty"`
    SubnetID      string   `json:"subnet_id,omitempty"`

    // Output fields
    StackName     string `json:"stack_name,omitempty"`
    StackID       string `json:"stack_id,omitempty"`
    InstanceID    string `json:"instance_id,omitempty"`
    PublicIP      string `json:"public_ip,omitempty"`
    SecurityGroup string `json:"security_group,omitempty"`
    AMIID         string `json:"ami_id,omitempty"`

    // Network resources for cleanup
    CreatedVPC            bool   `json:"created_vpc,omitempty"`
    CreatedSubnet         bool   `json:"created_subnet,omitempty"`
    InternetGatewayID     string `json:"internet_gateway_id,omitempty"`
    RouteTableID          string `json:"route_table_id,omitempty"`
    RouteTableAssociation string `json:"route_table_association_id,omitempty"`
}

// DNS configuration
type DNSConfig struct {
    Hostname     string   `json:"hostname,omitempty"`
    Domain       string   `json:"domain,omitempty"`
    TTL          int      `json:"ttl,omitempty"`
    IsApexDomain bool     `json:"is_apex_domain,omitempty"`
    CNAMEAliases []string `json:"cname_aliases,omitempty"`
    TargetIP     string   `json:"target_ip,omitempty"`

    // Output fields
    ZoneID     string      `json:"zone_id,omitempty"`
    FQDN       string      `json:"fqdn,omitempty"`
    DNSRecords []DNSRecord `json:"dns_records,omitempty"`
}

// Keep StackConfig for backward compatibility
type StackConfig struct {
    // All existing fields
    // Used for flat format conversion
}
```

### Step 2: Update Read/Write Logic

```go
func readConfig(stackName string) (*Config, string, error) {
    filename := resolveConfigPath(stackName)
    data, err := os.ReadFile(filename)
    if err != nil {
        return nil, "", err
    }

    // Try nested format
    var config Config
    if err := json.Unmarshal(data, &config); err == nil {
        if config.VM != nil || config.DNS != nil {
            // Nested format detected
            return &config, filename, nil
        }
    }

    // Fall back to flat format
    var flatConfig StackConfig
    if err := json.Unmarshal(data, &flatConfig); err != nil {
        return nil, "", fmt.Errorf("invalid config format: %w", err)
    }

    fmt.Println("Warning: Using deprecated flat config format. Consider migrating to nested format.")
    return convertFlatToNested(&flatConfig), filename, nil
}

func writeConfig(filename string, config *Config) error {
    data, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filename, data, 0644)
}
```

### Step 3: Update Creation Logic

```go
func createStack(stackName string) {
    ctx := context.Background()
    config, configFile, err := readConfig(stackName)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    // Validate config
    if err := validateConfig(config); err != nil {
        log.Fatalf("Invalid configuration: %v", err)
    }

    var publicIP string
    var region string

    // Create VM if specified
    if config.VM != nil {
        fmt.Println("Creating EC2 instance...")
        publicIP, region = createVMStack(ctx, config.VM)
        config.VM.PublicIP = publicIP
    }

    // Create DNS if specified
    if config.DNS != nil {
        // Use VM IP if created, otherwise require target_ip
        if publicIP != "" {
            config.DNS.TargetIP = publicIP
        }

        if config.DNS.TargetIP == "" {
            log.Fatal("DNS configuration requires target_ip when vm section is not present")
        }

        // Use region from VM if available
        if region == "" && config.VM != nil {
            region = config.VM.Region
        }
        if region == "" {
            region = "us-east-1" // Default
        }

        fmt.Println("Creating DNS records...")
        createDNSStack(ctx, config.DNS, region)
    }

    // Save updated config
    if err := writeConfig(configFile, config); err != nil {
        log.Fatalf("Failed to save config: %v", err)
    }
}

func deleteStack(stackName string) {
    ctx := context.Background()
    config, configFile, err := readConfig(stackName)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    // Delete DNS first (before IP goes away)
    if config.DNS != nil && len(config.DNS.DNSRecords) > 0 {
        fmt.Println("Deleting DNS records...")
        deleteDNSStack(ctx, config.DNS)
        // Clear DNS output fields
        config.DNS.ZoneID = ""
        config.DNS.FQDN = ""
        config.DNS.DNSRecords = nil
    }

    // Delete VM
    if config.VM != nil && config.VM.StackID != "" {
        fmt.Println("Deleting EC2 instance...")
        deleteVMStack(ctx, config.VM)
        // Clear VM output fields
        config.VM.StackName = ""
        config.VM.StackID = ""
        config.VM.InstanceID = ""
        config.VM.PublicIP = ""
        // ... clear all output fields
    }

    // Save cleared config
    if err := writeConfig(configFile, config); err != nil {
        log.Fatalf("Failed to save config: %v", err)
    }
}
```

### Step 4: Refactor Existing Functions

**Before:** `createStack()` does everything

**After:** Separate functions

```go
func createVMStack(ctx context.Context, vm *VMConfig) (publicIP string, region string) {
    // All EC2 creation logic
    // Returns public IP and region
}

func createDNSStack(ctx context.Context, dns *DNSConfig, region string) {
    // All DNS creation logic
    // Populates dns.DNSRecords, dns.ZoneID, dns.FQDN
}

func deleteVMStack(ctx context.Context, vm *VMConfig) {
    // Delete CloudFormation stack
    // Cleanup network resources
}

func deleteDNSStack(ctx context.Context, dns *DNSConfig) {
    // Delete Route53 records
}
```

## Command Line Interface

### No Changes to CLI

```bash
# All commands work the same
./bin/ec2 -c -n mystack    # Create (VM and/or DNS based on config)
./bin/ec2 -d -n mystack    # Delete (VM and/or DNS based on config)
```

### Help Text Updates

```
Usage: ./bin/ec2 [options]

Options:
  -c, --create    Create resources (EC2 and/or DNS based on config)
  -d, --delete    Delete resources (EC2 and/or DNS based on config)
  -n, --name      Stack name (required)

Config Format:
  Nested format (recommended):
    {
      "vm": { ... },    # Optional: EC2 configuration
      "dns": { ... }    # Optional: DNS configuration
    }

  At least one section (vm or dns) is required.

  DNS-only mode: Omit "vm" section and specify "dns.target_ip"
  VM-only mode: Omit "dns" section
  Full mode: Include both sections

Examples:
  # Full stack (EC2 + DNS)
  ./bin/ec2 -c -n mystack

  # DNS only
  ./bin/ec2 -c -n external-dns

  # Delete any stack type
  ./bin/ec2 -d -n mystack
```

## Example Workflows

### Workflow 1: Point Domain to DigitalOcean Droplet

```bash
# Create config
cat > stacks/do-dns.json << 'EOF'
{
  "dns": {
    "target_ip": "167.99.123.45",
    "hostname": "app",
    "domain": "example.com",
    "ttl": 300,
    "is_apex_domain": true,
    "cname_aliases": ["www"]
  }
}
EOF

# Create DNS records
./bin/ec2 -c -n do-dns

# Verify
dig +short app.example.com
# Output: 167.99.123.45

# Update IP later (edit config, recreate)
# Or delete when done
./bin/ec2 -d -n do-dns
```

### Workflow 2: Manage DNS for Existing EC2

```bash
# Get IP from existing EC2 instance
EXISTING_IP=$(aws ec2 describe-instances \
  --instance-ids i-0abc123 \
  --query 'Reservations[0].Instances[0].PublicIpAddress' \
  --output text)

# Create DNS config
cat > stacks/existing-vm.json << EOF
{
  "dns": {
    "target_ip": "$EXISTING_IP",
    "hostname": "legacy",
    "domain": "example.com",
    "ttl": 300
  }
}
EOF

# Create DNS
./bin/ec2 -c -n existing-vm
```

### Workflow 3: Create VM, Add DNS Later

```bash
# Step 1: Create VM only
cat > stacks/myvm.json << 'EOF'
{
  "vm": {
    "region": "us-east-1",
    "users": [{"username": "admin", "github_username": "gherlein"}]
  }
}
EOF

./bin/ec2 -c -n myvm
# Output: Public IP: 54.184.71.168

# Step 2: Add DNS configuration
# Edit stacks/myvm.json, add dns section:
{
  "vm": { ... },
  "dns": {
    "target_ip": "54.184.71.168",
    "hostname": "dev",
    "domain": "example.com"
  }
}

# Step 3: Update stack (create DNS)
./bin/ec2 -c -n myvm
# Detects existing VM, only creates DNS
```

### Workflow 4: Random Hostname Testing

```bash
# Create with random hostname
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
# Generated: k3m9x2a7.test.example.com

# Test...

./bin/ec2 -d -n test

# Create again - new random hostname
./bin/ec2 -c -n test
# Generated: x9p2q4.test.example.com
```

## Implementation Checklist

### Code Changes

- [ ] Define `Config`, `VMConfig`, `DNSConfig` structs
- [ ] Update `readConfig()` to support both formats
- [ ] Add `convertFlatToNested()` for backward compatibility
- [ ] Update `validateConfig()` with separate VM/DNS validation
- [ ] Refactor `createStack()` into `createVMStack()` and `createDNSStack()`
- [ ] Refactor `deleteStack()` into `deleteVMStack()` and `deleteDNSStack()`
- [ ] Update `writeConfig()` to handle nested format
- [ ] Add `target_ip` validation for DNS-only mode
- [ ] Update random hostname generation to work with DNS section
- [ ] Update cloud-init template data population

### Documentation Changes

- [ ] Update `README.md` with nested format examples
- [ ] Update `EXAMPLES.md` with DNS-only examples
- [ ] Create `DNS_ONLY_GUIDE.md` with detailed DNS-only documentation
- [ ] Update help text in CLI
- [ ] Add migration guide for existing configs
- [ ] Update `FEATURES_SUMMARY.md`

### Testing

- [ ] Test VM-only mode
- [ ] Test DNS-only mode
- [ ] Test full mode (VM + DNS)
- [ ] Test flat format backward compatibility
- [ ] Test random hostname with DNS-only
- [ ] Test updating existing stack (add DNS to VM-only)
- [ ] Test deletion in all modes
- [ ] Verify DNS records created correctly
- [ ] Verify EC2 resources cleaned up properly

### Example Files

- [ ] Create `example-dns-only.json`
- [ ] Create `example-vm-only.json`
- [ ] Create `example-full-nested.json`
- [ ] Create `example-dns-random-hostname.json`
- [ ] Update existing examples to nested format

## Benefits

### For Users

1. **Flexibility**: Manage DNS independently of EC2
2. **Simplicity**: Clear separation of concerns
3. **Cost Savings**: Point domains to cheaper infrastructure
4. **Migration**: Easy to move between cloud providers
5. **Testing**: Quick DNS setup for external servers

### For Code

1. **Modularity**: VM and DNS logic separated
2. **Maintainability**: Easier to modify each component
3. **Testability**: Can test VM and DNS independently
4. **Extensibility**: Easy to add new sections (storage, monitoring, etc.)

## Backward Compatibility

### Existing Flat Configs Continue to Work

```json
{
  "users": [...],
  "hostname": "dev",
  "domain": "example.com"
}
```

Automatically converted to:

```json
{
  "vm": {
    "users": [...]
  },
  "dns": {
    "hostname": "dev",
    "domain": "example.com"
  }
}
```

### No Breaking Changes

- All existing configs work without modification
- All existing commands work the same
- Output format changes (nested) but compatible
- Migration is optional, not required

## Future Enhancements

### Additional Sections

```json
{
  "vm": { ... },
  "dns": { ... },
  "storage": {
    "ebs_volumes": [
      {"size": 100, "type": "gp3", "mount": "/data"}
    ],
    "s3_buckets": ["backups", "logs"]
  },
  "monitoring": {
    "cloudwatch_alarms": true,
    "sns_topic": "arn:aws:sns:..."
  },
  "backup": {
    "schedule": "daily",
    "retention": 7
  }
}
```

### Multiple Targets

```json
{
  "dns": {
    "hostname": "lb",
    "domain": "example.com",
    "targets": [
      {"ip": "203.0.113.10", "weight": 70},
      {"ip": "203.0.113.11", "weight": 30}
    ],
    "routing_policy": "weighted"
  }
}
```

### Health Checks

```json
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "api",
    "domain": "example.com",
    "health_check": {
      "enabled": true,
      "path": "/health",
      "interval": 30
    }
  }
}
```

## Migration Guide for Users

### Step 1: Understand Current Config

**Your current config:**
```json
{
  "users": [...],
  "hostname": "dev",
  "domain": "example.com",
  "instance_type": "t3.micro"
}
```

### Step 2: Identify VM vs DNS Fields

**VM fields:** users, instance_type, os, cloud_init_file, packages, vpc_id, subnet_id
**DNS fields:** hostname, domain, ttl, is_apex_domain, cname_aliases

### Step 3: Restructure to Nested Format

```json
{
  "vm": {
    "users": [...],
    "instance_type": "t3.micro"
  },
  "dns": {
    "hostname": "dev",
    "domain": "example.com"
  }
}
```

### Step 4: (Optional) Use DNS-Only Mode

Remove `vm` section, add `target_ip`:

```json
{
  "dns": {
    "target_ip": "your.external.ip",
    "hostname": "dev",
    "domain": "example.com"
  }
}
```

### Step 5: Test

```bash
./bin/ec2 -c -n test
./bin/ec2 -d -n test
```

## Timeline

**Phase 1 (Week 1):**
- Implement nested structure support
- Add backward compatibility
- Update core create/delete logic

**Phase 2 (Week 2):**
- Refactor VM and DNS into separate functions
- Add validation for all modes
- Update documentation

**Phase 3 (Week 3):**
- Create examples for all modes
- Write migration guide
- Add deprecation warnings for flat format

**Phase 4 (Week 4):**
- Testing and bug fixes
- Performance optimization
- Final documentation polish

## Success Criteria

1. ✅ Can create DNS-only records with target_ip
2. ✅ Can create VM-only without DNS
3. ✅ Can create full stack (VM + DNS)
4. ✅ Existing flat configs still work
5. ✅ No breaking changes to CLI
6. ✅ Clear documentation for all modes
7. ✅ Examples for common use cases

## Summary

Restructure configuration to support:

**Current (Flat):**
```json
{"users": [...], "hostname": "dev", "domain": "example.com"}
```

**Proposed (Nested):**
```json
{
  "vm": {"users": [...]},
  "dns": {"hostname": "dev", "domain": "example.com"}
}
```

**DNS-Only (New Capability):**
```json
{
  "dns": {
    "target_ip": "203.0.113.10",
    "hostname": "dev",
    "domain": "example.com"
  }
}
```

This enables DNS management for any infrastructure, not just EC2 instances created by this tool.
