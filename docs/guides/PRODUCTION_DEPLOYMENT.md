# Production Deployment Guide: Multi-Machine Raft Cluster

## Overview

This guide covers deploying a 3-node Raft coordinator cluster across separate physical or virtual machines in production.

---

## 🏗️ Architecture

### Cluster Layout
```
                          Load Balancer (HAProxy/Nginx)
                                    ↓
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
    Machine 1                  Machine 2                  Machine 3
 192.168.1.10              192.168.1.11              192.168.1.12
┌─────────────┐          ┌─────────────┐          ┌─────────────┐
│Coordinator-1│◄────────►│Coordinator-2│◄────────►│Coordinator-3│
│  (Leader)   │   Raft   │ (Follower)  │   Raft   │ (Follower)  │
└─────────────┘          └─────────────┘          └─────────────┘
       │                        │                        │
       └────────────────────────┴────────────────────────┘
                          Shared PostgreSQL
                      (AWS RDS / Google CloudSQL)
```

### Port Requirements
| Port | Protocol | Purpose | Must Be Open |
|------|----------|---------|--------------|
| 7000 | TCP | Raft Consensus | Between coordinators |
| 9090 | TCP | gRPC (Workers/API) | From workers/API gateway |
| 8090 | TCP | HTTP (Health/Metrics) | From load balancer |
| 5432 | TCP | PostgreSQL | From all coordinators |

---

## 🚀 Deployment Steps

### Step 1: Prepare Each Machine

**Install Docker (on all 3 machines):**
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Start Docker
sudo systemctl start docker
sudo systemctl enable docker

# Add user to docker group
sudo usermod -aG docker $USER
```

**Open Required Ports (on all 3 machines):**
```bash
# Using UFW (Ubuntu)
sudo ufw allow 7000/tcp comment "Raft consensus"
sudo ufw allow 9090/tcp comment "gRPC"
sudo ufw allow 8090/tcp comment "HTTP health"

# Or using firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-port=7000/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --permanent --add-port=8090/tcp
sudo firewall-cmd --reload
```

---

### Step 2: Deploy Database (Shared)

**Option A: Managed Database (Recommended)**
```bash
# AWS RDS PostgreSQL
# Google Cloud SQL
# Azure Database for PostgreSQL

# Get connection string:
# postgres.prod.example.com:5432
```

**Option B: Self-hosted PostgreSQL**
```bash
# On a separate machine (192.168.1.5)
docker run -d \
  --name zkp-postgres \
  -e POSTGRES_DB=zkp_network \
  -e POSTGRES_USER=zkp_user \
  -e POSTGRES_PASSWORD=secure_password \
  -p 5432:5432 \
  -v postgres_data:/var/lib/postgresql/data \
  postgres:15-alpine

# Update configs to use: 192.168.1.5:5432
```

---

### Step 3: Deploy Coordinator-1 (Machine 1: 192.168.1.10)

**1. Copy project to machine:**
```bash
# On your local machine
scp -r distributed-zkp-network user@192.168.1.10:~/

# SSH to machine
ssh user@192.168.1.10
cd ~/distributed-zkp-network
```

**2. Set environment variables:**
```bash
# Create .env file
cat > .env << EOF
DB_PASSWORD=your_secure_password
COORDINATOR_ID=coordinator-1
RAFT_NODE_ID=coordinator-1
EOF
```

**3. Build Docker image:**
```bash
docker build -f deployments/docker/Dockerfile.coordinator -t zkp-coordinator:latest .
```

**4. Run coordinator:**
```bash
docker run -d \
  --name zkp-coordinator-1 \
  --restart unless-stopped \
  -p 7000:7000 \
  -p 9090:9090 \
  -p 8090:8090 \
  -v /var/lib/zkp/raft:/var/lib/zkp/raft \
  -v $(pwd)/configs/prod/coordinator-1.yaml:/app/config.yaml \
  --env-file .env \
  zkp-coordinator:latest --config /app/config.yaml
```

**5. Verify startup:**
```bash
# Check logs
docker logs -f zkp-coordinator-1

# Look for:
# "Bootstrapping Raft cluster"
# "Raft cluster bootstrapped successfully"
# "entering leader state"

# Check health
curl http://localhost:8090/health
```

---

### Step 4: Deploy Coordinator-2 (Machine 2: 192.168.1.11)

**Wait for Coordinator-1 to become leader first!**

```bash
# On Machine 2
ssh user@192.168.1.11
cd ~/distributed-zkp-network

# Create .env
cat > .env << EOF
DB_PASSWORD=your_secure_password
COORDINATOR_ID=coordinator-2
RAFT_NODE_ID=coordinator-2
EOF

# Build and run
docker build -f deployments/docker/Dockerfile.coordinator -t zkp-coordinator:latest .

docker run -d \
  --name zkp-coordinator-2 \
  --restart unless-stopped \
  -p 7000:7000 \
  -p 9090:9090 \
  -p 8090:8090 \
  -v /var/lib/zkp/raft:/var/lib/zkp/raft \
  -v $(pwd)/configs/prod/coordinator-2.yaml:/app/config.yaml \
  --env-file .env \
  zkp-coordinator:latest --config /app/config.yaml

# Check logs
docker logs -f zkp-coordinator-2

# Should see: "entering follower state"
```

---

### Step 5: Deploy Coordinator-3 (Machine 3: 192.168.1.12)

```bash
# On Machine 3
ssh user@192.168.1.12
cd ~/distributed-zkp-network

# Same process as Coordinator-2
cat > .env << EOF
DB_PASSWORD=your_secure_password
COORDINATOR_ID=coordinator-3
RAFT_NODE_ID=coordinator-3
EOF

docker build -f deployments/docker/Dockerfile.coordinator -t zkp-coordinator:latest .

docker run -d \
  --name zkp-coordinator-3 \
  --restart unless-stopped \
  -p 7000:7000 \
  -p 9090:9090 \
  -p 8090:8090 \
  -v /var/lib/zkp/raft:/var/lib/zkp/raft \
  -v $(pwd)/configs/prod/coordinator-3.yaml:/app/config.yaml \
  --env-file .env \
  zkp-coordinator:latest --config /app/config.yaml

docker logs -f zkp-coordinator-3
```

---

## 🔍 Verification

### Check Cluster Health
```bash
# From any machine
curl http://192.168.1.10:8090/health | jq '.raft'
curl http://192.168.1.11:8090/health | jq '.raft'
curl http://192.168.1.12:8090/health | jq '.raft'

# Expected output:
# One node: "state": "Leader", "is_leader": true
# Two nodes: "state": "Follower", "is_leader": false
```

### Check Network Connectivity
```bash
# From Machine 1
telnet 192.168.1.11 7000  # Should connect
telnet 192.168.1.12 7000  # Should connect

# From Machine 2
telnet 192.168.1.10 7000
telnet 192.168.1.12 7000

# From Machine 3
telnet 192.168.1.10 7000
telnet 192.168.1.11 7000
```

---

## 🔧 How They Communicate

### 1. **Raft Consensus (Port 7000)**
```go
// When coordinator-1 wants to replicate a log entry:
// 1. Leader (coordinator-1) sends AppendEntries RPC to followers
conn := grpc.Dial("192.168.1.11:7000")  // To coordinator-2
conn := grpc.Dial("192.168.1.12:7000")  // To coordinator-3

// 2. Followers acknowledge
// 3. Once majority (2/3) ack, entry is committed
```

**What happens:**
- Leader sends heartbeats every 2 seconds
- If leader dies, followers start election after 3 seconds
- New leader elected with majority votes (2/3)

### 2. **Worker Communication (Port 9090)**
```go
// Workers connect to ANY coordinator
workerConn := grpc.Dial("192.168.1.10:9090")  // Try coordinator-1

// If not leader, coordinator redirects:
response := RegisterWorker(...)
if !response.Success {
    // Response contains: "Not leader, connect to: 192.168.1.11:9090"
    workerConn = grpc.Dial(response.LeaderAddress)
}
```

### 3. **Database Access (Port 5432)**
```go
// All coordinators connect to SAME PostgreSQL
db1 := postgres.Connect("postgres.prod.example.com:5432")
db2 := postgres.Connect("postgres.prod.example.com:5432")
db3 := postgres.Connect("postgres.prod.example.com:5432")

// Raft ensures only leader writes to DB
if raftNode.IsLeader() {
    db.Exec("INSERT INTO tasks ...")
}
```

---

## 🛡️ Security Considerations

### 1. **TLS/SSL for Raft**
```go
// Add to raft_node.go
tlsConfig := &tls.Config{
    Certificates: []tls.Certificate{cert},
    ClientAuth:   tls.RequireAndVerifyClientCert,
    ClientCAs:    caCertPool,
}

transport := raft.NewNetworkTransportWithConfig(&raft.NetworkTransportConfig{
    ServerAddressProvider: addressProvider,
    MaxPool:              3,
    Timeout:              10 * time.Second,
    TLSConfig:           tlsConfig,  // Add this
})
```

### 2. **Firewall Rules**
```bash
# Only allow Raft traffic from known coordinator IPs
sudo ufw allow from 192.168.1.10 to any port 7000
sudo ufw allow from 192.168.1.11 to any port 7000
sudo ufw allow from 192.168.1.12 to any port 7000
```

### 3. **Database Security**
```yaml
database:
  ssl_mode: "verify-full"  # Require SSL + verify cert
  password: "${DB_PASSWORD}"  # Never hardcode!
```

---

## 📊 Monitoring

### Health Checks
```bash
# Add to cron (every minute)
* * * * * curl -sf http://localhost:8090/health || systemctl restart zkp-coordinator
```

### Prometheus Metrics
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'zkp-coordinators'
    static_configs:
      - targets:
        - '192.168.1.10:9091'
        - '192.168.1.11:9091'
        - '192.168.1.12:9091'
```

### Log Aggregation
```bash
# Send logs to centralized system
docker run -d \
  --log-driver=fluentd \
  --log-opt fluentd-address=logs.example.com:24224 \
  zkp-coordinator:latest
```

---

## 🚨 Failure Scenarios

### Scenario 1: Leader Fails
```
Before:
  Machine 1: Leader ✓
  Machine 2: Follower ✓
  Machine 3: Follower ✓

After Machine 1 crashes:
  Machine 1: ✗ (Down)
  Machine 2: Leader ✓ (Elected in 3-5 seconds)
  Machine 3: Follower ✓

Service continues with <3 seconds downtime
```

### Scenario 2: Network Partition
```
If network splits:
  Partition A: Machine 1 (1 node - no majority)
  Partition B: Machine 2, Machine 3 (2 nodes - has majority)

Result:
  - Partition B elects new leader
  - Partition A stays follower (can't get votes)
  - When partition heals, Machine 1 rejoins as follower
```

### Scenario 3: Database Fails
```
If PostgreSQL crashes:
  - All coordinators lose DB connection
  - Raft cluster stays healthy
  - New tasks can't be created
  - Once DB recovers, service resumes
```

---

## 🔄 Rolling Updates

```bash
# Update one node at a time:

# 1. Update follower first
ssh user@192.168.1.11
docker stop zkp-coordinator-2
docker pull zkp-coordinator:v2.0
docker start zkp-coordinator-2
# Wait for it to rejoin

# 2. Update another follower
ssh user@192.168.1.12
docker stop zkp-coordinator-3
docker pull zkp-coordinator:v2.0
docker start zkp-coordinator-3

# 3. Force leader to step down
curl -X POST http://192.168.1.10:8090/admin/step-down
# Cluster will elect new leader from updated nodes

# 4. Update old leader
ssh user@192.168.1.10
docker stop zkp-coordinator-1
docker pull zkp-coordinator:v2.0
docker start zkp-coordinator-1
```

---

## 📝 Troubleshooting

### Issue: "no known peers"
**Cause**: Bootstrap didn't run or failed
**Solution**:
```bash
# Clear Raft data on coordinator-1
docker exec zkp-coordinator-1 rm -rf /var/lib/zkp/raft/*
docker restart zkp-coordinator-1
# Check logs for "Bootstrapping Raft cluster"
```

### Issue: "election timeout"
**Cause**: Nodes can't reach each other
**Solution**:
```bash
# Check network connectivity
ping 192.168.1.11
telnet 192.168.1.11 7000

# Check firewall
sudo ufw status
sudo ufw allow 7000/tcp
```

### Issue: All nodes are followers
**Cause**: Clock skew or network issues
**Solution**:
```bash
# Sync clocks
sudo ntpdate -s time.nist.gov

# Restart coordinators in order
docker restart zkp-coordinator-1
sleep 10
docker restart zkp-coordinator-2
docker restart zkp-coordinator-3
```

---

## 🎯 Key Differences: Local vs Production

| Aspect | Local (Docker Compose) | Production (Multi-Machine) |
|--------|------------------------|----------------------------|
| **Networking** | `coordinator-1` (DNS) | `192.168.1.10` (IP) |
| **Volumes** | Docker volumes | Host filesystem `/var/lib/zkp` |
| **Config** | One file, env vars | Separate files per machine |
| **Bootstrap** | Automatic | Manual, sequential |
| **TLS** | Not needed | **REQUIRED** |
| **Monitoring** | Docker logs | Prometheus + ELK |
| **High Availability** | Simulated | Real (different machines) |

---

## ✅ Production Checklist

- [ ] All coordinators have unique IDs
- [ ] Firewall ports 7000, 9090, 8090 open
- [ ] PostgreSQL is accessible from all coordinators
- [ ] TLS enabled for Raft (if public internet)
- [ ] Load balancer configured for gRPC
- [ ] Monitoring (Prometheus) set up
- [ ] Log aggregation configured
- [ ] Backup strategy for PostgreSQL
- [ ] Disaster recovery plan documented
- [ ] Health check automation (cron/systemd)

---

## 📚 Additional Resources

- **Raft Paper**: https://raft.github.io/raft.pdf
- **HashiCorp Raft Docs**: https://pkg.go.dev/github.com/hashicorp/raft
- **Production Best Practices**: See `docs/RAFT_INTEGRATION_GUIDE.md`
- **Troubleshooting**: See `BOOTSTRAP_GUIDE.md`
