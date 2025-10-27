# ZKP Network Developer Cheatsheet

## 🚀 Quick Commands

```bash
# Setup & Dependencies
./setup.sh                          # Initial project setup
go mod tidy                         # Clean dependencies
go mod download                     # Download dependencies

# Development
go run cmd/api-gateway/main.go      # Run API Gateway
go run cmd/api-gateway/main.go --config configs/dev.yaml  # Custom config

# Docker
docker-compose -f deployments/docker/docker-compose.yml up -d       # Start all
docker-compose -f deployments/docker/docker-compose.yml down        # Stop all
docker-compose -f deployments/docker/docker-compose.yml logs -f api-gateway  # View logs
docker-compose -f deployments/docker/docker-compose.yml up -d --build api-gateway  # Rebuild

# Testing
curl http://localhost:8080/health                          # Health check
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -H "Content-Type: application/json" \
  -d '{"leaves":["0x11","0x22","0x33"],"leaf_index":1}'   # Generate proof
```

---

## 🏗️ Project Structure

```
distributed-zkp-network/
├── cmd/api-gateway/main.go          # ← Application entry point
├── internal/
│   ├── common/config/               # ← Configuration management
│   ├── zkp/
│   │   ├── circuits/                # ← ZKP circuit definitions
│   │   └── prover.go                # ← Proof generation wrapper
│   └── api/
│       ├── handlers/                # ← HTTP request handlers
│       ├── middleware/              # ← HTTP middleware
│       └── router/                  # ← Route definitions
├── configs/                         # ← YAML config files
└── deployments/docker/              # ← Docker configs
```

---

## 🔑 Key Concepts

### Circuit → Witness → Proof

```
Circuit: The rules (constraints)
  Example: "A + B = C"

Witness: Specific values that satisfy the rules
  Example: A=5, B=3, C=8

Proof: Cryptographic proof that witness satisfies circuit
  Size: ~200 bytes
  Verification: Milliseconds
```

### Public vs Private Inputs

```go
type ExampleCircuit struct {
    Secret  frontend.Variable `gnark:",secret"`   // Hidden from verifier
    Public  frontend.Variable `gnark:",public"`   // Known to verifier
}

// Verifier knows: Public input
// Verifier doesn't know: Secret input
// Proof: "I know a Secret that satisfies the constraints"
```

### Merkle Tree Depth

```
Depth 3 → 2^3 = 8 leaves
Depth 10 → 2^10 = 1,024 leaves
Depth 20 → 2^20 = 1,048,576 leaves

Proof size: Always = Depth * 32 bytes
```

---

## 🐛 Debugging Checklist

**Proof generation fails?**
- [ ] Check circuit Define() logic
- [ ] Verify witness values are in field
- [ ] Check logs for compilation errors
- [ ] Ensure constraints are satisfiable

**HTTP request hangs?**
- [ ] First request compiles circuit (30s normal)
- [ ] Check server timeout configuration
- [ ] Verify circuit cache is working
- [ ] Look for goroutine leaks

**"Invalid config" error?**
- [ ] Run config.Validate()
- [ ] Check YAML syntax
- [ ] Verify environment variable format
- [ ] Check for required fields

---

## 📊 Response Times

```
First proof:  ~30s  (compilation + proving)
Cached proof: ~2s   (only proving)
Health check: <1ms  (just HTTP)
```

---

## 🎯 Common Patterns

### Error Handling
```go
// ❌ Don't
if err != nil {
    panic(err)
}

// ✅ Do
if err != nil {
    logger.Error("operation failed", zap.Error(err))
    return fmt.Errorf("failed to do X: %w", err)
}
```

### Logging
```go
// ❌ Don't
log.Println("Request received")

// ✅ Do
logger.Info("HTTP request received",
    zap.String("path", r.URL.Path),
    zap.String("method", r.Method),
    zap.Duration("duration", time.Since(start)),
)
```

### Context Usage
```go
// ❌ Don't
func DoWork() {
    time.Sleep(30 * time.Second)
}

// ✅ Do
func DoWork(ctx context.Context) error {
    select {
    case <-time.After(30 * time.Second):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

---

## 🧪 Test Scenarios

### Valid Requests
```bash
# 2 leaves
{"leaves":["0x11","0x22"],"leaf_index":0}

# 4 leaves
{"leaves":["0x11","0x22","0x33","0x44"],"leaf_index":2}

# 8 leaves (depth = 3)
{"leaves":["0x11","0x22","0x33","0x44","0x55","0x66","0x77","0x88"],"leaf_index":5}
```

### Invalid Requests (should return 400)
```bash
# Empty leaves
{"leaves":[],"leaf_index":0}

# Invalid index
{"leaves":["0x11","0x22"],"leaf_index":99}

# Too many leaves
{"leaves":["0x11" x 2000],"leaf_index":0}

# Invalid hex
{"leaves":["not-hex"],"leaf_index":0}
```

---

## 🔧 Configuration Override

```bash
# Via environment variable
API_GATEWAY_SERVER_PORT=9999 go run cmd/api-gateway/main.go

# Via flag
go run cmd/api-gateway/main.go --config configs/custom.yaml

# Multiple overrides
API_GATEWAY_LOGGING_LEVEL=debug \
API_GATEWAY_SERVER_PORT=9999 \
go run cmd/api-gateway/main.go
```

---

## 📝 Useful gnark APIs

```go
// Define constraints
api.Add(a, b)           // a + b
api.Mul(a, b)           // a * b
api.Sub(a, b)           // a - b
api.Div(a, b)           // a / b
api.Select(cond, a, b)  // if cond then a else b

// Assertions
api.AssertIsEqual(a, b)     // a == b
api.AssertIsDifferent(a, b) // a != b
api.AssertIsBoolean(a)      // a ∈ {0, 1}

// Bit operations
bits := api.ToBinary(x, n)  // Convert to n bits
x := api.FromBinary(bits)   // Reconstruct from bits
```

---

## 🌐 API Endpoints

```
GET  /                        # Root (version info)
GET  /health                  # Health check

POST /api/v1/proofs/merkle    # Generate Merkle proof
```

---

## 📦 Dependencies

```
github.com/consensys/gnark              # ZKP library
github.com/gorilla/mux                  # HTTP router
github.com/spf13/viper                  # Configuration
go.uber.org/zap                         # Logging
github.com/prometheus/client_golang    # Metrics
```

---

## 🎨 Code Style

```go
// Package comment
package handlers

// Import groups: stdlib, external, internal
import (
    "fmt"
    "net/http"
    
    "go.uber.org/zap"
    
    "github.com/saiweb3dev/distributed-zkp-network/internal/zkp"
)

// Exported function (uppercase)
func NewHandler() *Handler { ... }

// Unexported function (lowercase)
func validateInput() error { ... }

// Method receiver: single letter or abbrev
func (h *Handler) Handle() { ... }
```

---

## 💡 Remember

1. **First proof is slow** (compilation) - this is normal
2. **Use structured logging** (zap fields, not printf)
3. **Validate early** (bad input? fail fast)
4. **Inject dependencies** (no globals)
5. **Context everywhere** (for cancellation)
6. **Graceful shutdown** (finish requests before exit)

---

## 🆘 When Stuck

1. Check logs: `docker-compose logs -f api-gateway`
2. Verify config: `cat configs/api-gateway.yaml`
3. Test health: `curl http://localhost:8080/health`
4. Read error message carefully
5. Add debug logging
6. Check this cheatsheet again!

---

**Last Updated:** 2025-10-23  
**Version:** 0.1.0