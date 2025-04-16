package authority

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	accounts2 "github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	block2 "github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

type List struct {
	candidates []Candidate
	thor       *thorclient.Client
}

func NewList(thor *thorclient.Client) *List {
	return &List{
		thor:       thor,
		candidates: make([]Candidate, 0),
	}
}

func (l *List) ShouldReset(block *blocks.JSONExpandedBlock) bool {
	if len(l.candidates) == 0 {
		return true
	}
	candidateMap := make(map[thor.Address]bool)
	for _, candidate := range l.candidates {
		candidateMap[candidate.Master] = true
	}

	hasAuthorityEvent := func() bool {
		for _, r := range block.Transactions {
			for _, o := range r.Outputs {
				for _, ev := range o.Events {
					if ev.Address == builtin.Authority.Address {
						return true
					}
				}
			}
		}
		return false
	}()

	// if no event emitted from Authority contract, it's believed that the candidates list not changed
	if !hasAuthorityEvent {
		// if no endorsor related transfer, or no event emitted from Params contract, the proposers list
		// can be reused
		hasEndorsorEvent := func() bool {
			for _, r := range block.Transactions {
				for _, o := range r.Outputs {
					for _, ev := range o.Events {
						if ev.Address == builtin.Params.Address {
							return true
						}
					}
					for _, t := range o.Transfers {
						if _, ok := candidateMap[t.Sender]; ok {
							return true
						}
						if _, ok := candidateMap[t.Recipient]; ok {
							return true
						}
					}
				}
			}
			return false
		}()

		return hasEndorsorEvent
	}

	return false
}

func (l *List) Len() int {
	return len(l.candidates)
}

func (l *List) Invalidate() {
	l.candidates = make([]Candidate, 0)
}

func (l *List) Init(revision thor.Bytes32) error {
	candidates, err := listAllCandidates(l.thor, revision)
	if err != nil {
		return err
	}
	l.candidates = candidates
	return nil
}

func (l *List) Shuffled(prev *blocks.JSONExpandedBlock) ([]thor.Address, error) {
	seed, err := l.generateSeed(prev.ID)
	if err != nil {
		return nil, err
	}
	block, err := l.thor.Block(strconv.FormatUint((uint64)(prev.Number+1), 10))
	if err != nil {
		return nil, err
	}
	nextBlockCandidates, err := listAllCandidates(l.thor, block.ID)
	if err != nil {
		return nil, err
	}
	return shuffleCandidates(l.candidates, nextBlockCandidates, seed, prev.Number), nil
}

func (l *List) generateSeed(parentID thor.Bytes32) (seed []byte, err error) {
	blockNum := binary.BigEndian.Uint32(parentID[:]) + 1
	epoch := blockNum / 8640
	seedNum := (epoch - 1) * 8640

	seedBlock, err := l.thor.Block(fmt.Sprintf("%d", seedNum))
	if err != nil {
		return
	}
	seedID := seedBlock.ID

	rawBlock := blocks.JSONRawBlockSummary{}
	res, status, err := l.thor.RawHTTPClient().RawHTTPGet("/blocks/" + hex.EncodeToString(seedID.Bytes()) + "?raw=true")
	if status != 200 {
		return
	}
	if err = json.Unmarshal(res, &rawBlock); err != nil {
		return
	}
	data, err := hex.DecodeString(rawBlock.Raw[2:])
	if err != nil {
		return
	}
	header := block2.Header{}
	err = rlp.DecodeBytes(data, &header)
	if err != nil {
		return
	}

	return header.Beta()
}

func listAllCandidates(thorClient *thorclient.Client, blockID thor.Bytes32) ([]Candidate, error) {
	gas := uint64(3000000)
	caller := thor.MustParseAddress("0x6d95e6dca01d109882fe1726a2fb9865fa41e7aa")
	gasPayer := thor.MustParseAddress("0xd3ae78222beadb038203be21ed5ce7c9b1bff602")
	authorityContract := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")

	contract, _ := hex.DecodeString(AuthorityListAll)
	clauses := [2]accounts2.Clause{
		{
			To:    nil,
			Value: nil,
			Data:  hexutil.Encode(contract),
		},
		{
			To:   &authorityContract,
			Data: "0x6f0470aa",
		},
	}

	body := &accounts2.BatchCallData{
		Gas:      gas,
		Caller:   &caller,
		GasPayer: &gasPayer,
		Clauses:  clauses[:],
	}

	response, err := thorClient.InspectClauses(body, thorclient.Revision(blockID.String()))
	if err != nil {
		return nil, err
	}

	data := response[1].Data[2:]

	valueType, _ := big.NewInt(0).SetString(data[:64], 16)
	if valueType.Cmp(big.NewInt(32)) != 0 {
		return nil, errors.New("Wrong type returned by the contract")
	}
	data = data[64:]
	amount, _ := big.NewInt(0).SetString(data[:64], 16)
	data = data[64:]

	candidates := make([]Candidate, amount.Uint64(), amount.Uint64())
	for index := uint64(0); index < amount.Uint64(); index++ {
		master := thor.MustParseAddress(data[24:64])
		data = data[64:]
		endorsor := thor.MustParseAddress(data[24:64])
		data = data[64:]
		identity, _ := hex.DecodeString(data[:64])
		data = data[64:]

		activeString := data[:64]
		active := true
		if activeString == "0000000000000000000000000000000000000000000000000000000000000000" {
			active = false
		}
		data = data[64:]

		candidate := Candidate{
			Master:    master,
			Endorsor:  endorsor,
			Indentity: identity,
			Active:    active,
		}
		candidates[index] = candidate
	}

	return candidates, nil
}

func shuffleCandidates(candidates []Candidate, nextBlockCandidates []Candidate, seed []byte, blockNumber uint32) []thor.Address {
	nextCandidates := make(map[thor.Address]Candidate)
	for _, p := range nextBlockCandidates {
		nextCandidates[p.Master] = p
	}

	var num [4]byte
	binary.BigEndian.PutUint32(num[:], blockNumber)
	var list []struct {
		addr thor.Address
		hash thor.Bytes32
	}
	for _, p := range candidates {
		nextCandidate, ok := nextCandidates[p.Master]
		if p.Active || (ok && nextCandidate.Active) {
			list = append(list, struct {
				addr thor.Address
				hash thor.Bytes32
			}{
				p.Master,
				thor.Blake2b(seed, num[:], p.Master.Bytes()),
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].hash.Bytes(), list[j].hash.Bytes()) < 0
	})

	shuffled := make([]thor.Address, 0, len(list))
	for _, t := range list {
		shuffled = append(shuffled, t.addr)
	}
	return shuffled
}
