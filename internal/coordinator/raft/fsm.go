//Finite State Machine (FSM)
//Raft applies logs through FSM to maintain consistent state

package raft

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/raft"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
)

// Command types that can be replicated via Raft
type CommandType string

const (
	CmdTaskAssigned  CommandType = "task_assigned"
	CmdTaskCompleted CommandType = "task_completed"
	CmdWorkerAdded   CommandType = "worker_added"
	CmdWorkerRemoved CommandType = "worker_removed"
)

type Command struct {
	Type    CommandType            `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

type CoordinatorFSM struct {
	workerRegistry *registry.WorkerRegistry
	taskRepo       postgres.TaskRepository
	logger         *zap.Logger
}

// NewCoordinatorFSM creates a new FSM instance
func NewCoordinatorFSM(
	workerRegistry *registry.WorkerRegistry,
	taskRepo postgres.TaskRepository,
	logger *zap.Logger,
) *CoordinatorFSM {
	return &CoordinatorFSM{
		workerRegistry: workerRegistry,
		taskRepo:       taskRepo,
		logger:         logger,
	}
}

// Apply is called by Raft when a log entry is committed
// This method MUST be deterministic - given the same input, it must produce
// the same output on all nodes
func (fsm *CoordinatorFSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		fsm.logger.Error("Failed to unmarshal command", zap.Error(err))
		return err
	}

	fsm.logger.Debug("FSM applying command",
		zap.String("type", string(cmd.Type)),
		zap.Uint64("index", log.Index),
	)

	switch cmd.Type {
	case CmdTaskAssigned:
		return fsm.applyTaskAssigned(cmd.Payload)
	case CmdTaskCompleted:
		return fsm.applyTaskCompleted(cmd.Payload)
	case CmdWorkerAdded:
		return fsm.applyWorkerAdded(cmd.Payload)
	case CmdWorkerRemoved:
		return fsm.applyWorkerRemoved(cmd.Payload)
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (fsm *CoordinatorFSM) applyTaskAssigned(payload map[string]interface{}) interface{} {
	taskID := payload["task_id"].(string)
	workerID := payload["worker_id"].(string)

	// Update in-memory state
	// The database is already updated by the scheduler before replication
	_ = fsm.workerRegistry.IncrementTaskCount(workerID)

	fsm.logger.Info("FSM: Task assigned",
		zap.String("task_id", taskID),
		zap.String("worker_id", workerID),
	)

	return nil
}

func (fsm *CoordinatorFSM) applyTaskCompleted(payload map[string]interface{}) interface{} {
	taskID := payload["task_id"].(string)
	workerID := payload["worker_id"].(string)

	// Decrement worker task count
	_ = fsm.workerRegistry.DecrementTaskCount(workerID) // Best effort

	fsm.logger.Info("FSM: Task completed",
		zap.String("task_id", taskID),
		zap.String("worker_id", workerID),
	)

	return nil
}

func (fsm *CoordinatorFSM) applyWorkerAdded(payload map[string]interface{}) interface{} {
	workerID := payload["worker_id"].(string)

	fsm.logger.Info("FSM: Worker added",
		zap.String("worker_id", workerID),
	)

	return nil
}

func (fsm *CoordinatorFSM) applyWorkerRemoved(payload map[string]interface{}) interface{} {
	workerID := payload["worker_id"].(string)

	fsm.logger.Info("FSM: Worker removed",
		zap.String("worker_id", workerID),
	)

	return nil
}

// Snapshot creates a point-in-time snapshot for recovery
// This is called periodically by Raft to create recovery points
func (fsm *CoordinatorFSM) Snapshot() (raft.FSMSnapshot, error) {
	// Capture current worker registry state
	stats := fsm.workerRegistry.GetStats()

	snapshot := &CoordinatorSnapshot{
		WorkerStats: stats,
	}

	fsm.logger.Info("FSM: Created snapshot",
		zap.Int("total_workers", stats.TotalWorkers),
	)

	return snapshot, nil
}

// Restore restores FSM from snapshot
// Called when a node starts up and needs to catch up with cluster state
func (fsm *CoordinatorFSM) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()

	var snap CoordinatorSnapshot
	if err := json.NewDecoder(snapshot).Decode(&snap); err != nil {
		return err
	}

	fsm.logger.Info("FSM: Restored from snapshot",
		zap.Int("total_workers", snap.WorkerStats.TotalWorkers),
	)
	return nil
}

// CoordinatorSnapshot holds the state for snapshot persistence
type CoordinatorSnapshot struct {
	WorkerStats registry.RegistryStats `json:"worker_stats"`
}

func (s *CoordinatorSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(s)
	if err != nil {
		_ = sink.Cancel() // Best effort cancel
		return err
	}
	return sink.Close()
}

func (s *CoordinatorSnapshot) Release() {}
