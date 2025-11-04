# Scripts Directory

## 🎯 Essential Scripts (Daily Use)

### Unix/Linux/macOS (`scripts/sh/`)

| Script             | Purpose                  | Usage                           |
| ------------------ | ------------------------ | ------------------------------- |
| `start-cluster.sh` | Start the entire cluster | `./scripts/sh/start-cluster.sh` |
| `stop-services.sh` | Stop all services        | `./scripts/sh/stop-services.sh` |
| `test-services.sh` | Run health checks        | `./scripts/sh/test-services.sh` |

### Windows (`scripts/bat/`)

| Script              | Purpose                  | Usage                           |
| ------------------- | ------------------------ | ------------------------------- |
| `start-cluster.bat` | Start the entire cluster | `scripts\bat\start-cluster.bat` |
| `stop-services.bat` | Stop all services        | `scripts\bat\stop-services.bat` |
| `test-services.bat` | Run health checks        | `scripts\bat\test-services.bat` |

---

## 🚀 Quick Start

### Start Everything

```bash
# Unix
./scripts/sh/start-cluster.sh

# Windows
scripts\bat\start-cluster.bat
```

### Stop Everything

```bash
# Unix
./scripts/sh/stop-services.sh

# Windows
scripts\bat\stop-services.bat
```

### Test Services

```bash
# Unix
./scripts/sh/test-services.sh

# Windows
scripts\bat\test-services.bat
```

---

## 📦 Archived Scripts

Rarely-used scripts have been moved to `archive/scripts/`:

### Unix Scripts (archive/scripts/sh/)

- `bootstrap-raft.sh` - Manual Raft cluster bootstrap (rarely needed)
- `install-raft-deps.sh` - One-time dependency installation
- `start-services.sh` - Alternative startup (use start-cluster.sh instead)
- `setup.sh` - Initial project setup (one-time use)
- `test.sh` - Old test script (use test/e2e/run-tests.sh instead)
- `refactor-project.sh` - Project refactoring tool (one-time use)
- `generate-protos.sh` - Regenerate protocol buffers (only when .proto files change)

### Windows Scripts (archive/scripts/bat/)

- `bootstrap-raft.bat` - Manual Raft cluster bootstrap (rarely needed)
- `install-raft-deps.bat` - One-time dependency installation
- `start-services.bat` - Alternative startup (use start-cluster.bat instead)

**When to use archived scripts:**

- Initial setup: `archive/scripts/sh/setup.sh`
- Proto file changes: `archive/scripts/sh/generate-protos.sh`
- Manual Raft issues: `archive/scripts/sh/bootstrap-raft.sh`

---

## 🔄 Common Workflows

### 1. Normal Development

```bash
# Start
./scripts/sh/start-cluster.sh

# Develop...

# Stop
./scripts/sh/stop-services.sh
```

### 2. Quick Health Check

```bash
./scripts/sh/test-services.sh
```

### 3. Complete Test Suite

```bash
# Unix
./test/e2e/run-tests.sh

# Windows
test\e2e\run-tests.bat
```

### 4. Reset Everything

```bash
# Stop services
./scripts/sh/stop-services.sh

# Clean Docker volumes
docker volume prune -f

# Restart
./scripts/sh/start-cluster.sh
```

---

## 📊 Simplification Results

| Category        | Before | After | Reduction |
| --------------- | ------ | ----- | --------- |
| Unix scripts    | 10     | 3     | -70%      |
| Windows scripts | 6      | 3     | -50%      |
| **Total**       | **16** | **6** | **-62%**  |

---

## 🎯 Design Principles

1. **Simplicity First**: Only scripts you use daily are in the main directory
2. **Archive, Don't Delete**: Rarely-used scripts preserved in archive/
3. **Clear Naming**: Each script's purpose is obvious from its name
4. **Platform Specific**: sh/ for Unix, bat/ for Windows
5. **One-Action Scripts**: Each script does one clear thing

---

## 🛠️ Makefile Alternative

You can also use the Makefile for common tasks:

```bash
make cluster        # Start cluster
make stop          # Stop services
make test          # Run tests
make clean         # Clean up
```

---

## 📝 Notes

- **Docker Compose**: All scripts use `deployments/docker/docker-compose-cluster.yml`
- **Ports Used**:

  - API Gateway: 8080
  - Coordinators: 8090-8092 (HTTP), 9090-9092 (gRPC)
  - Raft: 7000-7002
  - Database: 5432

- **Services Started**:
  - PostgreSQL database
  - 3 Coordinators (Raft cluster)
  - 2 Workers
  - 1 API Gateway

---

## 🆘 Need Help?

- **Cluster won't start**: Check `docker ps` and logs with `docker logs zkp-coordinator-1`
- **Tests failing**: See [test/README.md](../../test/README.md)
- **Port conflicts**: Stop other services or change ports in configs/
- **Archived script needed**: Scripts are in `archive/scripts/sh/` or `archive/scripts/bat/`

---

## 📖 Related Documentation

- [Main README](../../README.md)
- [Test Guide](../../test/README.md)
- [Documentation Index](../../docs/README.md)
- [Raft Guide](../../docs/RAFT_GUIDE.md)
