# Testing Guide

This guide covers how to run tests in Foundry, including unit tests and integration tests with testcontainers.

## Prerequisites

- **Go 1.25+** installed
- **Docker** running (for integration tests)
- **Git** for cloning the repository

## Test Structure

Foundry has two types of tests:

### 1. Unit Tests
- Located alongside source files (`*_test.go`)
- Fast, no external dependencies
- Run by default with `go test`

### 2. Integration Tests
- Located in `v1/test/integration/`
- Use testcontainers for real SSH servers
- Require Docker to be running
- Skip in short mode (`-short` flag)
- Use build tag `integration`

## Running Tests

### Quick Test (Unit Tests Only)

```bash
cd v1

# Run all unit tests
go test ./... -short

# With coverage
go test ./... -short -cover

# Verbose output
go test ./... -short -v
```

### Full Test Suite (Unit + Integration)

```bash
cd v1

# Method 1: Without -short flag (integration tests run)
go test ./...

# Method 2: With integration build tag
go test -tags=integration ./...

# With coverage
go test ./... -cover

# Verbose output
go test ./... -v
```

### Integration Tests Only

```bash
cd v1

# Run just integration tests
go test ./test/integration/...

# With verbose output
go test -v ./test/integration/...

# With build tag
go test -tags=integration -v ./test/integration/...
```

### Using the Tools Script

```bash
cd v1

# Run unit tests
./tools test

# Run with coverage report
./tools coverage

# Run integration tests
./tools test-integration
```

## Integration Test Details

### What They Test

The integration tests (`test/integration/phase1_test.go`) verify:

1. **Config Management**
   - Creating and validating configurations
   - Secret reference syntax validation

2. **SSH Operations**
   - Connecting to real SSH server (in container)
   - Key generation and installation
   - Command execution

3. **Host Registry**
   - Adding, listing, updating hosts
   - Thread-safe operations

4. **Secret Resolution**
   - Environment variable resolution
   - ~/.foundryvars file resolution
   - Instance-scoped secret paths

### How They Work

```go
// 1. Start an SSH container
container, port, cleanup := setupSSHContainer(t, ctx)
defer cleanup()

// 2. Run tests against the container
testSSHConnection(t, port)
testKeyGeneration(t)
testSSHExecution(t, container, port, keyPair)

// 3. Container is automatically cleaned up
```

### SSH Container Details

The integration tests use `linuxserver/openssh-server`:
- Runs OpenSSH server on port 2222
- Default user: `testuser`
- Default password: `testpass`
- Supports password and key authentication
- Automatically cleaned up after tests

### Running Integration Tests Manually

```bash
cd v1

# 1. Ensure Docker is running
docker info

# 2. Run integration tests
go test ./test/integration/... -v

# Expected output:
# === RUN   TestPhase1Workflow
# Starting SSH test container...
# Testing config creation and validation...
# ✓ Config validation successful
# Testing SSH connection...
# ✓ SSH connection successful
# ...
# --- PASS: TestPhase1Workflow (15.23s)
# PASS
```

## Test Coverage

### Viewing Coverage

```bash
cd v1

# Generate coverage report
go test ./... -short -coverprofile=coverage.out

# View in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# Open in browser
xdg-open coverage.html  # Linux
open coverage.html      # macOS
```

### Current Coverage

As of Phase 1 completion:

```
Package                                          Coverage
-------------------------------------------------------
cmd/foundry/commands/config                      77.3%
cmd/foundry/commands/host                        22.1% (unit only)
internal/config                                  83.3%
internal/host                                    100.0%
internal/secrets                                 89.1%
internal/ssh                                     62.7%
```

**Note**: Host command coverage is lower because full testing is done in integration tests.

## Troubleshooting

### Integration Tests Fail to Start Container

**Problem**: "Cannot connect to Docker daemon"

**Solution**:
```bash
# Check Docker is running
docker info

# Start Docker (Linux)
sudo systemctl start docker

# Start Docker Desktop (macOS/Windows)
# Open Docker Desktop application
```

### Integration Tests Timeout

**Problem**: Container takes too long to start

**Solution**:
```bash
# Pull the image first
docker pull linuxserver/openssh-server:latest

# Then run tests
go test ./test/integration/... -v
```

### Port Already in Use

**Problem**: "bind: address already in use"

**Solution**:
Testcontainers automatically maps to random available ports, but if issues persist:

```bash
# Find and kill the process using the port
lsof -i :2222
kill -9 <PID>

# Or restart Docker
docker restart
```

### Tests Pass Locally but Fail in CI

**Problem**: Integration tests fail in CI environment

**Solution**:
CI environments need Docker-in-Docker or Docker socket access:

```yaml
# GitHub Actions example
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      docker:
        image: docker:dind
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test ./...
```

## Writing New Tests

### Unit Test Template

```go
package mypackage

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test"

    // Act
    result := MyFunction(input)

    // Assert
    assert.Equal(t, "expected", result)
}

func TestMyFunction_ErrorCase(t *testing.T) {
    // Test error paths
    _, err := MyFunction("")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "expected message")
}
```

### Integration Test Template

```go
// +build integration

package integration

import (
    "context"
    "testing"
    "github.com/testcontainers/testcontainers-go"
    "github.com/stretchr/testify/require"
)

func TestMyIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    ctx := context.Background()

    // Start container
    req := testcontainers.ContainerRequest{
        Image: "myimage:latest",
        // ... configuration ...
    }

    container, err := testcontainers.GenericContainer(ctx,
        testcontainers.GenericContainerRequest{
            ContainerRequest: req,
            Started:          true,
        })
    require.NoError(t, err)
    defer container.Terminate(ctx)

    // Run tests against container
    // ...
}
```

## Best Practices

### 1. Use Table-Driven Tests

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    error
    }{
        {"valid", "test", nil},
        {"empty", "", ErrEmpty},
        {"invalid", "!@#", ErrInvalid},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Validate(tt.input)
            assert.Equal(t, tt.want, err)
        })
    }
}
```

### 2. Clean Up Resources

```go
func TestWithCleanup(t *testing.T) {
    resource := setupResource(t)
    defer resource.Cleanup() // Always clean up

    // Test with resource
}
```

### 3. Use Subtests for Organization

```go
func TestConfig(t *testing.T) {
    t.Run("validation", func(t *testing.T) {
        // Validation tests
    })

    t.Run("loading", func(t *testing.T) {
        // Loading tests
    })
}
```

### 4. Test Both Happy and Error Paths

```go
func TestOperation(t *testing.T) {
    // Happy path
    result, err := Operation("valid")
    require.NoError(t, err)
    assert.Equal(t, expected, result)

    // Error path
    _, err = Operation("invalid")
    require.Error(t, err)
}
```

### 5. Use Test Fixtures

```go
// test/fixtures/valid-config.yaml
version: v1
cluster:
  name: test
  # ...

// In test:
cfg, err := config.Load("../fixtures/valid-config.yaml")
require.NoError(t, err)
```

## Continuous Integration

### GitHub Actions

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'

      - name: Run unit tests
        run: go test -v -short -cover ./...
        working-directory: v1

      - name: Run integration tests
        run: go test -v ./...
        working-directory: v1

      - name: Generate coverage
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out
        working-directory: v1
```

## Performance Testing

For performance-critical code:

```go
func BenchmarkOperation(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Operation("input")
    }
}

// Run with:
// go test -bench=. -benchmem
```

## Next Steps

- Write tests for new features before implementing
- Aim for >80% coverage on new code
- Add integration tests for complex workflows
- Run tests before committing
- Check CI results on PRs

## Resources

- [Go Testing](https://golang.org/pkg/testing/)
- [Testify](https://github.com/stretchr/testify)
- [Testcontainers](https://golang.testcontainers.org/)
