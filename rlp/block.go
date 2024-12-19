package rlp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"io"
	"math/big"
	"sync"

	a "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	b "github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/go-ecvrf"
	"golang.org/x/crypto/blake2b"
)

type Header struct {
	ParentID    common.Hash
	Timestamp   uint64
	GasLimit    uint64
	Beneficiary common.Address

	GasUsed    uint64
	TotalScore uint64

	TxsRootFeatures txsRootFeatures
	StateRoot       common.Hash
	ReceiptsRoot    common.Hash

	Signature []byte

	Extension extension
}

// Number returns sequential number of this block.
func (h *Header) Number() uint32 {
	// inferred from parent id
	return binary.BigEndian.Uint32(h.ParentID[:]) + 1
}

// TxsRoot returns merkle root of txs contained in this block.
func (h *Header) TxsRoot() common.Hash {
	return h.TxsRootFeatures.Root
}

// TxsFeatures returns supported txs features.
func (h *Header) TxsFeatures() uint32 {
	return h.TxsRootFeatures.Features
}

// ID computes id of block.
// The block ID is defined as: blockNumber + hash(signingHash, signer)[4:].
func (h *Header) ID() (id common.Hash) {

	defer func() {
		// overwrite first 4 bytes of block hash to block number.
		binary.BigEndian.PutUint32(id[:], h.Number())
	}()
	signer, err := h.Signer()
	if err != nil {
		return common.Hash{}
	}

	return Blake2b(h.SigningHash().Bytes(), signer[:])
}
func Blake2b(data ...[]byte) common.Hash {
	if len(data) == 1 {
		// the quick version
		return blake2b.Sum256(data[0])
	} else {
		return Blake2bFn(func(w io.Writer) {
			for _, b := range data {
				w.Write(b)
			}
		})
	}
}

type blake2bState struct {
	hash.Hash
	b32 common.Hash
}

func NewBlake2b() hash.Hash {
	hash, _ := blake2b.New256(nil)
	return hash
}

var blake2bStatePool = sync.Pool{
	New: func() interface{} {
		return &blake2bState{
			Hash: NewBlake2b(),
		}
	},
}

// Blake2bFn computes blake2b-256 checksum for the provided writer.
func Blake2bFn(fn func(w io.Writer)) (h common.Hash) {
	w := blake2bStatePool.Get().(*blake2bState)
	fn(w)
	w.Sum(w.b32[:0])
	h = w.b32 // to avoid 1 alloc
	w.Reset()
	blake2bStatePool.Put(w)
	return
}

// SigningHash computes hash of all header fields excluding signature.
func (h *Header) SigningHash() (hash common.Hash) {

	return Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			&h.ParentID,
			h.Timestamp,
			h.GasLimit,
			&h.Beneficiary,

			h.GasUsed,
			h.TotalScore,

			&h.TxsRootFeatures,
			&h.StateRoot,
			&h.ReceiptsRoot,
		})
	})
}

// withSignature create a new Header object with signature set.
//func (h *Header) withSignature(sig []byte) *Header {
//	cpy := Header{body: h.body}
//	cpy.body.Signature = append([]byte(nil), sig...)
//	return &cpy
//}

// pubkey recover leader's public key.
func (h *Header) pubkey() (pubkey *ecdsa.PublicKey, err error) {

	if len(h.Signature) < 65 {
		return nil, errors.New("invalid signature length")
	}

	return crypto.SigToPub(h.SigningHash().Bytes(), ComplexSignature(h.Signature).Signature())
}

// Signer extract signer of the block from signature.
func (h *Header) Signer() (common.Address, error) {
	if h.Number() == 0 {
		// special case for genesis block
		return common.Address{}, nil
	}

	pub, err := h.pubkey()
	if err != nil {
		return common.Address{}, err
	}

	return common.Address(crypto.PubkeyToAddress(*pub)), nil
}

// Alpha returns the alpha in the header.
func (h *Header) Alpha() []byte {
	return h.Extension.Alpha
}

// COM returns whether the packer votes COM.
func (h *Header) COM() bool {
	return h.Extension.COM
}

// Beta verifies the VRF proof in header's signature and returns the beta.
func (h *Header) Beta() (beta []byte, err error) {
	if h.Number() == 0 || len(h.Signature) == 65 {
		return
	}

	if len(h.Signature) != 81+65 {
		return nil, errors.New("invalid signature length")
	}
	pub, err := h.pubkey()
	if err != nil {
		return
	}

	return vrf.Verify(pub, h.Extension.Alpha, ComplexSignature(h.Signature).Proof())
}

type _txsRootFeatures txsRootFeatures
type txsRootFeatures struct {
	Root     common.Hash
	Features uint32 // supported features
}

func (trf *txsRootFeatures) EncodeRLP(w io.Writer) error {
	if trf.Features == 0 {
		// backward compatible
		return rlp.Encode(w, &trf.Root)
	}

	return rlp.Encode(w, (*_txsRootFeatures)(trf))
}
func (trf *txsRootFeatures) DecodeRLP(s *rlp.Stream) error {
	kind, _, _ := s.Kind()
	if kind == rlp.List {
		var obj _txsRootFeatures
		if err := s.Decode(&obj); err != nil {
			return err
		}
		*trf = txsRootFeatures(obj)
	} else {
		var root common.Hash
		if err := s.Decode(&root); err != nil {
			return err
		}
		*trf = txsRootFeatures{
			root,
			0,
		}
	}
	return nil
}

type _extension extension
type extension struct {
	Alpha []byte
	COM   bool
}

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	if ex.COM {
		return rlp.Encode(w, (*_extension)(ex))
	}

	if len(ex.Alpha) != 0 {
		return rlp.Encode(w, []interface{}{
			ex.Alpha,
		})
	}
	return nil
}

// DecodeRLP implements rlp.Decoder.
func (ex *extension) DecodeRLP(s *rlp.Stream) error {
	var raws []rlp.RawValue

	if err := s.Decode(&raws); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*ex = extension{
				nil,
				false,
			}
			return nil
		}
	}

	if len(raws) == 0 || len(raws) > 2 {
		return errors.New("rlp: unexpected extension")
	} else {
		var alpha []byte
		if err := rlp.DecodeBytes(raws[0], &alpha); err != nil {
			return err
		}

		// only alpha, make sure it's trimmed
		if len(raws) == 1 {
			if len(alpha) == 0 {
				return errors.New("rlp: extension must be trimmed")
			}

			*ex = extension{
				Alpha: alpha,
				COM:   false,
			}
			return nil
		}

		var com bool
		if err := rlp.DecodeBytes(raws[1], &com); err != nil {
			return err
		}

		// COM must be trimmed if not set
		if !com {
			return errors.New("rlp: extension must be trimmed")
		}

		*ex = extension{
			Alpha: alpha,
			COM:   com,
		}
		return nil
	}
}

type ComplexSignature []byte

// NewComplexSignature creates a new signature.
func NewComplexSignature(signature, proof []byte) (ComplexSignature, error) {
	if len(signature) != 65 {
		return nil, errors.New("invalid signature length, 65 bytes required")
	}
	if len(proof) != 81 {
		return nil, errors.New("invalid proof length, 81 bytes required")
	}

	var ms ComplexSignature
	ms = make([]byte, 0, 81+65)
	ms = append(ms, signature...)
	ms = append(ms, proof...)

	return ms, nil
}

// Signature returns the ECDSA signature.
func (ms ComplexSignature) Signature() []byte {
	return ms[:65]
}

// Proof returns the VRF proof.
func (ms ComplexSignature) Proof() []byte {
	return ms[65:]
}

var vrf = ecvrf.New(&ecvrf.Config{
	Curve:       &mergedCurve{},
	SuiteString: 0xfe,
	Cofactor:    0x01,
	NewHasher:   sha256.New,
	Decompress: func(_ elliptic.Curve, pk []byte) (x, y *big.Int) {
		return b.DecompressPubkey(pk)
	},
})

// Prove constructs a VRF proof `pi` for the given input `alpha`,
// using the private key `sk`. The hash output is returned as `beta`.
func Prove(sk *ecdsa.PrivateKey, alpha []byte) (beta, pi []byte, err error) {
	return vrf.Prove(sk, alpha)
}

// Verify checks the proof `pi` of the message `alpha` against the given
// public key `pk`. The hash output is returned as `beta`.
func Verify(pk *ecdsa.PublicKey, alpha, pi []byte) (beta []byte, err error) {
	return vrf.Verify(pk, alpha, pi)
}

// mergedCurve merges fast parts of two secp256k1 curve implementations.
type mergedCurve struct{}

// Params returns the parameters for the curve.
func (c *mergedCurve) Params() *elliptic.CurveParams {
	return a.S256().Params()
}

// IsOnCurve reports whether the given (x,y) lies on the curve.
func (c *mergedCurve) IsOnCurve(x, y *big.Int) bool {
	return a.S256().IsOnCurve(x, y)
}

// Add returns the sum of (x1,y1) and (x2,y2)
func (c *mergedCurve) Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int) {
	return a.S256().Add(x1, y1, x2, y2)
}

// Double returns 2*(x,y)
func (c *mergedCurve) Double(x1, y1 *big.Int) (x, y *big.Int) {
	return a.S256().Double(x1, y1)
}

// ScalarMult returns k*(Bx,By) where k is a number in big-endian form.
func (c *mergedCurve) ScalarMult(x1, y1 *big.Int, k []byte) (x, y *big.Int) {
	return b.S256().ScalarMult(x1, y1, k)
}

// ScalarBaseMult returns k*G, where G is the base point of the group
// and k is an integer in big-endian form.
func (c *mergedCurve) ScalarBaseMult(k []byte) (x, y *big.Int) {
	return b.S256().ScalarBaseMult(k)
}

type JSONRawBlockSummary struct {
	Raw string `json:"raw"`
}
