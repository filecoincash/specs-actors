package miner

import (
	"sort"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type TerminationResult struct {
	// Sectors maps epochs at which sectors expired, to bitfields of sector
	// numbers.
	Sectors map[abi.ChainEpoch]*abi.BitField
	// Counts the number of partitions & sectors processed.
	PartitionsProcessed, SectorsProcessed uint64
}

func (t *TerminationResult) Add(newResult TerminationResult) error {
	if t.Sectors == nil {
		t.Sectors = make(map[abi.ChainEpoch]*abi.BitField, len(newResult.Sectors))
	}
	t.PartitionsProcessed += newResult.PartitionsProcessed
	t.SectorsProcessed += newResult.SectorsProcessed
	for epoch, newSectors := range newResult.Sectors {
		if oldSectors, ok := t.Sectors[epoch]; !ok {
			t.Sectors[epoch] = newSectors
		} else {
			var err error
			t.Sectors[epoch], err = bitfield.MergeBitFields(oldSectors, newSectors)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *TerminationResult) BelowLimit(maxPartitions, maxSectors uint64) bool {
	return t.PartitionsProcessed < maxPartitions && t.SectorsProcessed < maxSectors
}

func (t *TerminationResult) IsEmpty() bool {
	return t.SectorsProcessed == 0
}

func (t *TerminationResult) ForEach(cb func(epoch abi.ChainEpoch, sectors *abi.BitField) error) error {
	// We're sorting here, so iterating over the map is fine.
	epochs := make([]abi.ChainEpoch, 0, len(t.Sectors))
	for epoch := range t.Sectors { //nolint:nomaprange
		epochs = append(epochs, epoch)
	}
	sort.Slice(epochs, func(i, j int) bool {
		return epochs[i] < epochs[j]
	})
	for _, epoch := range epochs {
		err := cb(epoch, t.Sectors[epoch])
		if err != nil {
			return err
		}
	}
	return nil
}