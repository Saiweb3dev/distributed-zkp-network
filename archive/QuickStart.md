# Distributed ZKP Network - Quick Start Guide

## 🚀 Getting Started in 5 Minutes

### Prerequisites
- Go 1.22+
- Docker & Docker Compose
- Git

### Setup & Run

```bash
# 1. Clone and setup
git clone <your-repo>
cd distributed-zkp-network
chmod +x setup.sh
./setup.sh

# 2. Start infrastructure
docker-compose -f deployments/docker/docker-compose.yml up -d

# 3. Run the API Gateway
go run cmd/api-gateway/main.go --config configs/api-gateway.yaml
```

You should see:
```
INFO  Starting ZKP Network API Gateway
INFO  Prover initialized successfully
INFO  Server started successfully  address=0.0.0.0:8080
```

### Test It Works

```bash
# Health check
curl http://localhost:8080/health

# Generate a Merkle proof
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -H "Content-Type: application/json" \
  -d '{
    "leaves": [
      "0x1111111111111111",
      "0x2222222222222222", 
      "0x3333333333333333",
      "0x4444444444444444"
    ],
    "leaf_index": 2
  }'
```

Expected response:
```json
{
  "success": true,
  "proof": "0x3a2b1c...",
  "merkle_root": "0x9f8e7d...",
  "generation_time": "2.5s",
  "circuit_type": "*circuits.MerkleProofCircuitInputs"
}
```

---

## 📚 Understanding the Code

### Architecture Overview

```
Request Flow:
HTTP Request → Router → Middleware → Handler → Prover → gnark → Response

Layer Breakdown:
┌─────────────────────────────────────────┐
│  HTTP Layer (handlers, router)          │  ← You are here
├─────────────────────────────────────────┤
│  Business Logic (Merkle tree building)  │
├─────────────────────────────────────────┤
│  ZKP Layer (prover wrapper)             │
├─────────────────────────────────────────┤
│  gnark Library (circuits, proving)      │
└─────────────────────────────────────────┘
```

### Key Files Explained

#### 1. **`internal/common/config/config.go`** - Configuration Management
**What it does:** Loads settings from YAML files and environment variables.

**Key concepts:**
- **Viper**: Industry-standard config library
- **Validation**: Checks config at startup (fail fast)
- **Environment priority**: ENV vars override YAML

**Try this:**
```bash
# Override config via environment variable
API_GATEWAY_SERVER_PORT=9999 go run cmd/api-gateway/main.go
```

**Learning points:**
- Struct tags (`mapstructure`) map YAML keys to Go fields
- `setDefaults()` ensures safe fallbacks
- `Validate()` catches errors early

---

#### 2. **`internal/zkp/circuits/example_circuit.go`** - ZKP Circuits

**What it does:** Defines the mathematical constraints for zero-knowledge proofs.

**The Merkle Circuit Explained:**

```go
// This circuit proves: "I know a leaf in the Merkle tree"
// WITHOUT revealing which leaf or the path

Inputs:
  - Leaf (secret): The value we're proving exists
  - Path (secret): Sibling hashes to reconstruct root
  - PathIndices (secret): Left (0) or right (1) at each level
  - Root (public): Everyone knows this, we prove we can recreate it

Constraints:
  1. Start with leaf hash
  2. For each level, hash with sibling (order depends on index)
  3. Final computed hash MUST equal public root
```

**Visual example:**
```
Tree:              Proof for Leaf2:
    ROOT           Need: [Leaf1, Hash34, ROOT]
   /    \          Indices: [1, 0] (right, then left)
 H12    H34        
 / \    / \        Verification:
L1 L2  L3 L4       1. Hash(Leaf1, Leaf2) = H12 ✓
   ^              2. Hash(H12, H34) = ROOT ✓
   |
  This one!
```

**Try this experiment:**
```go
// In the circuit, try changing:
api.AssertIsEqual(currentHash, circuit.Root)
// to:
api.AssertIsEqual(currentHash, frontend.Variable(0))

// What happens? Proof generation will FAIL
// Why? The constraint can't be satisfied
```

**Learning points:**
- `frontend.Variable` = placeholder for values
- `api.Add/Mul/Select` = create constraints, not compute
- Circuit runs during both proving AND verifying

---

#### 3. **`internal/zkp/prover.go`** - Proof Generation Wrapper

**What it does:** Wraps gnark's complex API into something simple.

**The compilation cache explained:**

```
First proof:  Compile (30s) + Prove (2s) = 32s  😱
Second proof: Prove (2s) = 2s ✨ (cached!)
```

**How caching works:**
```go
// sync.Map = thread-safe map
// Key = circuit type (e.g., "MerkleProof")
// Value = compiled R1CS + proving/verifying keys

getOrCompileCircuit():
  1. Check cache → Found? Return it
  2. Not found? Acquire lock
  3. Double-check (another thread might have compiled)
  4. Compile + generate keys (slow!)
  5. Store in cache
  6. Return
```

**Try this:**
```bash
# First request takes ~30s (compilation)
time curl -X POST http://localhost:8080/api/v1/proofs/merkle -d '...'

# Second request takes ~2s (cached)
time curl -X POST http://localhost:8080/api/v1/proofs/merkle -d '...'
```

**Learning points:**
- Circuit compilation is ONE-TIME per circuit type
- R1CS = Rank-1 Constraint System (math representation)
- Proving key (pk) = secret, Verifying key (vk) = public

---

#### 4. **`internal/api/handlers/proof_handler.go`** - HTTP Business Logic

**What it does:** Connects HTTP requests to ZKP operations.

**Request processing flow:**

```
1. Parse JSON → MerkleProofRequest struct
2. Validate (leaf_index in range? leaves < 1024?)
3. Convert hex strings → bytes
4. Build Merkle tree from leaves
5. Generate Merkle proof path
6. Create circuit witness (fill in secret values)
7. Call prover.GenerateProof()
8. Return proof + metadata as JSON
```

**Error handling strategy:**
```go
Validation errors    → 400 Bad Request
  (client's fault)

Proof gen failures   → 500 Internal Server Error
  (our fault)

Timeouts             → 504 Gateway Timeout
  (too slow)
```

**Try breaking things:**
```bash
# Send invalid leaf index (should return 400)
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -d '{"leaves": ["0x11", "0x22"], "leaf_index": 999}'

# Send too many leaves (should return 400)
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -d '{"leaves": ['$(python3 -c 'print("[\"0x11\"]," * 2000)')'], "leaf_index": 0}'
```

**Learning points:**
- Validate early, fail fast
- Separate parsing from business logic
- Always return meaningful errors

---

#### 5. **`cmd/api-gateway/main.go`** - Application Wiring

**What it does:** Brings all components together.

**Bootstrap sequence:**

```
1. Parse flags        (--config path)
2. Load config        (YAML + ENV vars)
3. Init logger        (structured logging)
4. Init prover        (ZKP engine)
5. Init handlers      (HTTP layer)
6. Setup router       (routes + middleware)
7. Start server       (in goroutine)
8. Wait for signal    (SIGINT/SIGTERM)
9. Graceful shutdown  (finish requests, then exit)
```

**Dependency injection pattern:**
```go
// ❌ DON'T DO THIS (global state, hard to test)
var globalProver *zkp.Prover

func main() {
    globalProver = zkp.NewProver()
    handler := NewHandler() // implicitly uses globalProver
}

// ✅ DO THIS (inject dependencies)
func main() {
    prover := zkp.NewProver()
    handler := NewHandler(prover) // explicit dependency
}

// Why? Testing:
func TestHandler(t *testing.T) {
    mockProver := &MockProver{} // fake for testing
    handler := NewHandler(mockProver)
    // Now we can test handler in isolation!
}
```

**Graceful shutdown explained:**
```go
Server receives SIGTERM
  ↓
srv.Shutdown(ctx) called
  ↓
Server stops accepting NEW requests
  ↓
Waits up to 30s for IN-FLIGHT requests
  ↓
Closes all connections
  ↓
Process exits

Why? No dropped requests, no corrupted data
```

**Try this:**
```bash
# Start server
go run cmd/api-gateway/main.go

# In another terminal, send a long request
curl -X POST http://localhost:8080/api/v1/proofs/merkle -d '...' &

# Quickly press Ctrl+C in server terminal
# Watch the logs - it waits for the request to finish!
```

**Learning points:**
- No globals = testable code
- Start server in goroutine = can handle signals
- Context with timeout = controlled shutdown

---

## 🐳 Docker Deep Dive

### Dockerfile Best Practices

**Multi-stage build benefits:**
```
Builder stage:  1.2 GB (Go compiler, tools)
Final stage:    18 MB  (only binary + alpine)

Why? Faster pulls, smaller attack surface
```

**Layer caching:**
```dockerfile
COPY go.mod go.sum ./    ← Layer 1 (rarely changes)
RUN go mod download       ← Layer 2 (cached if Layer 1 unchanged)

COPY . .                  ← Layer 3 (changes often)
RUN go build              ← Layer 4 (only rebuilds if source changed)
```

**Try this:**
```bash
# First build (no cache)
time docker build -f deployments/docker/Dockerfile.api-gateway -t zkp:v1 .

# Change a source file
echo "// comment" >> internal/api/handlers/proof_handler.go

# Second build (uses cache for layers 1-2)
time docker build -f deployments/docker/Dockerfile.api-gateway -t zkp:v2 .

# Much faster! 🚀
```

---

## 🔍 Debugging & Troubleshooting

### Common Issues

**1. "Failed to compile circuit"**
```
Cause: Circuit constraints are invalid
Fix: Check Define() method logic
Debug: Add logging before compilation
```

**2. "Proof generation timeout"**
```
Cause: First proof compiles circuit (slow)
Fix: Pre-warm cache or increase timeout
Debug: Check generation_time in response
```

**3. "Invalid leaf index"**
```
Cause: Client sent index >= len(leaves)
Fix: Client-side validation
Debug: Check request payload
```

### Logging Examples

```bash
# View all logs
docker-compose -f deployments/docker/docker-compose.yml logs -f

# View only API Gateway logs
docker-compose -f deployments/docker/docker-compose.yml logs -f api-gateway

# Search for errors
docker-compose -f deployments/docker/docker-compose.yml logs api-gateway | grep ERROR

# Watch live requests
docker-compose -f deployments/docker/docker-compose.yml logs -f api-gateway | grep "HTTP request"
```

---

## 🧪 Testing Your Changes

### Manual Testing Workflow

```bash
# 1. Make a change to handler
vim internal/api/handlers/proof_handler.go

# 2. Rebuild (fast - uses cached layers)
docker-compose -f deployments/docker/docker-compose.yml up -d --build api-gateway

# 3. Test the endpoint
curl -X POST http://localhost:8080/api/v1/proofs/merkle -d '...'

# 4. Check logs
docker-compose logs -f api-gateway
```

### Testing Different Scenarios

```bash
# Small tree (2 leaves)
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -H "Content-Type: application/json" \
  -d '{"leaves": ["0xaa", "0xbb"], "leaf_index": 0}'

# Larger tree (8 leaves)
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -H "Content-Type: application/json" \
  -d '{"leaves": ["0x11","0x22","0x33","0x44","0x55","0x66","0x77","0x88"], "leaf_index": 5}'

# Edge case: Single leaf
curl -X POST http://localhost:8080/api/v1/proofs/merkle \
  -H "Content-Type: application/json" \
  -d '{"leaves": ["0xdeadbeef"], "leaf_index": 0}'
```

---

## 📈 Next Steps for Learning

### Week 1: Understand the Basics
- [ ] Trace a request from curl to gnark and back
- [ ] Modify the Merkle circuit to log intermediate hashes
- [ ] Add a `/debug` endpoint that shows circuit stats

### Week 2: Add Features
- [ ] Implement the Addition circuit endpoint
- [ ] Add proof verification endpoint
- [ ] Add metrics (Prometheus counters for proofs generated)

### Week 3: Production Hardening
- [ ] Add unit tests for handlers
- [ ] Add integration tests (TestMain pattern)
- [ ] Add rate limiting middleware
- [ ] Add authentication (JWT)

### Week 4: Distributed Systems
- [ ] Add PostgreSQL persistence for proofs
- [ ] Add Redis caching for Merkle roots
- [ ] Add coordinator node (basic version)

---

## 🎯 Challenge Exercises

### Beginner
1. Add a new endpoint `/api/v1/proofs/addition` for the Addition circuit
2. Make the Merkle tree depth configurable via request
3. Add request ID logging for tracing

### Intermediate
4. Implement proof verification endpoint
5. Add Prometheus metrics for proof generation time
6. Cache Merkle tree roots in Redis

### Advanced
7. Support different hash functions (MiMC vs Poseidon)
8. Implement batch proof generation
9. Add PLONK backend support (in addition to Groth16)

---

## 📖 Further Reading

- [gnark Documentation](https://docs.gnark.consensys.net/)
- [Groth16 Paper](https://eprint.iacr.org/2016/260.pdf)
- [Merkle Trees Explained](https://brilliant.org/wiki/merkle-tree/)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)

---

## 🤝 Contributing

This is a learning project! Feel free to:
- Break things and learn from failures
- Add comments explaining what you learned
- Try alternative implementations
- Ask questions in issues

---

## 📝 License

MIT License - See LICENSE file for details