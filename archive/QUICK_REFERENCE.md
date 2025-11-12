# 🚀 Quick Reference Card

## Essential Commands (Use These Every Day)

### Start/Stop Cluster
```bash
make cluster          # Start everything (3 coordinators, 2 workers, API gateway)
make stop            # Stop all services
make restart         # Restart cluster
```

### Health & Monitoring
```bash
make health          # Quick health check
make ps              # Show running containers
make logs            # View all logs
make logs-coordinator # Coordinator logs only
make logs-worker     # Worker logs only
make logs-gateway    # Gateway logs only
```

### Testing
```bash
./test/e2e/run-tests.sh    # Unix E2E tests
test\e2e\run-tests.bat     # Windows E2E tests
make test                  # Unit + integration tests
```

### Maintenance
```bash
make clean-volumes   # Remove Raft data volumes
make restart         # Clean restart
```

---

## Script Locations

### Essential (Daily Use)
```
scripts/sh/              scripts/bat/
├── start-cluster.sh    ├── start-cluster.bat
├── stop-services.sh    ├── stop-services.bat
└── test-services.sh    └── test-services.bat
```

### Archived (Rarely Needed)
```
archive/scripts/sh/      archive/scripts/bat/
├── bootstrap-raft.sh   ├── bootstrap-raft.bat
├── setup.sh            ├── install-raft-deps.bat
├── test.sh             └── start-services.bat
├── generate-protos.sh
└── ... (3 more)
```

---

## Service URLs

```
API Gateway:    http://localhost:8080
  /health       Health check
  /ready        Readiness check

Coordinators:
  Coordinator-1: http://localhost:8090/health
  Coordinator-2: http://localhost:8091/health
  Coordinator-3: http://localhost:8092/health

Database:      localhost:5432
```

---

## Quick Troubleshooting

### Cluster won't start
```bash
make stop
docker ps -a
make cluster
```

### Check logs
```bash
make logs-coordinator
```

### Clean restart
```bash
make stop
make clean-volumes
make cluster
```

### Tests failing
```bash
make health
docker logs zkp-coordinator-1
```

---

## File Counts

| Type | Count | Location |
|------|-------|----------|
| Essential scripts | 6 | scripts/ |
| Archived scripts | 10 | archive/scripts/ |
| Documentation | 5 | docs/, scripts/README.md |
| Test suites | 2 | test/e2e/ |

---

## 3 Commands = 99% of Use Cases

```bash
1. make cluster    # Start
2. make health     # Check
3. make stop       # Stop
```

**That's all you need to remember!** ✨
