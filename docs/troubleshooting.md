# Troubleshooting

Common issues and solutions when using Cloister.

## Installation Issues

### "go install: command not found"

Go is not installed or not in your PATH.

**Solution:**
1. Install Go from https://go.dev/dl/
2. Ensure `$GOPATH/bin` is in your PATH:
   ```bash
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

### "docker: command not found"

Docker is not installed.

**Solution:**
- **macOS:** Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) or [OrbStack](https://orbstack.dev/)
- **Linux:** Install Docker Engine per [official docs](https://docs.docker.com/engine/install/)

## Startup Issues

### "Cannot connect to the Docker daemon"

Docker is installed but not running.

**Solution:**
```bash
# Linux
sudo systemctl start docker

# macOS
# Open Docker Desktop or OrbStack application
```

### "Guardian failed to start"

The guardian container couldn't be created.

**Check:**
1. Docker is running: `docker info`
2. Ports aren't in use: `lsof -i :9997` and `lsof -i :9999`
3. Network exists: `docker network ls | grep cloister`

**Reset if needed:**
```bash
docker rm -f cloister-guardian
docker network rm cloister-net
cloister guardian start
```

### "Project not detected"

Not in a git repository, or git isn't initialized.

**Solution:**
```bash
# Initialize git if needed
git init

# Then try again
cloister start
```

## Network Issues

### "Domain not in allowlist"

The container tried to reach a domain that isn't allowed.

**Solutions:**

1. Add to project config:
   ```yaml
   # ~/.config/cloister/projects/my-app.yaml
   proxy:
     allow:
       - domain: docs.example.com
   ```

2. Add to global config for all projects:
   ```yaml
   # ~/.config/cloister/config.yaml
   proxy:
     allow:
       - domain: docs.example.com
   ```

3. Enable approval mode for unlisted domains:
   ```yaml
   # ~/.config/cloister/config.yaml
   proxy:
     unlisted_domain_behavior: request_approval
   ```

### "Connection refused" inside container

Proxy isn't reachable.

**Check:**
```bash
# Inside cloister
echo $HTTPS_PROXY
# Should show http://token:TOKEN@cloister-guardian:3128

# Test proxy connectivity
curl -v https://api.anthropic.com/v1/models
```

**If proxy URL is empty:** Container wasn't started correctly. Exit and restart:
```bash
exit
cloister stop
cloister start
```

### Package installation fails

The package registry domain might not be allowlisted.

**Common registries to allowlist:**
```yaml
proxy:
  allow:
    # npm
    - domain: registry.npmjs.org
    - domain: registry.yarnpkg.com

    # Python
    - domain: pypi.org
    - domain: files.pythonhosted.org

    # Go
    - domain: proxy.golang.org
    - domain: sum.golang.org

    # Rust
    - domain: crates.io
    - domain: static.crates.io
```

## Credential Issues

### Claude prompts for login inside container

Credentials weren't injected properly.

**Solution:**
1. Re-run setup: `cloister setup claude`
2. Restart the cloister:
   ```bash
   cloister stop
   cloister start
   ```

### "Authentication failed" with Claude

Token may have expired.

**Solution:**
1. Get a fresh token:
   ```bash
   claude setup-token
   ```
2. Update Cloister:
   ```bash
   cloister setup claude
   ```
3. Restart the cloister

### OAuth token expired

OAuth tokens last about a year. If expired:

```bash
claude setup-token  # Get new token (Claude Code CLI command)
cloister setup claude  # Update config
cloister stop && cloister start  # Restart
```

## Hostexec Issues

### Request not appearing in approval UI

**Check:**
1. Guardian is running: `cloister guardian status`
2. UI is accessible: Open http://localhost:9999
3. Browser console for errors (SSE connection issues)

**Try:**
- Refresh the approval UI page
- Restart guardian: `cloister guardian stop && cloister guardian start`

### Command denied unexpectedly

Commands not matching any pattern are denied by default.

**Check patterns:**
```bash
cloister config show
```

Look at `hostexec.auto_approve` and `hostexec.manual_approve` patterns.

### Hostexec timeout

Requests timeout after 5 minutes without approval.

**Solutions:**
- Approve faster
- Add frequently-used commands to auto-approve:
  ```yaml
  hostexec:
    auto_approve:
      - pattern: "^git push origin"
  ```

## Container Issues

### Container won't start

**Check Docker logs:**
```bash
docker logs cloister-my-app
```

**Common causes:**
- Port conflict
- Volume mount failure
- Image not found

**Reset:**
```bash
cloister stop my-app
docker rm -f cloister-my-app  # Force remove if needed
cloister start
```

### Files missing in /work

Bind mount may have failed.

**Verify mount:**
```bash
# Inside cloister
mount | grep /work
```

**If empty:** Container needs restart with correct mount.

### Permission denied on files

Container user may differ from host user.

**Check:**
```bash
# Inside cloister
id
ls -la /work
```

## Performance Issues

### Slow container startup

First run downloads the image. Subsequent starts should be faster.

**Check image exists:**
```bash
docker images | grep cloister
```

### Slow network inside container

Proxy overhead is minimal but DNS resolution may add latency.

## Getting Help

### Checking versions

```bash
cloister --version
docker --version
go version
```

### Reporting issues

- [GitHub Issues](https://github.com/xdg/cloister/issues) — Report bugs and request features
- [GitHub Discussions](https://github.com/xdg/cloister/discussions) — Ask questions and share tips

When filing an issue, include:
- Cloister version
- OS and Docker version
- Steps to reproduce
- Error messages
- Relevant config (redact credentials)

## Next Steps

- [Configuration](configuration.md) — Config file reference
- [Command Reference](command-reference.md) — All CLI commands
