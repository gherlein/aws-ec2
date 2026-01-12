# DNS Enhancement Plan: CNAME and Apex Domain Support

## Overview

Enhance the EC2 instance manager to support:
1. **CNAME records**: Allow additional DNS aliases pointing to the primary hostname
2. **Apex domain records**: Optionally make the instance respond to bare domain queries

## Requirements Analysis

### Use Cases

**Use Case 1: CNAME Aliases**
- User creates EC2 instance with hostname `dev.example.com`
- User wants additional aliases: `api.example.com`, `staging.example.com`
- CNAMEs created: `api.example.com -> dev.example.com`, `staging.example.com -> dev.example.com`

**Use Case 2: Apex Domain**
- User creates EC2 instance with hostname `www.example.com`
- User wants `example.com` (bare domain) to also point to this instance
- A record created for apex: `example.com -> <IP address>`

**Use Case 3: Combined**
- Primary: `app.example.com -> <IP>`
- CNAMEs: `api.example.com -> app.example.com`, `web.example.com -> app.example.com`
- Apex: `example.com -> <IP>`

### Technical Constraints

1. **CNAME Restrictions (RFC 1034)**:
   - CNAME records cannot exist at the zone apex
   - CNAME records cannot coexist with other record types for the same name
   - CNAME must point to a domain name, not an IP address

2. **Apex Domain Records**:
   - Must use A record (not CNAME) at apex
   - Requires the same IP address as the primary hostname

3. **Route53 Specifics**:
   - All records must be in the same hosted zone
   - Deletion requires exact match of record parameters

## Configuration Schema Design

### New Input Fields

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "ttl": 300,

  // NEW: Create A record for bare domain (example.com -> IP)
  "is_apex_domain": false,

  // NEW: Additional CNAME aliases pointing to hostname.domain
  "cname_aliases": ["api", "staging", "web"]
}
```

### New Output Fields

```json
{
  // ... existing output fields ...

  // NEW: Track all created DNS records for cleanup
  "dns_records": [
    {
      "name": "dev.example.com",
      "type": "A",
      "value": "54.184.71.168",
      "ttl": 300
    },
    {
      "name": "api.example.com",
      "type": "CNAME",
      "value": "dev.example.com",
      "ttl": 300
    },
    {
      "name": "example.com",
      "type": "A",
      "value": "54.184.71.168",
      "ttl": 300
    }
  ]
}
```

### Field Specifications

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `is_apex_domain` | boolean | `false` | If true, create A record for bare domain pointing to instance IP |
| `cname_aliases` | string[] | `[]` | List of hostnames (without domain) to create as CNAMEs pointing to primary hostname |
| `dns_records` | object[] | `[]` | Auto-filled. Tracks all DNS records created for proper cleanup |

### Validation Rules

1. **CNAMEs require primary hostname**:
   - If `cname_aliases` is non-empty, both `hostname` and `domain` must be specified

2. **Apex requires domain**:
   - If `is_apex_domain` is true, `domain` must be specified

3. **No duplicate names**:
   - CNAME aliases cannot duplicate the primary `hostname`
   - CNAME aliases cannot be empty strings

4. **Apex not in CNAME list**:
   - Cannot include empty string in `cname_aliases` (would conflict with apex)

## Implementation Design

### Data Structure Changes

**Go struct updates** (main.go line 21-39):

```go
type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`   // "A" or "CNAME"
	Value string `json:"value"`  // IP address for A, FQDN for CNAME
	TTL   int    `json:"ttl"`
}

type StackConfig struct {
	// Input fields (user provides)
	GitHubUsername string   `json:"github_username"`
	InstanceType   string   `json:"instance_type,omitempty"`
	Hostname       string   `json:"hostname,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	TTL            int      `json:"ttl,omitempty"`
	IsApexDomain   bool     `json:"is_apex_domain,omitempty"`      // NEW
	CNAMEAliases   []string `json:"cname_aliases,omitempty"`       // NEW

	// Output fields (program fills in)
	StackName     string      `json:"stack_name,omitempty"`
	StackID       string      `json:"stack_id,omitempty"`
	Region        string      `json:"region,omitempty"`
	InstanceID    string      `json:"instance_id,omitempty"`
	PublicIP      string      `json:"public_ip,omitempty"`
	SecurityGroup string      `json:"security_group,omitempty"`
	ZoneID        string      `json:"zone_id,omitempty"`
	FQDN          string      `json:"fqdn,omitempty"`
	SSHCommand    string      `json:"ssh_command,omitempty"`
	DNSRecords    []DNSRecord `json:"dns_records,omitempty"`        // NEW
}
```

### Function Changes

#### 1. New Validation Function

```go
func validateDNSConfig(cfg *StackConfig) error {
	// Validate CNAME aliases
	if len(cfg.CNAMEAliases) > 0 {
		if cfg.Hostname == "" || cfg.Domain == "" {
			return fmt.Errorf("cname_aliases requires both hostname and domain")
		}

		// Check for duplicates and empty strings
		seen := make(map[string]bool)
		for _, alias := range cfg.CNAMEAliases {
			if alias == "" {
				return fmt.Errorf("cname_aliases cannot contain empty strings")
			}
			if alias == cfg.Hostname {
				return fmt.Errorf("cname_aliases cannot duplicate primary hostname: %s", alias)
			}
			if seen[alias] {
				return fmt.Errorf("duplicate cname_alias: %s", alias)
			}
			seen[alias] = true
		}
	}

	// Validate apex domain
	if cfg.IsApexDomain && cfg.Domain == "" {
		return fmt.Errorf("is_apex_domain requires domain to be specified")
	}

	return nil
}
```

#### 2. Enhanced createDNSRecords Function

Replace `createDNSRecord()` with `createDNSRecords()` that handles multiple record types:

```go
func createDNSRecords(ctx context.Context, r53Client *route53.Client, cfg *StackConfig) ([]DNSRecord, error) {
	var createdRecords []DNSRecord

	if cfg.Domain == "" {
		return createdRecords, nil // No DNS configuration
	}

	// 1. Create primary A record (hostname.domain -> IP)
	if cfg.Hostname != "" {
		fqdn := fmt.Sprintf("%s.%s", cfg.Hostname, cfg.Domain)
		err := createARecord(ctx, r53Client, cfg.ZoneID, fqdn, cfg.PublicIP, cfg.TTL)
		if err != nil {
			return createdRecords, fmt.Errorf("failed to create primary A record: %w", err)
		}
		createdRecords = append(createdRecords, DNSRecord{
			Name:  fqdn,
			Type:  "A",
			Value: cfg.PublicIP,
			TTL:   cfg.TTL,
		})
	}

	// 2. Create CNAME records (alias.domain -> hostname.domain)
	if cfg.Hostname != "" && len(cfg.CNAMEAliases) > 0 {
		targetFQDN := fmt.Sprintf("%s.%s", cfg.Hostname, cfg.Domain)
		for _, alias := range cfg.CNAMEAliases {
			aliasFQDN := fmt.Sprintf("%s.%s", alias, cfg.Domain)
			err := createCNAMERecord(ctx, r53Client, cfg.ZoneID, aliasFQDN, targetFQDN, cfg.TTL)
			if err != nil {
				// Cleanup on failure
				deleteCreatedRecords(ctx, r53Client, cfg.ZoneID, createdRecords)
				return nil, fmt.Errorf("failed to create CNAME %s: %w", aliasFQDN, err)
			}
			createdRecords = append(createdRecords, DNSRecord{
				Name:  aliasFQDN,
				Type:  "CNAME",
				Value: targetFQDN,
				TTL:   cfg.TTL,
			})
		}
	}

	// 3. Create apex A record (domain -> IP)
	if cfg.IsApexDomain {
		err := createARecord(ctx, r53Client, cfg.ZoneID, cfg.Domain, cfg.PublicIP, cfg.TTL)
		if err != nil {
			// Cleanup on failure
			deleteCreatedRecords(ctx, r53Client, cfg.ZoneID, createdRecords)
			return nil, fmt.Errorf("failed to create apex A record: %w", err)
		}
		createdRecords = append(createdRecords, DNSRecord{
			Name:  cfg.Domain,
			Type:  "A",
			Value: cfg.PublicIP,
			TTL:   cfg.TTL,
		})
	}

	return createdRecords, nil
}
```

#### 3. New Helper Functions

```go
func createARecord(ctx context.Context, r53Client *route53.Client, zoneID, name, ip string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
						Type: r53types.RRTypeA,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(ip)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func createCNAMERecord(ctx context.Context, r53Client *route53.Client, zoneID, name, target string, ttl int) error {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	if !strings.HasSuffix(target, ".") {
		target = target + "."
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(name),
						Type: r53types.RRTypeCname,
						TTL:  aws.Int64(int64(ttl)),
						ResourceRecords: []r53types.ResourceRecord{
							{Value: aws.String(target)},
						},
					},
				},
			},
		},
	}

	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	return err
}

func deleteCreatedRecords(ctx context.Context, r53Client *route53.Client, zoneID string, records []DNSRecord) {
	for _, record := range records {
		if record.Type == "A" {
			deleteARecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		} else if record.Type == "CNAME" {
			deleteCNAMERecord(ctx, r53Client, zoneID, record.Name, record.Value, record.TTL)
		}
	}
}
```

#### 4. Updated createStack Function

Modify `createStack()` function (main.go line 307):

```go
func createStack(stackName string) {
	// ... existing setup code ...

	// NEW: Validate DNS configuration
	if err := validateDNSConfig(stackCfg); err != nil {
		log.Fatalf("Invalid DNS configuration: %v", err)
	}

	// ... existing CloudFormation creation code ...

	// Create DNS records if configured
	if zoneID != "" {
		fmt.Println("Creating DNS records...")
		stackCfg.ZoneID = zoneID
		dnsRecords, err := createDNSRecords(ctx, r53Client, stackCfg)
		if err != nil {
			log.Printf("Warning: failed to create DNS records: %v", err)
		} else {
			fmt.Printf("Created %d DNS record(s) successfully\n", len(dnsRecords))
			stackCfg.DNSRecords = dnsRecords

			// Set FQDN to primary hostname or apex if no hostname
			if stackCfg.Hostname != "" {
				stackCfg.FQDN = fmt.Sprintf("%s.%s", stackCfg.Hostname, stackCfg.Domain)
			} else if stackCfg.IsApexDomain {
				stackCfg.FQDN = stackCfg.Domain
			}

			if stackCfg.FQDN != "" {
				stackCfg.SSHCommand = fmt.Sprintf("ssh %s@%s", stackCfg.GitHubUsername, stackCfg.FQDN)
			}
		}
	}

	// ... existing config write code ...
}
```

#### 5. Updated deleteStack Function

Modify `deleteStack()` function (main.go line 449) to delete all tracked records:

```go
func deleteStack(stackName string) {
	// ... existing setup code ...

	// Delete DNS records if they were configured
	if stackCfg != nil && stackCfg.ZoneID != "" && len(stackCfg.DNSRecords) > 0 {
		fmt.Printf("Deleting %d DNS record(s)...\n", len(stackCfg.DNSRecords))
		r53Client := route53.NewFromConfig(awsCfg)

		for _, record := range stackCfg.DNSRecords {
			fmt.Printf("  Deleting %s record: %s -> %s\n", record.Type, record.Name, record.Value)

			var err error
			if record.Type == "A" {
				err = deleteARecord(ctx, r53Client, stackCfg.ZoneID, record.Name, record.Value, record.TTL)
			} else if record.Type == "CNAME" {
				err = deleteCNAMERecord(ctx, r53Client, stackCfg.ZoneID, record.Name, record.Value, record.TTL)
			}

			if err != nil {
				log.Printf("Warning: failed to delete DNS record %s: %v", record.Name, err)
			}
		}
		fmt.Println("DNS records deleted")
	}

	// ... existing CloudFormation deletion code ...
}
```

## Backward Compatibility

All changes maintain backward compatibility:

1. **Existing configs without new fields**: Work as before
   - `is_apex_domain` defaults to `false`
   - `cname_aliases` defaults to empty array `[]`
   - Behavior identical to current implementation

2. **Config file migration**: No migration needed
   - Old configs continue to work without modification
   - New fields are optional and additive

3. **DNS record tracking**: For existing deployments
   - If `dns_records` is empty but `fqdn` exists, fallback to legacy cleanup method
   - Ensures existing stacks can be deleted

## Testing Plan

### Test Case 1: CNAME Only
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com",
  "cname_aliases": ["api", "staging"]
}
```

Expected DNS records:
- A: `dev.example.com -> <IP>`
- CNAME: `api.example.com -> dev.example.com`
- CNAME: `staging.example.com -> dev.example.com`

### Test Case 2: Apex Only
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "www",
  "domain": "example.com",
  "is_apex_domain": true
}
```

Expected DNS records:
- A: `www.example.com -> <IP>`
- A: `example.com -> <IP>`

### Test Case 3: Combined
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "app",
  "domain": "example.com",
  "is_apex_domain": true,
  "cname_aliases": ["api", "web"]
}
```

Expected DNS records:
- A: `app.example.com -> <IP>`
- CNAME: `api.example.com -> app.example.com`
- CNAME: `web.example.com -> app.example.com`
- A: `example.com -> <IP>`

### Test Case 4: Apex Without Hostname
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "domain": "example.com",
  "is_apex_domain": true
}
```

Expected DNS records:
- A: `example.com -> <IP>`

### Test Case 5: Backward Compatibility
```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "dev",
  "domain": "example.com"
}
```

Expected DNS records (unchanged from current behavior):
- A: `dev.example.com -> <IP>`

## Documentation Updates

### README.md Updates

Update configuration table to include new fields:

```markdown
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `is_apex_domain` | No | `false` | Create A record for bare domain (e.g., `example.com` -> IP) |
| `cname_aliases` | No | `[]` | Array of hostnames for CNAME records pointing to primary hostname |
```

Add examples section:

```markdown
### With Apex Domain

Create instance accessible at both `www.example.com` and `example.com`:

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "www",
  "domain": "example.com",
  "is_apex_domain": true
}
```

### With CNAME Aliases

Create instance with multiple DNS names:

```json
{
  "github_username": "gherlein",
  "instance_type": "t3.micro",
  "hostname": "app",
  "domain": "example.com",
  "cname_aliases": ["api", "staging", "web"]
}
```

Access via: `app.example.com`, `api.example.com`, `staging.example.com`, or `web.example.com`
```

## Implementation Checklist

- [ ] Update `StackConfig` struct with new fields
- [ ] Add `DNSRecord` struct
- [ ] Implement `validateDNSConfig()` function
- [ ] Implement `createARecord()` helper
- [ ] Implement `createCNAMERecord()` helper
- [ ] Implement `deleteARecord()` helper (refactor existing)
- [ ] Implement `deleteCNAMERecord()` helper
- [ ] Implement `createDNSRecords()` orchestration function
- [ ] Implement `deleteCreatedRecords()` cleanup function
- [ ] Update `createStack()` to use new DNS functions
- [ ] Update `deleteStack()` to delete all tracked records
- [ ] Update `example.json` with new fields
- [ ] Update README.md with new features and examples
- [ ] Test backward compatibility
- [ ] Test CNAME-only configuration
- [ ] Test apex-only configuration
- [ ] Test combined configuration
- [ ] Test validation error cases

## Edge Cases and Error Handling

1. **Partial creation failure**: If CNAME creation fails after A record succeeds, rollback all DNS records
2. **Zone lookup failure**: Fatal error, same as current behavior
3. **Invalid CNAME alias**: Validation error before any DNS changes
4. **Conflicting records**: Route53 will return error if records already exist (UPSERT will update them)
5. **Empty dns_records on delete**: Fallback to legacy single-record delete if available

## Security Considerations

No new security implications:
- Same IAM permissions required (`route53:ChangeResourceRecordSets`)
- No additional AWS API calls beyond existing Route53 operations
- DNS record types (A and CNAME) are standard and safe

## Performance Impact

Minimal performance impact:
- Additional Route53 API calls proportional to number of CNAME aliases
- Each record creation is a separate API call (Route53 limitation)
- Stack creation time increases by ~0.5-1 second per additional DNS record
