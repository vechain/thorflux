package authority

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"time"

	"github.com/vechain/thorflux/excel"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/api"

	"github.com/vechain/thorflux/types"

	"github.com/ethereum/go-ethereum/common/hexutil"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

var topFiveProposers = 5

type List struct {
	candidates []Candidate
	thor       *thorclient.Client
	owners     map[thor.Address]*excel.Owner
	ownersRepo string
}

func NewList(thorClient *thorclient.Client, ownersRepo string) *List {
	return &List{
		thor:       thorClient,
		candidates: make([]Candidate, 0),
		owners:     make(map[thor.Address]*excel.Owner),
		ownersRepo: ownersRepo,
	}
}

func (l *List) ShouldReset(block *api.JSONExpandedBlock) bool {
	if len(l.candidates) == 0 {
		return true
	}
	candidateMap := make(map[thor.Address]bool)
	for _, candidate := range l.candidates {
		candidateMap[candidate.Endorsor] = true
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

	l.RefreshOwnersList()
	return nil
}

func (l *List) RefreshOwnersList() {
	owners, err := excel.ParseOwnersFromXLSX(l.ownersRepo)
	if err != nil {
		slog.Warn("Cannot parse owners file", "error", err)
	} else {
		for _, owner := range *owners {
			l.owners[owner.MasterAddress] = &owner
		}
	}
}

func (l *List) Shuffled(prev *api.JSONExpandedBlock, seed []byte) ([]thor.Address, error) {
	if len(l.candidates) == 0 {
		if err := l.Init(prev.ID); err != nil {
			return nil, fmt.Errorf("failed to initialize authority list: %w", err)
		}
	}
	return shuffleCandidates(l.candidates, seed, prev.Number), nil
}

func (l *List) Write(event *types.Event) []*write.Point {
	if event.HayabusaStatus.Active {
		return nil
	}

	block := event.Block
	prev := event.Prev
	epoch := block.Number / thor.EpochLength()

	points := make([]*write.Point, 0)

	if prev != nil {
		// Process recent slots
		slotsSinceLastBlock := (block.Timestamp - prev.Timestamp + thor.BlockInterval() - 1) / thor.BlockInterval()

		// Write detailed slot data for the last hour (360 slots)
		const detailedSlotWindow = 360
		startSlot := uint64(0)
		if slotsSinceLastBlock > detailedSlotWindow {
			startSlot = slotsSinceLastBlock - detailedSlotWindow
		}
		proposer := block.Signer
		ownerName, contact := l.getOwnerAndContactForProposer(proposer)

		p := influxdb2.NewPoint(
			"recent_slots",
			map[string]string{"chain_tag": event.DefaultTags["chain_tag"], "filled": "1", "proposer": proposer.String(), "owner": ownerName, "contact": contact},
			map[string]interface{}{"epoch": epoch, "block_number": block.Number},
			event.Timestamp,
		)
		points = append(points, p)

		proposers := make(map[string]interface{})
		for _, candidate := range l.candidates {
			proposers[candidate.Master.String()] = candidate.Active
		}

		shuffledCandidates, err := l.Shuffled(prev, event.Seed)
		if err != nil {
			slog.Error("Error shuffling", "err", err.Error())
		}

		proposers["signer"] = block.Signer.String()
		topProposers := topFiveProposers
		if len(shuffledCandidates) < topFiveProposers {
			topProposers = len(shuffledCandidates)
		}
		for idx := range topProposers {
			proposers["candidates"+strconv.Itoa(idx)] = shuffledCandidates[idx].String()
		}
		authNodes := influxdb2.NewPoint(
			"authority_nodes",
			map[string]string{"chain_tag": event.DefaultTags["chain_tag"], "block_number": strconv.Itoa(int(block.Number))},
			proposers,
			time.Unix(int64(block.Timestamp), 0),
		)
		points = append(points, authNodes)

		if len(shuffledCandidates) > 0 && shuffledCandidates[0].String() != block.Signer.String() {
			missedSlotData := make(map[string]interface{})
			missedSlotData["expected_proposer"] = shuffledCandidates[0].String()
			missedSlot := influxdb2.NewPoint(
				"missed_slots",
				map[string]string{"chain_tag": event.DefaultTags["chain_tag"], "block_number": strconv.Itoa(int(block.Number)), "actual_proposer": block.Signer.String()},
				missedSlotData,
				time.Unix(int64(block.Timestamp), 0),
			)
			points = append(points, missedSlot)
		}

		for a := startSlot; a < slotsSinceLastBlock-1; a++ {
			rawTime := prev.Timestamp + a*thor.BlockInterval()
			slotTime := time.Unix(int64(rawTime), 0)
			isFilled := a == slotsSinceLastBlock-1
			value := 0
			if isFilled {
				value = 1
			} else {
				slog.Warn("EMPTY SLOT", "number", block.Number)
				if int(a) >= len(shuffledCandidates) {
					slog.Error("Out of bounds", "shuffleCandidates", shuffledCandidates)
					proposer = thor.Address{}
				} else {
					proposer = shuffledCandidates[a]
				}
			}

			ownerName, contact = l.getOwnerAndContactForProposer(proposer)
			p := influxdb2.NewPoint(
				"recent_slots",
				map[string]string{"chain_tag": event.DefaultTags["chain_tag"], "filled": fmt.Sprintf("%d", value), "proposer": proposer.String(), "owner": ownerName, "contact": contact},
				map[string]interface{}{"epoch": epoch, "block_number": block.Number},
				slotTime,
			)
			points = append(points, p)
		}

		// Aggregate older slot data
		if slotsSinceLastBlock > detailedSlotWindow {
			olderMissedSlots := slotsSinceLastBlock - detailedSlotWindow - 1
			olderFilledSlots := 1 // The previous block
			aggregateTime := time.Unix(int64(prev.Timestamp), 0)

			p := influxdb2.NewPoint(
				"aggregated_slots",
				map[string]string{"chain_tag": event.DefaultTags["chain_tag"]},
				map[string]interface{}{
					"missed": olderMissedSlots,
					"filled": olderFilledSlots,
				},
				aggregateTime,
			)
			points = append(points, p)
		}
	}

	if l.ShouldReset(block) {
		slog.Info("Authority list reset", "block", block.ID, "number", block.Number)
		if err := l.Init(block.ID); err != nil {
			slog.Error("failed to initialize authority list", "error", err)
			return points
		}
	} else if block.Number%(thor.EpochLength()*2) == 0 {
		slog.Info("Refreshing owners list", "block", block.ID, "number", block.Number)
		l.RefreshOwnersList()
	}

	return points
}

func listAllCandidates(thorClient *thorclient.Client, blockID thor.Bytes32) ([]Candidate, error) {
	gas := uint64(3000000)
	caller := thor.MustParseAddress("0x6d95e6dca01d109882fe1726a2fb9865fa41e7aa")
	gasPayer := thor.MustParseAddress("0xd3ae78222beadb038203be21ed5ce7c9b1bff602")
	authorityContract := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")

	contract, _ := hex.DecodeString(AuthorityListAll)
	clauses := api.Clauses{
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

	body := &api.BatchCallData{
		Gas:      gas,
		Caller:   &caller,
		GasPayer: &gasPayer,
		Clauses:  clauses,
	}

	response, err := thorClient.InspectClauses(body, thorclient.Revision(blockID.String()))
	if err != nil {
		return nil, err
	}

	data := response[1].Data[2:]

	valueType, _ := big.NewInt(0).SetString(data[:64], 16)
	if valueType.Cmp(big.NewInt(32)) != 0 {
		return nil, errors.New("wrong type returned by the contract")
	}
	data = data[64:]
	amount, _ := big.NewInt(0).SetString(data[:64], 16)
	data = data[64:]

	candidates := make([]Candidate, amount.Uint64())
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

func shuffleCandidates(candidates []Candidate, seed []byte, blockNumber uint32) []thor.Address {
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], blockNumber)
	var list []struct {
		addr thor.Address
		hash thor.Bytes32
	}
	for _, p := range candidates {
		if p.Active {
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

func (l *List) getOwnerAndContactForProposer(proposer thor.Address) (string, string) {
	ownerName := "?"
	contact := "?"
	owner := l.owners[proposer]
	if owner != nil {
		ownerName = owner.Owner
		contact = owner.PointOfContact
	}
	return ownerName, contact
}
