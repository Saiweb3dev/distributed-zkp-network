package handlers

import (
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/consensys/gnark/frontend"
    "github.com/consensys/gnark-crypto/ecc"
    "github.com/saiweb3dev/distributed-zkp-network/internal/zkp"
    "github.com/saiweb3dev/distributed-zkp-network/internal/zkp/circuits"
    "go.uber.org/zap"
)

// ============================================================================
// HTTP Request/Response Models
// ============================================================================

type MerkleProofRequest struct {
    Leaves    []string `json:"leaves"`
    LeafIndex int      `json:"leaf_index"`
}

type ProofResponse struct {
    Success        bool                   `json:"success"`
    Proof          string                 `json:"proof,omitempty"`
    MerkleRoot     string                 `json:"merkle_root,omitempty"`
    PublicInputs   map[string]interface{} `json:"public_inputs,omitempty"`
    GenerationTime string                 `json:"generation_time,omitempty"`
    CircuitType    string                 `json:"circuit_type,omitempty"`
    Error          string                 `json:"error,omitempty"`
}

// ============================================================================
// ProofHandler
// ============================================================================

type ProofHandler struct {
    prover  zkp.Prover
    factory *circuits.CircuitFactory
    logger  *zap.Logger
    curve   ecc.ID
}

func NewProofHandler(prover zkp.Prover, logger *zap.Logger) *ProofHandler {
    return &ProofHandler{
        prover:  prover,
        factory: circuits.NewCircuitFactory(),
        logger:  logger,
        curve:   ecc.BN254,
    }
}

func (h *ProofHandler) GenerateMerkleProof(w http.ResponseWriter, r *http.Request) {
    var req MerkleProofRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, http.StatusBadRequest, "Invalid request body", err)
        return
    }
    
    if err := h.validateMerkleRequest(&req); err != nil {
        h.respondError(w, http.StatusBadRequest, "Invalid request", err)
        return
    }
    
    h.logger.Info("Received Merkle proof request",
        zap.Int("num_leaves", len(req.Leaves)),
        zap.Int("leaf_index", req.LeafIndex),
    )
    
    leaves, err := h.parseLeaves(req.Leaves)
    if err != nil {
        h.respondError(w, http.StatusBadRequest, "Invalid leaf format", err)
        return
    }
    
    tree := zkp.NewMerkleTree(leaves, h.curve)
    if tree == nil {
        h.respondError(w, http.StatusInternalServerError, "Failed to build tree", nil)
        return
    }
    
    path, indices, err := tree.GenerateProof(req.LeafIndex)
    if err != nil {
        h.respondError(w, http.StatusBadRequest, "Failed to generate Merkle proof", err)
        return
    }
    
    h.logger.Info("Generated Merkle path",
        zap.Int("path_length", len(path)),
        zap.String("root", hex.EncodeToString(tree.Root)),
    )
    
    // Create witness with actual values
    circuitInstance := h.createMerkleCircuit(tree.Depth, leaves[req.LeafIndex], path, indices, tree.Root)
    
    // Create template with initialized slices (for compilation)
    circuitTemplate := h.factory.MerkleProof(tree.Depth)
    
    // Generate ZK proof
    startTime := time.Now()
    proof, err := h.prover.GenerateProof(circuitTemplate, circuitInstance)
    duration := time.Since(startTime)
    
    if err != nil {
        h.logger.Error("Failed to generate ZK proof",
            zap.Error(err),
            zap.Duration("duration", duration),
        )
        h.respondError(w, http.StatusInternalServerError, "Proof generation failed", err)
        return
    }
    
    h.logger.Info("Successfully generated ZK proof",
        zap.Duration("duration", duration),
        zap.Int("proof_size", len(proof.ProofData)),
    )
    
    h.respondSuccess(w, proof, duration, hex.EncodeToString(tree.Root))
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *ProofHandler) validateMerkleRequest(req *MerkleProofRequest) error {
    if len(req.Leaves) == 0 {
        return fmt.Errorf("leaves array cannot be empty")
    }
    
    if len(req.Leaves) > 1024 {
        return fmt.Errorf("too many leaves (max 1024)")
    }
    
    if req.LeafIndex < 0 || req.LeafIndex >= len(req.Leaves) {
        return fmt.Errorf("leaf_index out of range")
    }
    
    return nil
}

func (h *ProofHandler) parseLeaves(hexLeaves []string) ([][]byte, error) {
    leaves := make([][]byte, len(hexLeaves))
    
    for i, hexLeaf := range hexLeaves {
        if len(hexLeaf) > 2 && hexLeaf[:2] == "0x" {
            hexLeaf = hexLeaf[2:]
        }
        
        leaf, err := hex.DecodeString(hexLeaf)
        if err != nil {
            return nil, fmt.Errorf("invalid hex at index %d: %w", i, err)
        }
        
        leaves[i] = leaf
    }
    
    return leaves, nil
}

func (h *ProofHandler) createMerkleCircuit(depth int, leaf []byte, path [][]byte, indices []int, root []byte) *circuits.MerkleProofCircuitInputs {
    leafField := zkp.BytesToFieldElement(leaf)
    rootField := zkp.BytesToFieldElement(root)
    
    pathFields := make([]frontend.Variable, len(path))
    for i, p := range path {
        pathFields[i] = zkp.BytesToFieldElement(p)
    }
    
    indicesFields := make([]frontend.Variable, len(indices))
    for i, idx := range indices {
        indicesFields[i] = idx
    }
    
    return &circuits.MerkleProofCircuitInputs{
        Leaf:        leafField,
        Path:        pathFields,
        PathIndices: indicesFields,
        Root:        rootField,
        Depth:       depth,
    }
}

func (h *ProofHandler) respondSuccess(w http.ResponseWriter, proof *zkp.Proof, duration time.Duration, root string) {
    response := ProofResponse{
        Success:        true,
        Proof:          hex.EncodeToString(proof.ProofData),
        MerkleRoot:     root,
        PublicInputs:   proof.PublicInputs,
        GenerationTime: duration.String(),
        CircuitType:    proof.CircuitType,
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) respondError(w http.ResponseWriter, statusCode int, message string, err error) {
    errorMsg := message
    if err != nil {
        errorMsg = fmt.Sprintf("%s: %v", message, err)
        h.logger.Error(message, zap.Error(err))
    }
    
    response := ProofResponse{
        Success: false,
        Error:   errorMsg,
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
    response := map[string]interface{}{
        "status": "healthy",
        "time":   time.Now().Format(time.RFC3339),
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}