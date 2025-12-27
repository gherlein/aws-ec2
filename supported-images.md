# Supported Base Images

The `base_image` field in your stack configuration can be set to either:
1. An **SSM Parameter path** (recommended) - AWS automatically keeps these updated
2. A specific **AMI ID** (e.g., `ami-0abc123def456`)

## SSM Parameter Paths

Use these paths in the `base_image` field. AWS maintains these parameters with the latest AMI IDs.

### Amazon Linux 2023 (Default)

| Description | SSM Parameter Path |
|-------------|-------------------|
| **AL2023 x86_64 (default)** | `/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64` |
| AL2023 x86_64 minimal | `/aws/service/ami-amazon-linux-latest/al2023-ami-minimal-kernel-default-x86_64` |
| AL2023 ARM64 | `/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64` |
| AL2023 ARM64 minimal | `/aws/service/ami-amazon-linux-latest/al2023-ami-minimal-kernel-default-arm64` |

### Amazon Linux 2

| Description | SSM Parameter Path |
|-------------|-------------------|
| AL2 x86_64 | `/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2` |
| AL2 x86_64 minimal | `/aws/service/ami-amazon-linux-latest/amzn2-ami-minimal-hvm-x86_64-ebs` |
| AL2 ARM64 | `/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-arm64-gp2` |
| AL2 ARM64 minimal | `/aws/service/ami-amazon-linux-latest/amzn2-ami-minimal-hvm-arm64-ebs` |

### Ubuntu

| Description | SSM Parameter Path |
|-------------|-------------------|
| Ubuntu 24.04 LTS x86_64 | `/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id` |
| Ubuntu 24.04 LTS ARM64 | `/aws/service/canonical/ubuntu/server/24.04/stable/current/arm64/hvm/ebs-gp3/ami-id` |
| Ubuntu 22.04 LTS x86_64 | `/aws/service/canonical/ubuntu/server/22.04/stable/current/amd64/hvm/ebs-gp2/ami-id` |
| Ubuntu 22.04 LTS ARM64 | `/aws/service/canonical/ubuntu/server/22.04/stable/current/arm64/hvm/ebs-gp2/ami-id` |
| Ubuntu 20.04 LTS x86_64 | `/aws/service/canonical/ubuntu/server/20.04/stable/current/amd64/hvm/ebs-gp2/ami-id` |
| Ubuntu 20.04 LTS ARM64 | `/aws/service/canonical/ubuntu/server/20.04/stable/current/arm64/hvm/ebs-gp2/ami-id` |

### Debian

| Description | SSM Parameter Path |
|-------------|-------------------|
| Debian 12 x86_64 | `/aws/service/debian/release/12/latest/amd64` |
| Debian 12 ARM64 | `/aws/service/debian/release/12/latest/arm64` |
| Debian 11 x86_64 | `/aws/service/debian/release/11/latest/amd64` |
| Debian 11 ARM64 | `/aws/service/debian/release/11/latest/arm64` |

### Red Hat Enterprise Linux (RHEL)

| Description | SSM Parameter Path |
|-------------|-------------------|
| RHEL 9 x86_64 | `/aws/service/redhat/rhel/9/x86_64/latest` |
| RHEL 9 ARM64 | `/aws/service/redhat/rhel/9/arm64/latest` |
| RHEL 8 x86_64 | `/aws/service/redhat/rhel/8/x86_64/latest` |
| RHEL 8 ARM64 | `/aws/service/redhat/rhel/8/arm64/latest` |

### SUSE Linux Enterprise Server (SLES)

| Description | SSM Parameter Path |
|-------------|-------------------|
| SLES 15 SP5 x86_64 | `/aws/service/suse/sles/15-sp5/x86_64/latest` |
| SLES 15 SP5 ARM64 | `/aws/service/suse/sles/15-sp5/arm64/latest` |

## Usage Examples

### Using the default (Amazon Linux 2023)

```json
{
  "github_username": "myuser",
  "instance_type": "t3.micro"
}
```

### Using Ubuntu 24.04

```json
{
  "github_username": "myuser",
  "instance_type": "t3.micro",
  "base_image": "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}
```

### Using a specific AMI ID

```json
{
  "github_username": "myuser",
  "instance_type": "t3.micro",
  "base_image": "ami-0abc123def456789"
}
```

## Notes

### Default User Names by Distribution

When using a custom cloud-init script, note that different distributions use different default usernames:

| Distribution | Default User |
|--------------|--------------|
| Amazon Linux | `ec2-user` |
| Ubuntu | `ubuntu` |
| Debian | `admin` |
| RHEL | `ec2-user` |
| SUSE | `ec2-user` |

### Package Managers

| Distribution | Package Manager | Install Command |
|--------------|-----------------|-----------------|
| Amazon Linux 2023 | dnf | `dnf install -y package` |
| Amazon Linux 2 | yum | `yum install -y package` |
| Ubuntu/Debian | apt | `apt-get update && apt-get install -y package` |
| RHEL | dnf/yum | `dnf install -y package` |
| SUSE | zypper | `zypper install -y package` |

### Finding More AMI Parameters

List available SSM parameters for AMIs:

```bash
# Amazon Linux
aws ssm get-parameters-by-path \
  --path /aws/service/ami-amazon-linux-latest \
  --query 'Parameters[*].Name'

# Ubuntu
aws ssm get-parameters-by-path \
  --path /aws/service/canonical/ubuntu/server \
  --recursive \
  --query 'Parameters[*].Name'
```

### ARM64 Instance Types

If using ARM64 images, make sure to use an ARM-based instance type:
- `t4g.micro`, `t4g.small`, `t4g.medium` (free tier eligible)
- `m6g.*`, `c6g.*`, `r6g.*` families
