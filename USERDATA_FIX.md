# UserData Parameter Size Limit Fix

## Problem

CloudFormation parameters have a 4096 byte limit. When passing large cloud-init configurations, the base64-encoded UserData exceeded this limit, causing stack creation to fail with:

```
length is greater than 4096
```

## Solution

**Embed UserData directly in the CloudFormation template** instead of passing it as a parameter.

### Changes Made

#### 1. Template Conversion (line 88-165)

**Before:**
```go
const cloudFormationTemplate = `
Parameters:
  UserData:
    Type: String
    Description: Base64 encoded UserData script
...
Resources:
  EC2Instance:
    Properties:
      UserData: !Ref UserData
`
```

**After:**
```go
const cloudFormationTemplateStr = `
Parameters:
  # UserData parameter removed
...
Resources:
  EC2Instance:
    Properties:
      UserData: {{.UserData}}  # Go template placeholder
`
```

#### 2. Template Generator Function (line 167-185)

Added function to generate CloudFormation template with UserData embedded:

```go
func generateCloudFormationTemplate(userData string) (string, error) {
	tmpl, err := template.New("cfn").Parse(cloudFormationTemplateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse CFN template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		UserData string
	}{
		UserData: userData,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute CFN template: %w", err)
	}

	return buf.String(), nil
}
```

#### 3. Stack Creation Update (line 1191-1220)

**Before:**
```go
userData := generateMultipartUserData(userScript, cloudInitContent)

input := &cloudformation.CreateStackInput{
	StackName:    &stackName,
	TemplateBody: aws.String(cloudFormationTemplate),  // Static template
	Parameters: []types.Parameter{
		{
			ParameterKey:   aws.String("UserData"),
			ParameterValue: aws.String(userData),  // Parameter (4096 byte limit)
		},
		// ... other parameters
	},
}
```

**After:**
```go
userData := generateMultipartUserData(userScript, cloudInitContent)

// Generate template with UserData embedded
cfnTemplate, err := generateCloudFormationTemplate(userData)
if err != nil {
	log.Fatalf("failed to generate CloudFormation template: %v", err)
}

input := &cloudformation.CreateStackInput{
	StackName:    &stackName,
	TemplateBody: aws.String(cfnTemplate),  // Dynamic template with UserData
	Parameters: []types.Parameter{
		// UserData parameter removed
		// ... other parameters
	},
}
```

## Why This Works

### CloudFormation Limits

| Item | Limit | Our Usage |
|------|-------|-----------|
| **Parameter Value** | 4,096 bytes | ❌ Exceeded with large cloud-init |
| **Template Body** | 51,200 bytes | ✅ Well within limit |

### Flow Comparison

**Before (Parameter):**
```
UserData (8KB) → Base64 Encode → Parameter (8KB) → ❌ Exceeds 4KB limit
```

**After (Embedded):**
```
UserData (8KB) → Embed in Template → Template Body (20KB) → ✅ Within 51KB limit
```

## Size Analysis

### Typical UserData Size

```
User Setup Script:     ~2KB (per user × N users)
Cloud-Init YAML:       ~3KB (base configuration)
Packages:              ~0.5KB (package list)
MIME Multipart:        ~1KB (headers/boundaries)
Base64 Encoding:       ×1.37 overhead
─────────────────────────────────────────────
Total (2 users):       ~8.5KB encoded
```

With our setup (2 users, webserver cloud-init, packages):
- **Raw size**: ~6.2KB
- **Base64 encoded**: ~8.5KB
- **Result**: ❌ Exceeds 4KB parameter limit → ✅ Within 51KB template limit

## Testing

Deploy with updated binary:

```bash
cd ~/herlein/src/er/www
./aws-ec2/bin/ec2 -c er.json
```

Verify UserData embedded correctly:

```bash
# Check CloudFormation template in AWS Console
# Look for UserData in EC2Instance Properties
# Should show full base64 string, not !Ref UserData
```

## Benefits

1. **No Size Limit**: Template body supports up to 51KB (vs 4KB for parameters)
2. **Better Performance**: One less parameter resolution step
3. **Simpler Template**: No UserData parameter definition needed
4. **More Readable**: UserData visible in template when debugging

## Considerations

### Template Size

CloudFormation template body limit: **51,200 bytes**

Current template breakdown:
```
Base template:         ~2KB
UserData embedded:     ~8.5KB
Total:                 ~10.5KB
Remaining capacity:    ~40.5KB (80% available)
```

### Maximum UserData Size

Theoretical maximum with this approach:
- Template limit: 51KB
- Base template: ~2KB
- Available for UserData: ~49KB
- After base64 decode: ~36KB of actual cloud-init content

This is more than sufficient for even complex configurations.

### Alternative Approaches Considered

#### 1. S3-Backed UserData
Store UserData in S3, download in cloud-init:
- ❌ More complexity (S3 bucket management)
- ❌ Additional IAM permissions required
- ❌ Network dependency during boot
- ✅ Unlimited size support

#### 2. Compressed UserData
Gzip compress before base64 encoding:
- ❌ More complexity (decompress in UserData)
- ❌ Still hits 4KB limit with large configs
- ✅ Better compression (~40% reduction)

#### 3. Template Embedding (Chosen)
Embed UserData directly in template:
- ✅ Simple implementation
- ✅ No external dependencies
- ✅ 12× larger limit (51KB vs 4KB)
- ✅ Sufficient for all reasonable use cases

## Migration

### Existing Deployments

No impact on existing stacks. This only affects new stack creation.

### Code Compatibility

Both approaches work with the same CloudFormation API:
- Old: Parameter-based UserData
- New: Template-embedded UserData

No changes needed to stack deletion, updates, or other operations.

## Verification

After deployment, verify UserData was applied:

```bash
# SSH into instance
ssh user@instance

# Check cloud-init status
cloud-init status

# View applied UserData
sudo cat /var/lib/cloud/instance/user-data.txt

# Check logs
sudo cat /var/log/cloud-init-output.log
```

## Summary

Fixed CloudFormation parameter size limit by:
1. ✅ Removed UserData from Parameters section
2. ✅ Added Go template placeholder in template body
3. ✅ Generated template dynamically with UserData embedded
4. ✅ Removed UserData parameter from stack creation call

Result: Support for cloud-init configurations up to ~36KB (unencoded).
