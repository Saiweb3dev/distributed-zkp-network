package raft

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"go.uber.org/zap"
)

type RaftNode struct {
	raft   *raft.Raft
	fsm    *CoordinatorFSM
	logger *zap.Logger
}

func NewRaftNode(
	nodeID string,
	raftAddr string,
	raftDir string,
	fsm *CoordinatorFSM,
	logger *zap.Logger,
) (*RaftNode, error) {
	// Create raft directory if it doesn't exist
	if err := os.MkdirAll(raftDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create raft directory: %w", err)
	}

	// Raft configuration
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)
	config.HeartbeatTimeout = 1000 * time.Millisecond
	config.ElectionTimeout = 1000 * time.Millisecond
	config.CommitTimeout = 50 * time.Millisecond
	config.SnapshotInterval = 120 * time.Second
	config.SnapshotThreshold = 8192

	logger.Info("Initializing Raft node",
		zap.String("node_id", nodeID),
		zap.String("raft_addr", raftAddr),
		zap.String("raft_dir", raftDir),
	)

	// Stable store (for logs)
	logStore, err := raftboltdb.NewBoltStore(
		filepath.Join(raftDir, "raft-log.db"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log store: %w", err)
	}

	// Stable store (for cluster config)
	stableStore, err := raftboltdb.NewBoltStore(
		filepath.Join(raftDir, "raft-stable.db"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stable store: %w", err)
	}

	// Snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(
		raftDir, 3, os.Stderr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// TCP transport
	addr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve raft address: %w", err)
	}

	transport, err := raft.NewTCPTransport(
		raftAddr, addr, 3, 10*time.Second, os.Stderr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Create Raft instance
	r, err := raft.NewRaft(
		config,
		fsm,
		logStore,
		stableStore,
		snapshotStore,
		transport,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft: %w", err)
	}

	logger.Info("Raft node created successfully")

	return &RaftNode{
		raft:   r,
		fsm:    fsm,
		logger: logger,
	}, nil
}

func (rn *RaftNode) IsLeader() bool {
	return rn.raft.State() == raft.Leader
}

func (rn *RaftNode) GetState() raft.RaftState {
	return rn.raft.State()
}

func (rn *RaftNode) GetLeaderAddress() string {
	_, leaderID := rn.raft.LeaderWithID()
	return string(leaderID)
}

func (rn *RaftNode) Apply(cmd []byte, timeout time.Duration) error {
	f := rn.raft.Apply(cmd, timeout)
	return f.Error()
}

func (rn *RaftNode) Bootstrap(peers []raft.Server) error {
	config := raft.Configuration{Servers: peers}
	f := rn.raft.BootstrapCluster(config)
	return f.Error()
}

func (rn *RaftNode) AddVoter(id raft.ServerID, address raft.ServerAddress, timeout time.Duration) error {
	f := rn.raft.AddVoter(id, address, 0, timeout)
	return f.Error()
}

func (rn *RaftNode) RemoveServer(id raft.ServerID, timeout time.Duration) error {
	f := rn.raft.RemoveServer(id, 0, timeout)
	return f.Error()
}

func (rn *RaftNode) Shutdown() error {
	rn.logger.Info("Shutting down Raft node")
	return rn.raft.Shutdown().Error()
}

// Stats returns current Raft statistics
func (rn *RaftNode) Stats() map[string]string {
	return rn.raft.Stats()
}
