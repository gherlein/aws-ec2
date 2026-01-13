# Basic Authentication Setup

## Overview

The `cloud-init/webserver.yaml` template automatically configures HTTP Basic Authentication on all web content.

## Default Credentials

**Username:** `emerging`
**Password:** `emerging2026`

These credentials are required to access the website via browser or API.

## How It Works

### During Instance Creation

1. **Password Hash Generation** (cloud-init line 119):
   ```bash
   HASH=$(caddy hash-password --plaintext 'emerging2026')
   ```

2. **Caddyfile Configuration** (cloud-init lines 124-127):
   ```
   basicauth {
     emerging $HASH
   }
   ```

3. **All domains protected**:
   - Primary: `hostname.domain.com`
   - Apex (if enabled): `domain.com`
   - Aliases: `alias.domain.com`

### Security

- Password hashed using bcrypt (Caddy default)
- Hash generated at deployment time (not hardcoded)
- HTTPS required (credentials encrypted in transit)
- Standard HTTP 401/WWW-Authenticate flow

## Access Examples

### Browser Access

1. Navigate to `https://yoursite.com`
2. Browser prompts for credentials
3. Enter:
   - Username: `emerging`
   - Password: `emerging2026`
4. Access granted

### Command Line Access

```bash
# Without credentials - 401 Unauthorized
curl https://emergingrobotics.ai/
# Response: 401 Unauthorized

# With credentials - 200 OK
curl -u emerging:emerging2026 https://emergingrobotics.ai/
# Response: <html content>

# Or with Authorization header
curl -H "Authorization: Basic ZW1lcmdpbmc6ZW1lcmdpbmcyMDI2" https://emergingrobotics.ai/
```

### API Access

```bash
# Using basic auth
curl -u emerging:emerging2026 https://emergingrobotics.ai/api/endpoint

# Using header
curl -H "Authorization: Basic $(echo -n emerging:emerging2026 | base64)" https://emergingrobotics.ai/api/endpoint
```

## Changing Credentials

### Before Instance Creation

Edit `cloud-init/webserver.yaml` line 119:

```yaml
# Generate password hash for basic auth
- HASH=$(caddy hash-password --plaintext 'your-new-password')
```

And line 126:

```yaml
basicauth {
  your-username $HASH
}
```

### After Instance Creation

**Option 1: Update Caddyfile manually**

```bash
# SSH into server
ssh user@server

# Generate new hash
NEW_HASH=$(caddy hash-password --plaintext 'new-password')

# Update Caddyfile
sudo tee /etc/caddy/Caddyfile > /dev/null << EOF
your-domain.com {
  basicauth {
    newuser $NEW_HASH
  }

  root * /var/www/html
  file_server
  # ... rest of config
}
EOF

# Reload
sudo systemctl reload caddy
```

**Option 2: Use caddy hash-password interactively**

```bash
# Generate hash
caddy hash-password
# Enter password when prompted
# Copy the hash

# Edit Caddyfile
sudo vim /etc/caddy/Caddyfile

# Update basicauth section with new hash
# Reload
sudo systemctl reload caddy
```

## Multiple Users

Add multiple username/hash pairs:

```yaml
basicauth {
  emerging $HASH
  admin $ADMIN_HASH
  developer $DEV_HASH
}
```

In cloud-init:

```yaml
runcmd:
  # Generate multiple hashes
  - HASH_EMERGING=$(caddy hash-password --plaintext 'emerging2026')
  - HASH_ADMIN=$(caddy hash-password --plaintext 'admin-password')
  - HASH_DEV=$(caddy hash-password --plaintext 'dev-password')

  # Write Caddyfile
  - |
    cat > /etc/caddy/Caddyfile << EOF
    {{.FQDN}} {
      basicauth {
        emerging \$HASH_EMERGING
        admin \$HASH_ADMIN
        developer \$HASH_DEV
      }
      root * {{.WorkingDir}}
      file_server
    }
    EOF
```

## Disabling Basic Auth

### Before Instance Creation

Remove the `basicauth` block from `cloud-init/webserver.yaml`:

```yaml
# Delete lines 125-127
basicauth {
  emerging $HASH
}
```

And remove the hash generation (line 119):

```yaml
# Delete line 119
- HASH=$(caddy hash-password --plaintext 'emerging2026')
```

### After Instance Creation

```bash
ssh user@server
sudo vim /etc/caddy/Caddyfile
# Remove basicauth block
sudo systemctl reload caddy
```

## Protecting Specific Paths

Protect only certain paths (e.g., admin panel):

```
your-domain.com {
  root * /var/www/html
  file_server

  @admin path /admin/*
  basicauth @admin {
    admin $HASH
  }
}
```

Public content accessible without auth, `/admin/*` requires credentials.

## Testing

### Verify Protection

```bash
# Should return 401
curl -I https://yoursite.com/

# Should return 200
curl -I -u emerging:emerging2026 https://yoursite.com/
```

### Verify Hash

```bash
ssh user@server
cat /etc/caddy/Caddyfile | grep -A 2 basicauth
```

Should show:
```
basicauth {
  emerging $2a$14$...hash...
}
```

## Common Issues

### "401 Unauthorized" with correct credentials

- Check username/password are correct
- Check hash was generated properly
- Check Caddyfile syntax

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
```

### Browser doesn't prompt for password

- Check basicauth block is in Caddyfile
- Check Caddy was reloaded after changes
- Clear browser cache
- Try incognito/private browsing

### Hash not set in Caddyfile

If you see literal `$HASH` in Caddyfile instead of hash value:

```bash
# Check if hash was generated
echo $HASH

# Regenerate and update manually
HASH=$(caddy hash-password --plaintext 'emerging2026')
sudo sed -i "s/\$HASH/$HASH/" /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

## Security Best Practices

### Development/Testing

✅ Use default credentials for convenience:
- Username: `emerging`
- Password: `emerging2026`

### Production

❌ Don't use default credentials:
- Change username to something non-obvious
- Use strong password (16+ characters, mixed case, numbers, symbols)
- Consider using environment variables for credentials
- Implement IP-based access restrictions
- Use certificate-based authentication for APIs

### Changing Default Credentials

**Recommended for production:**

```yaml
# In cloud-init/webserver.yaml
- HASH=$(caddy hash-password --plaintext 'YOUR_STRONG_PASSWORD_HERE')

# In Caddyfile
basicauth {
  your_custom_username $HASH
}
```

## Integration with Deployment

Basic auth doesn't affect deployment:

```bash
# Deploy files via SSH/rsync (not HTTP)
make deploy CONFIG=er.json
# Works normally - uses SSH authentication

# Access site via HTTPS
curl -u emerging:emerging2026 https://yoursite.com/
# Requires basic auth
```

Deployment uses SSH (unaffected), web access uses HTTP basic auth.

## Summary

**Current Setup:**
- ✅ Username: `emerging`
- ✅ Password: `emerging2026`
- ✅ All domains protected
- ✅ HTTPS required
- ✅ Configured automatically during instance creation

**To Access:**
- Browser: Will prompt automatically
- curl: `curl -u emerging:emerging2026 https://yoursite.com/`
- API: Include `Authorization: Basic` header

**To Change:**
- Edit `cloud-init/webserver.yaml` before creating instance
- Or update Caddyfile manually on running instance

The setup is automatic and works immediately after instance creation.
