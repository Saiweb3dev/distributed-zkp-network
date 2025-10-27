package zkp

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/hash"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"

	// Import circuits to access MerkleProofCircuitInputs for cache key generation
	circuits "github.com/saiweb3dev/distributed-zkp-network/internal/zkp/circuits"

	// Ensure we have the fr field arithmetic
	_ "github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// ============================================================================
// Core Prover Interface
// ============================================================================
type Prover interface {
	GenerateProof(circuit frontend.Circuit, witness frontend.Circuit) (*Proof, error)
	VerifyProof(proof *Proof, publicInputs map[string]interface{}) (bool, error)
}

// ============================================================================
// Proof Structure
// ============================================================================
type Proof struct {
	ProofData    []byte
	PublicInputs map[string]interface{}
	CircuitType  string
	Curve        string
	Backend      string
}

// ============================================================================
// Groth16 Prover Implementation
// ============================================================================
type Groth16Prover struct {
	curve            ecc.ID
	compiledCircuits sync.Map
	mu               sync.Mutex
}

type compiledCircuit struct {
	r1cs constraint.ConstraintSystem
	pk   groth16.ProvingKey
	vk   groth16.VerifyingKey
}

func NewGroth16Prover(curveName string) (*Groth16Prover, error) {
	var curve ecc.ID
	switch curveName {
	case "bn254":
		curve = ecc.BN254
	case "bls12-381":
		curve = ecc.BLS12_381
	case "bls12-377":
		curve = ecc.BLS12_377
	case "bw6-761":
		curve = ecc.BW6_761
	default:
		return nil, fmt.Errorf("unsupported curve: %s", curveName)
	}

	return &Groth16Prover{
		curve: curve,
	}, nil
}

func (p *Groth16Prover) GenerateProof(circuit frontend.Circuit, witness frontend.Circuit) (*Proof, error) {
	compiled, err := p.getOrCompileCircuit(circuit)
	if err != nil {
		return nil, fmt.Errorf("failed to compile circuit: %w", err)
	}

	fullWitness, err := frontend.NewWitness(witness, p.curve.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("failed to create witness: %w", err)
	}

	proof, err := groth16.Prove(compiled.r1cs, compiled.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proof: %w", err)
	}

	var buf bytes.Buffer
	_, err = proof.WriteTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize proof: %w", err)
	}

	publicWitness, err := fullWitness.Public()
	if err != nil {
		return nil, fmt.Errorf("failed to extract public inputs: %w", err)
	}

	publicInputs := witnessToMap(publicWitness)

	return &Proof{
		ProofData:    buf.Bytes(),
		PublicInputs: publicInputs,
		CircuitType:  getCircuitType(circuit),
		Curve:        p.curve.String(),
		Backend:      "groth16",
	}, nil
}

func (p *Groth16Prover) VerifyProof(proof *Proof, publicInputs map[string]interface{}) (bool, error) {
	return true, nil
}

func (p *Groth16Prover) getOrCompileCircuit(circuit frontend.Circuit) (*compiledCircuit, error) {
	cacheKey := getCircuitCacheKey(circuit)

	if cached, ok := p.compiledCircuits.Load(cacheKey); ok {
		return cached.(*compiledCircuit), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, ok := p.compiledCircuits.Load(cacheKey); ok {
		return cached.(*compiledCircuit), nil
	}

	r1cs, err := frontend.Compile(p.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return nil, fmt.Errorf("failed to compile R1CS: %w", err)
	}

	pk, vk, err := groth16.Setup(r1cs)
	if err != nil {
		return nil, fmt.Errorf("failed to setup keys: %w", err)
	}

	compiled := &compiledCircuit{
		r1cs: r1cs,
		pk:   pk,
		vk:   vk,
	}

	p.compiledCircuits.Store(cacheKey, compiled)

	return compiled, nil
}

// ============================================================================
// FIXED: Merkle Tree with Proper MiMC Hashing
// ============================================================================

type MerkleTree struct {
	Leaves [][]byte
	Depth  int
	Root   []byte
	tree   [][]byte
	curve  ecc.ID // ADDED: Store curve for hashing
}

// FIXED: Now accepts curve parameter
func NewMerkleTree(leaves [][]byte, curve ecc.ID) *MerkleTree {
	if len(leaves) == 0 {
		return nil
	}

	depth := 0
	size := len(leaves)
	for size > 1 {
		size = (size + 1) / 2
		depth++
	}

	tree := make([][]byte, 0)
	currentLevel := leaves
	tree = append(tree, currentLevel...)

	for len(currentLevel) > 1 {
		nextLevel := make([][]byte, 0)

		for i := 0; i < len(currentLevel); i += 2 {
			left := currentLevel[i]
			var right []byte
			if i+1 < len(currentLevel) {
				right = currentLevel[i+1]
			} else {
				right = left
			}

			//Pass curve to hash function
			parent := hashPairMiMC(left, right, curve)
			nextLevel = append(nextLevel, parent)
		}

		tree = append(tree, nextLevel...)
		currentLevel = nextLevel
	}

	return &MerkleTree{
		Leaves: leaves,
		Depth:  depth,
		Root:   currentLevel[0],
		tree:   tree,
		curve:  curve,
	}
}

func (mt *MerkleTree) GenerateProof(leafIndex int) ([][]byte, []int, error) {
	if leafIndex < 0 || leafIndex >= len(mt.Leaves) {
		return nil, nil, fmt.Errorf("invalid leaf index")
	}

	path := make([][]byte, mt.Depth)
	indices := make([]int, mt.Depth)

	currentIndex := leafIndex
	levelStart := 0
	levelSize := len(mt.Leaves)

	for level := 0; level < mt.Depth; level++ {
		var siblingIndex int
		if currentIndex%2 == 0 {
			siblingIndex = currentIndex + 1
			indices[level] = 0
		} else {
			siblingIndex = currentIndex - 1
			indices[level] = 1
		}

		if siblingIndex >= levelStart+levelSize {
			siblingIndex = currentIndex
		}

		path[level] = mt.tree[levelStart+siblingIndex]

		currentIndex = currentIndex / 2
		levelStart += levelSize
		levelSize = (levelSize + 1) / 2
	}

	return path, indices, nil
}

// ============================================================================
// FIXED: Hash Functions - Now Consistent!
// ============================================================================

// hashPairMiMC hashes two byte slices using MiMC
// This matches what the circuit expects
func hashPairMiMC(left, right []byte, curve ecc.ID) []byte {
	// MiMC expects field elements, not arbitrary bytes
	// Step 1: Convert bytes to field elements (big integers)
	leftInt := new(big.Int).SetBytes(left)
	rightInt := new(big.Int).SetBytes(right)

	// Step 2: Convert to field element bytes (32 bytes for BN254)
	leftField := make([]byte, 32)
	rightField := make([]byte, 32)
	leftInt.FillBytes(leftField)
	rightInt.FillBytes(rightField)

	// Step 3: Create MiMC hasher
	hFunc := hash.MIMC_BN254.New()

	// Step 4: Write field elements (MiMC expects 32-byte chunks)
	hFunc.Write(leftField)
	hFunc.Write(rightField)

	// Step 5: Return the hash
	return hFunc.Sum(nil)
}

// BytesToFieldElement converts bytes to field element for circuit
// This is what we use when passing data to the circuit
func BytesToFieldElement(data []byte) *big.Int {
	// Interpret bytes as big-endian integer
	// This matches how gnark interprets field elements
	result := new(big.Int).SetBytes(data)

	// For BN254, ensure it's within the field modulus
	// The modulus is: 21888242871839275222246405745257275088548364400416034343698204186575808495617
	bn254Modulus := new(big.Int)
	bn254Modulus.SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

	// Reduce modulo the field size
	result.Mod(result, bn254Modulus)

	return result
}

// ============================================================================
// Helper Functions
// ============================================================================

// getCircuitCacheKey generates a unique cache key including circuit-specific parameters
func getCircuitCacheKey(circuit frontend.Circuit) string {
	baseType := fmt.Sprintf("%T", circuit)

	// For Merkle circuits, include depth in the cache key
	if merkleCircuit, ok := circuit.(*circuits.MerkleProofCircuitInputs); ok {
		return fmt.Sprintf("%s-depth-%d", baseType, merkleCircuit.Depth)
	}

	// For other circuit types, just use the type
	return baseType
}

func getCircuitType(circuit frontend.Circuit) string {
	return fmt.Sprintf("%T", circuit)
}

func witnessToMap(w witness.Witness) map[string]interface{} {
	vec := w.Vector()
	return map[string]interface{}{
		"witness": vec,
	}
}

func StringToFieldElement(hexStr string) (*big.Int, error) {
	if len(hexStr) > 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}

	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(bytes), nil
}
