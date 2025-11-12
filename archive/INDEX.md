# 🧪 Test Suite Index

Quick navigation to all testing resources.

---

## 📚 Main Documentation

| Document | Purpose | Time Required |
|----------|---------|---------------|
| **[Test README](README.md)** | Overview and summary | 5 min |
| **[Testing Quick Reference](TESTING_QUICKSTART.md)** | Fast command reference | 2 min |
| **[Manual Test Suite](manual/TEST_SUITE.md)** | Comprehensive 33-test guide | 30-45 min |

---

## 🤖 Automated Scripts

| Script | Platform | Tests | Usage |
|--------|----------|-------|-------|
| **[run-tests.sh](e2e/run-tests.sh)** | Linux/Mac | 12 tests | `./test/e2e/run-tests.sh` |
| **[run-tests.bat](e2e/run-tests.bat)** | Windows | 18 tests | `test\e2e\run-tests.bat` |

---

## 🎯 By Test Category

### Infrastructure Tests
- Container health → Manual 1.1, Auto Test 1
- Network connectivity → Manual 6.1
- Resource monitoring → Manual 9.3, Auto Test 12

### Raft Consensus Tests
- Leader election → Manual 2.1, Auto Test 3
- Node identity → Manual 2.3, Auto Test 4
- Term stability → Manual 2.2, Auto Test 6
- Log replication → Manual 2.4, Auto Test 9
- Leader failover → Manual 5.1-5.3

### Application Tests
- Worker registration → Manual 3.1, Auto Test 5
- Task submission → Manual 4.1
- Task execution → Manual 4.4
- Task status → Manual 4.2
- Concurrent tasks → Manual 4.5

### Performance Tests
- Throughput → Manual 9.1
- Response time → Manual 9.2, Auto Test 11
- Memory usage → Manual 9.3, Auto Test 12

### API Tests
- Health endpoints → Manual 1.2-1.3, Auto Test 2
- CORS → Manual 8.1
- Rate limiting → Manual 8.2
- Error handling → Manual 8.3

---

## 🚀 Quick Start Paths

### Path 1: Quick Smoke Test (2 minutes)
```bash
# Check services
docker ps --filter "name=zkp-"

# Check leader
curl http://localhost:8090/health | grep "is_leader"

# Submit test task
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type":"test","public_inputs":["1"]}'
```
**From**: [TESTING_QUICKSTART.md](TESTING_QUICKSTART.md#scenario-1-basic-smoke-test)

---

### Path 2: Automated Testing (5 minutes)
```bash
# Linux/Mac
./test/e2e/run-tests.sh

# Windows
test\e2e\run-tests.bat
```
**From**: [run-tests.sh](e2e/run-tests.sh)

---

### Path 3: Full Manual Testing (45 minutes)
1. Open [TEST_SUITE.md](manual/TEST_SUITE.md)
2. Execute tests 1.1 → 10.3 in sequence
3. Use template to track results
4. Document failures

**From**: [TEST_SUITE.md](manual/TEST_SUITE.md)

---

### Path 4: Leader Failover Test (5 minutes)
```bash
# Find leader
LEADER=$(curl -s http://localhost:8090/health | grep '"is_leader":true' && echo "coordinator-1")

# Stop leader
docker stop zkp-$LEADER

# Wait for election
sleep 5

# Check new leader
curl http://localhost:8091/health | grep "is_leader"

# Restart
docker start zkp-$LEADER
```
**From**: [TESTING_QUICKSTART.md](TESTING_QUICKSTART.md#scenario-2-failover-test)

---

## 🔍 Find by Problem

| Problem | Check This Test | Quick Fix Command |
|---------|----------------|-------------------|
| No leader elected | Manual 2.1, Auto Test 3 | Verify unique node IDs |
| Workers not registering | Manual 3.1, Auto Test 5 | Check coordinator address |
| Tasks not executing | Manual 4.3-4.4 | Verify worker logs |
| Slow response time | Manual 9.2, Auto Test 11 | Check resource usage |
| Database errors | Manual 7.1, Auto Test 7 | Verify credentials |
| API not responding | Manual 8.1, Auto Test 2 | Check port 8080 |
| High memory usage | Manual 9.3, Auto Test 12 | Restart containers |
| Constant re-elections | Manual 2.2, Auto Test 6 | Check network stability |

---

## 📊 Test Coverage Matrix

| Feature | Manual | Auto | Integration | Load | Failover |
|---------|--------|------|-------------|------|----------|
| Health checks | ✓ | ✓ | ✓ | - | - |
| Leader election | ✓ | ✓ | ✓ | - | ✓ |
| Node identity | ✓ | ✓ | ✓ | - | - |
| Log replication | ✓ | ✓ | ✓ | - | - |
| Worker registration | ✓ | ✓ | ✓ | - | ✓ |
| Task submission | ✓ | - | ✓ | ✓ | - |
| Task execution | ✓ | - | ✓ | ✓ | - |
| Database ops | ✓ | ✓ | ✓ | - | ✓ |
| API Gateway | ✓ | ✓ | ✓ | ✓ | - |
| Performance | ✓ | ✓ | - | ✓ | - |
| Failover | ✓ | - | ✓ | - | ✓ |

**Legend**: ✓ = Covered, - = Not covered

---

## ✅ Pre-Production Checklist

Use before production deployment:

### Must Pass (Critical) ⭐
- [ ] Test 2.1: Leader election
- [ ] Test 2.3: Node identity  
- [ ] Test 3.1: Worker registration
- [ ] Test 4.1: Task submission
- [ ] Test 5.1: Leader failover
- [ ] Test 7.1: Database connectivity

### Should Pass (Important)
- [ ] All automated tests (12/12)
- [ ] Response time < 200ms
- [ ] Throughput > 50 tasks/sec
- [ ] Memory usage acceptable
- [ ] No errors in logs

### Nice to Have
- [ ] Load tests completed
- [ ] Security tests passed
- [ ] Performance benchmarks met
- [ ] Chaos tests executed

**From**: [README.md](README.md#pre-production-checklist)

---

## 🔗 External Resources

### Raft Documentation
- [Raft Success Summary](../RAFT_SUCCESS.md)
- [Raft Integration Guide](../docs/RAFT_INTEGRATION_GUIDE.md)
- [Issues Solved](../docs/ISSUES_SOLVED.md)

### Deployment
- [Production Deployment](../docs/PRODUCTION_DEPLOYMENT.md)
- [Bootstrap Guide](../BOOTSTRAP_GUIDE.md)
- [Cluster Quick Start](../CLUSTER_QUICKSTART.md)

### Architecture
- [Coordinator Architecture](../docs/COORDINATOR_ARCHITECTURE_GUIDE.md)
- [Worker Architecture](../docs/WORKER_ARCHITECTURE_GUIDE.md)
- [gRPC Setup](../docs/architecture/gRPC_Setup_Guide.md)

---

## 📝 Test Execution Log Template

```
Date: ___________
Tester: ___________
Environment: [ ] Local [ ] Staging [ ] Production
Version/Commit: ___________

Quick Tests:
[ ] Smoke test passed (2 min)
[ ] Automated tests passed (5 min)
[ ] Manual subset passed (list: _________)

Issues Found:
1. _______________________
2. _______________________

Performance Metrics:
- Response time: _____ ms (target: <100ms)
- Throughput: _____ tasks/sec (target: >50)
- Memory usage: Coord=___MB Worker=___MB (target: <500MB)

Pass Rate: ____% (___/33 manual + ___/12 auto)

Recommendation: [ ] PASS [ ] CONDITIONAL PASS [ ] FAIL
Notes: _______________________________________
```

---

## 🎓 Training Path

### For New Team Members

**Day 1: Understanding**
1. Read [RAFT_SUCCESS.md](../RAFT_SUCCESS.md) - Understand what was built
2. Read [README.md](README.md) - Understand test coverage
3. Review [TESTING_QUICKSTART.md](TESTING_QUICKSTART.md) - Learn commands

**Day 2: Hands-on**
4. Run smoke test (2 min)
5. Run automated tests (5 min)
6. Execute Manual Tests 1.1-3.4 (15 min)

**Day 3: Advanced**
7. Execute Manual Tests 4.1-6.2 (20 min)
8. Try failover scenarios (Manual 5.1-5.3)
9. Review logs and troubleshooting

**Day 4: Mastery**
10. Complete full manual test suite (45 min)
11. Modify test scripts
12. Document findings

---

## 🏆 Current Test Status

**As of Raft Integration Success (Nov 4, 2025):**

✅ **Passing:**
- All 7 containers running
- Leader elected (coordinator-1)
- 2 followers active (coordinator-2, coordinator-3)
- Workers registered (1-2 active)
- Unique node IDs verified
- Database connectivity working
- Health endpoints responding
- Raft term stable

⚠️ **Known Issues:**
- None critical

🎯 **Ready For:**
- Development testing ✓
- QA validation ✓
- Production deployment ✓ (with security hardening)

---

## 📞 Support

**Having issues with tests?**

1. Check [TESTING_QUICKSTART.md](TESTING_QUICKSTART.md#troubleshooting)
2. Review [ISSUES_SOLVED.md](../docs/ISSUES_SOLVED.md)
3. Check logs: `docker logs zkp-coordinator-1`
4. Restart cluster: `docker-compose down && docker-compose up -d`

**Test failures?**
- Compare with pass criteria in [TEST_SUITE.md](manual/TEST_SUITE.md)
- Check troubleshooting section
- Verify environment matches requirements

---

**Last Updated**: November 4, 2025  
**Test Coverage**: 45+ tests (33 manual + 12 automated)  
**Status**: Production Ready ✓
