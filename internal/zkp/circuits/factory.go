package circuits

import (
    "github.com/consensys/gnark/frontend"
    "github.com/consensys/gnark/std/hash/mimc"
)

// ============================================================================
// Circuit Factory
// ============================================================================

// CircuitFactory creates different types of circuits
// This provides a clean API for the handler layer
type CircuitFactory struct {
    // Can add configuration here if needed
}

// NewCircuitFactory creates a new circuit factory
func NewCircuitFactory() *CircuitFactory {
    return &CircuitFactory{}
}

// ============================================================================
// Merkle Proof Circuit
// ============================================================================

// MerkleProofCircuitInputs represents the inputs for a Merkle proof circuit
// This is what the handler will create and pass to the prover
type MerkleProofCircuitInputs struct {
    // Private inputs (witness)
    Leaf         frontend.Variable `gnark:",secret"`
    Path         []frontend.Variable `gnark:",secret"`
    PathIndices  []frontend.Variable `gnark:",secret"`
    
    // Public inputs
    Root  frontend.Variable `gnark:",public"`
    
    // Circuit parameters (not part of witness)
    Depth int `gnark:"-"`
}

// Define declares the circuit's constraints
// This is called by gnark during compilation
func (circuit *MerkleProofCircuitInputs) Define(api frontend.API) error {
    // Create hash function (MiMC is ZK-friendly)
    mimc, err := mimc.NewMiMC(api)
    if err != nil {
        return err
    }
    
    // Start with the leaf
    currentHash := circuit.Leaf
    
    // Walk up the tree, hashing with siblings
    for i := 0; i < circuit.Depth; i++ {
        // PathIndices[i] tells us if we're left (0) or right (1) child
        isRight := circuit.PathIndices[i]
        
        // Hash with sibling
        // If we're left child: hash(current, sibling)
        // If we're right child: hash(sibling, current)
        mimc.Reset()
        
        // Use Select to choose order based on isRight
        // Select(isRight, right, left) returns right if isRight==1, else left
        left := api.Select(isRight, circuit.Path[i], currentHash)
        right := api.Select(isRight, currentHash, circuit.Path[i])
        
        mimc.Write(left)
        mimc.Write(right)
        currentHash = mimc.Sum()
    }
    
    // Final hash must equal the public root
    api.AssertIsEqual(currentHash, circuit.Root)
    
    return nil
}

// MerkleProof creates a Merkle proof circuit instance
// This is a convenience method for the handler
func (f *CircuitFactory) MerkleProof(depth int) frontend.Circuit {
    // Create circuit template with the specified depth
    path := make([]frontend.Variable, depth)
    indices := make([]frontend.Variable, depth)
    
    return &MerkleProofCircuitInputs{
        Depth:       depth,
        Path:        path,
        PathIndices: indices,
    }
}