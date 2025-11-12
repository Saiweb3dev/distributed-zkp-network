-- internal/storage/postgres/migrations/001_initial_schema.up.sql
-- Phase 2: Distributed ZKP Network Database Schema

-- ============================================================================
-- Tasks Table: Core persistent task queue
-- ============================================================================

CREATE TYPE task_status AS ENUM (
    'pending',      -- Task created, awaiting assignment
    'assigned',     -- Assigned to worker, not yet started
    'in_progress',  -- Worker actively generating proof
    'completed',    -- Proof successfully generated
    'failed'        -- Proof generation failed
);

CREATE TYPE circuit_type AS ENUM (
    'merkle_proof',
    'range_proof',
    'addition'
);

CREATE TABLE tasks (
    -- Identification
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Task specification
    circuit_type circuit_type NOT NULL,
    input_data JSONB NOT NULL,  -- Circuit-specific inputs
    
    -- Status tracking
    status task_status NOT NULL DEFAULT 'pending',
    error_message TEXT,  -- Populated if status = 'failed'
    
    -- Assignment tracking
    worker_id VARCHAR(255),  -- Which worker is processing this
    assigned_at TIMESTAMP,
    
    -- Timing information
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,  -- When worker began processing
    completed_at TIMESTAMP,  -- When proof generation finished
    
    -- Result storage
    proof_data BYTEA,  -- Serialized proof (populated when completed)
    proof_metadata JSONB,  -- Additional proof information
    merkle_root VARCHAR(255),  -- For Merkle proofs specifically
    
    -- Retry and monitoring
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    
    -- Indexing for efficient queries
    CONSTRAINT valid_status_transitions CHECK (
        (status = 'pending' AND worker_id IS NULL) OR
        (status IN ('assigned', 'in_progress') AND worker_id IS NOT NULL) OR
        (status IN ('completed', 'failed'))
    )
);

-- Indexes for query performance
CREATE INDEX idx_tasks_status ON tasks(status) WHERE status IN ('pending', 'assigned');
CREATE INDEX idx_tasks_worker_id ON tasks(worker_id) WHERE worker_id IS NOT NULL;
CREATE INDEX idx_tasks_created_at ON tasks(created_at DESC);
CREATE INDEX idx_tasks_stale ON tasks(status, started_at) 
    WHERE status = 'in_progress';  -- Find stale tasks for reassignment

-- ============================================================================
-- Workers Table: Registry of all workers
-- ============================================================================

CREATE TYPE worker_status AS ENUM (
    'active',      -- Healthy and accepting work
    'inactive',    -- Registered but not currently available
    'dead'         -- Failed health checks, awaiting cleanup
);

CREATE TABLE workers (
    -- Identification
    id VARCHAR(255) PRIMARY KEY,  -- Worker generates unique ID on startup
    
    -- Connection information
    address VARCHAR(255) NOT NULL,  -- Host:port for gRPC connection
    
    -- Capacity tracking
    max_concurrent_tasks INTEGER NOT NULL DEFAULT 4,
    current_task_count INTEGER NOT NULL DEFAULT 0,
    
    -- Health monitoring
    status worker_status NOT NULL DEFAULT 'active',
    last_heartbeat_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Metadata
    capabilities JSONB,  -- Which circuit types this worker supports
    registered_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Resource information (for future load balancing)
    cpu_cores INTEGER,
    memory_mb INTEGER,
    
    CONSTRAINT positive_capacity CHECK (max_concurrent_tasks > 0),
    CONSTRAINT valid_current_count CHECK (current_task_count >= 0)
);

CREATE INDEX idx_workers_status ON workers(status);
CREATE INDEX idx_workers_last_heartbeat ON workers(last_heartbeat_at) 
    WHERE status = 'active';

-- ============================================================================
-- Coordinator State Table: Raft cluster membership
-- ============================================================================

CREATE TABLE coordinator_state (
    -- Raft node identification
    node_id VARCHAR(255) PRIMARY KEY,
    
    -- Network address
    raft_address VARCHAR(255) NOT NULL,
    api_address VARCHAR(255) NOT NULL,
    
    -- Cluster role
    is_leader BOOLEAN NOT NULL DEFAULT FALSE,
    leader_id VARCHAR(255),  -- Current leader (all nodes track this)
    
    -- Health
    last_seen_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Metadata
    joined_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_coordinator_leader ON coordinator_state(is_leader) WHERE is_leader = TRUE;

-- ============================================================================
-- Task Assignments Table: Historical audit trail
-- ============================================================================

CREATE TABLE task_assignments (
    id BIGSERIAL PRIMARY KEY,
    
    -- References
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    worker_id VARCHAR(255) NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    
    -- Assignment details
    assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    -- Outcome
    success BOOLEAN,
    error_message TEXT,
    
    -- Performance metrics
    duration_seconds DECIMAL(10, 3),  -- How long proof generation took
    
    CONSTRAINT valid_completion CHECK (
        (completed_at IS NULL AND success IS NULL) OR
        (completed_at IS NOT NULL AND success IS NOT NULL)
    )
);

CREATE INDEX idx_task_assignments_task_id ON task_assignments(task_id);
CREATE INDEX idx_task_assignments_worker_id ON task_assignments(worker_id);
CREATE INDEX idx_task_assignments_assigned_at ON task_assignments(assigned_at DESC);

-- ============================================================================
-- Views for Common Queries
-- ============================================================================

-- Available workers with capacity
CREATE VIEW available_workers AS
SELECT 
    w.*,
    (w.max_concurrent_tasks - w.current_task_count) AS available_slots
FROM workers w
WHERE 
    w.status = 'active'
    AND w.current_task_count < w.max_concurrent_tasks
    AND w.last_heartbeat_at > (NOW() - INTERVAL '30 seconds')
ORDER BY available_slots DESC;

-- Stale tasks that need reassignment
CREATE VIEW stale_tasks AS
SELECT 
    t.*,
    EXTRACT(EPOCH FROM (NOW() - t.started_at)) AS stale_seconds
FROM tasks t
WHERE 
    t.status = 'in_progress'
    AND t.started_at < (NOW() - INTERVAL '5 minutes')  -- Configurable timeout
ORDER BY t.started_at ASC;

-- Task statistics by status
CREATE VIEW task_stats AS
SELECT 
    status,
    COUNT(*) AS count,
    AVG(EXTRACT(EPOCH FROM (completed_at - created_at))) AS avg_duration_seconds
FROM tasks
WHERE created_at > (NOW() - INTERVAL '24 hours')
GROUP BY status;

-- Worker performance metrics
CREATE VIEW worker_performance AS
SELECT 
    w.id AS worker_id,
    w.status,
    COUNT(ta.id) AS total_assignments,
    SUM(CASE WHEN ta.success THEN 1 ELSE 0 END) AS successful_tasks,
    AVG(ta.duration_seconds) AS avg_duration_seconds,
    MAX(ta.assigned_at) AS last_assignment_at
FROM workers w
LEFT JOIN task_assignments ta ON w.id = ta.worker_id
WHERE ta.assigned_at > (NOW() - INTERVAL '24 hours')
GROUP BY w.id, w.status;

-- ============================================================================
-- Functions for Task Management
-- ============================================================================

-- Function to assign a task to a worker
CREATE OR REPLACE FUNCTION assign_task_to_worker(
    p_task_id UUID,
    p_worker_id VARCHAR(255)
) RETURNS BOOLEAN AS $$
DECLARE
    v_success BOOLEAN;
BEGIN
    -- Update task status
    UPDATE tasks 
    SET 
        status = 'assigned',
        worker_id = p_worker_id,
        assigned_at = NOW()
    WHERE 
        id = p_task_id 
        AND status = 'pending'
    RETURNING TRUE INTO v_success;
    
    -- Increment worker's task count
    IF v_success THEN
        UPDATE workers 
        SET current_task_count = current_task_count + 1
        WHERE id = p_worker_id;
        
        -- Create assignment record
        INSERT INTO task_assignments (task_id, worker_id)
        VALUES (p_task_id, p_worker_id);
    END IF;
    
    RETURN COALESCE(v_success, FALSE);
END;
$$ LANGUAGE plpgsql;

END;
$$ LANGUAGE plpgsql;

-- Function to start a task (transition from assigned to in_progress)
CREATE OR REPLACE FUNCTION start_task(
    p_task_id UUID,
    p_worker_id VARCHAR(255)
) RETURNS BOOLEAN AS $$
DECLARE
    v_success BOOLEAN;
BEGIN
    -- Update task to in_progress
    UPDATE tasks 
    SET 
        status = 'in_progress',
        started_at = NOW()
    WHERE 
        id = p_task_id 
        AND status = 'assigned'
        AND worker_id = p_worker_id  -- Ensure task is assigned to this worker
    RETURNING TRUE INTO v_success;
    
    RETURN COALESCE(v_success, FALSE);
END;
$$ LANGUAGE plpgsql;

-- Function to complete a task
CREATE OR REPLACE FUNCTION complete_task(
    p_task_id UUID,
    p_proof_data BYTEA,
    p_proof_metadata JSONB,
    p_merkle_root VARCHAR(255) DEFAULT NULL
) RETURNS BOOLEAN AS $$
DECLARE
    v_worker_id VARCHAR(255);
    v_started_at TIMESTAMP;
    v_duration DECIMAL(10, 3);
BEGIN
    -- Update task
    UPDATE tasks 
    SET 
        status = 'completed',
        proof_data = p_proof_data,
        proof_metadata = p_proof_metadata,
        merkle_root = p_merkle_root,
        completed_at = NOW()
    WHERE 
        id = p_task_id 
        AND status = 'in_progress'
    RETURNING worker_id, started_at INTO v_worker_id, v_started_at;
    
    IF v_worker_id IS NULL THEN
        RETURN FALSE;
    END IF;
    
    -- Calculate duration
    v_duration := EXTRACT(EPOCH FROM (NOW() - v_started_at));
    
    -- Decrement worker's task count
    UPDATE workers 
    SET current_task_count = current_task_count - 1
    WHERE id = v_worker_id;
    
    -- Update assignment record
    UPDATE task_assignments 
    SET 
        completed_at = NOW(),
        success = TRUE,
        duration_seconds = v_duration
    WHERE 
        task_id = p_task_id 
        AND worker_id = v_worker_id
        AND completed_at IS NULL;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- Function to fail a task
CREATE OR REPLACE FUNCTION fail_task(
    p_task_id UUID,
    p_error_message TEXT
) RETURNS BOOLEAN AS $$
DECLARE
    v_worker_id VARCHAR(255);
    v_retry_count INTEGER;
    v_max_retries INTEGER;
BEGIN
    -- Get current retry info
    SELECT worker_id, retry_count, max_retries 
    INTO v_worker_id, v_retry_count, v_max_retries
    FROM tasks 
    WHERE id = p_task_id;
    
    IF v_worker_id IS NULL THEN
        RETURN FALSE;
    END IF;
    
    -- Decide whether to retry or fail permanently
    IF v_retry_count < v_max_retries THEN
        -- Reset to pending for retry
        UPDATE tasks 
        SET 
            status = 'pending',
            worker_id = NULL,
            assigned_at = NULL,
            started_at = NULL,
            retry_count = retry_count + 1,
            error_message = p_error_message
        WHERE id = p_task_id;
    ELSE
        -- Permanent failure
        UPDATE tasks 
        SET 
            status = 'failed',
            error_message = p_error_message,
            completed_at = NOW()
        WHERE id = p_task_id;
    END IF;
    
    -- Decrement worker's task count
    UPDATE workers 
    SET current_task_count = current_task_count - 1
    WHERE id = v_worker_id;
    
    -- Update assignment record
    UPDATE task_assignments 
    SET 
        completed_at = NOW(),
        success = FALSE,
        error_message = p_error_message
    WHERE 
        task_id = p_task_id 
        AND worker_id = v_worker_id
        AND completed_at IS NULL;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- Cleanup Functions (for maintenance)
-- ============================================================================

-- Mark dead workers
CREATE OR REPLACE FUNCTION mark_dead_workers() RETURNS INTEGER AS $$
DECLARE
    v_count INTEGER;
BEGIN
    UPDATE workers 
    SET status = 'dead'
    WHERE 
        status = 'active'
        AND last_heartbeat_at < (NOW() - INTERVAL '1 minute')
    RETURNING COUNT(*) INTO v_count;
    
    RETURN COALESCE(v_count, 0);
END;
$$ LANGUAGE plpgsql;

-- Cleanup old completed tasks (retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_tasks(
    p_retention_days INTEGER DEFAULT 30
) RETURNS INTEGER AS $$
DECLARE
    v_count INTEGER;
BEGIN
    DELETE FROM tasks 
    WHERE 
        status IN ('completed', 'failed')
        AND completed_at < (NOW() - MAKE_INTERVAL(days => p_retention_days))
    RETURNING COUNT(*) INTO v_count;
    
    RETURN COALESCE(v_count, 0);
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- Initial Data
-- ============================================================================

-- Insert coordinator nodes (example, adjust for your deployment)
INSERT INTO coordinator_state (node_id, raft_address, api_address) VALUES
    ('coord-1', 'localhost:7000', 'localhost:9090'),
    ('coord-2', 'localhost:7001', 'localhost:9091'),
    ('coord-3', 'localhost:7002', 'localhost:9092');

-- ============================================================================
-- Comments explaining design decisions
-- ============================================================================

COMMENT ON TABLE tasks IS 'Persistent task queue with full lifecycle tracking';
COMMENT ON COLUMN tasks.input_data IS 'JSON blob containing circuit-specific inputs (leaves, indices, etc.)';
COMMENT ON COLUMN tasks.proof_data IS 'Binary proof bytes, serialized by gnark';
COMMENT ON CONSTRAINT valid_status_transitions ON tasks IS 'Enforces state machine: pending->assigned->in_progress->completed/failed';

COMMENT ON TABLE workers IS 'Registry of all worker nodes with capacity tracking';
COMMENT ON COLUMN workers.last_heartbeat_at IS 'Updated every 5s by worker, coordinator marks dead if stale';

COMMENT ON TABLE coordinator_state IS 'Tracks Raft cluster membership and leadership';

COMMENT ON FUNCTION assign_task_to_worker IS 'Atomic operation to assign pending task to available worker';
COMMENT ON FUNCTION complete_task IS 'Atomic operation to mark task completed and update metrics';
COMMENT ON FUNCTION fail_task IS 'Handles task failure with automatic retry logic up to max_retries';