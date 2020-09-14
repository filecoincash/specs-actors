package migration

import (
	"context"

	address "github.com/filecoin-project/go-address"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/v2/actors/states"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
	smoothing2 "github.com/filecoin-project/specs-actors/v2/actors/util/smoothing"
)

type powerMigrator struct {
	actorsIn *states.TreeTop
}

func (m *powerMigrator) MigrateState(ctx context.Context, store cbor.IpldStore, head cid.Cid) (cid.Cid, error) {
	var inState power0.State
	if err := store.Get(ctx, head, &inState); err != nil {
		return cid.Undef, err
	}

	cronEventsRoot, err := m.migrateCronEvents(ctx, store, inState.CronEventQueue)
	if err != nil {
		return cid.Undef, err
	}

	claimsRoot, err := m.migrateClaims(ctx, store, inState.Claims)
	if err != nil {
		return cid.Undef, err
	}

	outState := power2.State{
		TotalRawBytePower:         inState.TotalRawBytePower,
		TotalBytesCommitted:       inState.TotalBytesCommitted,
		TotalQualityAdjPower:      inState.TotalQualityAdjPower,
		TotalQABytesCommitted:     inState.TotalQABytesCommitted,
		TotalPledgeCollateral:     inState.TotalPledgeCollateral,
		ThisEpochRawBytePower:     inState.ThisEpochRawBytePower,
		ThisEpochQualityAdjPower:  inState.ThisEpochQualityAdjPower,
		ThisEpochPledgeCollateral: inState.ThisEpochPledgeCollateral,
		ThisEpochQAPowerSmoothed:  smoothing2.FilterEstimate(*inState.ThisEpochQAPowerSmoothed),
		MinerCount:                inState.MinerCount,
		MinerAboveMinPowerCount:   inState.MinerAboveMinPowerCount,
		CronEventQueue:            cronEventsRoot,
		FirstCronEpoch:            inState.FirstCronEpoch,
		Claims:                    claimsRoot,
		ProofValidationBatch:      nil, // Set nil at the end of every epoch in cron handler
	}

	return store.Put(ctx, &outState)
}

func (m *powerMigrator) migrateCronEvents(ctx context.Context, store cbor.IpldStore, root cid.Cid) (cid.Cid, error) {
	// The HAMT has changed, but the value (an AMT[CronEvent] root) is identical.
	// The AMT queues may contain miner0.CronEventWorkerKeyChange, but these will be ignored by the miner
	// actor so are safe to leave behind.
	var _ = power0.CronEvent(power2.CronEvent{})

	return migrateHAMTRaw(ctx, store, root)
}

func (m *powerMigrator) migrateClaims(ctx context.Context, store cbor.IpldStore, root cid.Cid) (cid.Cid, error) {
	inMap, err := adt0.AsMap(adt0.WrapStore(ctx, store), root)
	if err != nil {
		return cid.Undef, err
	}
	outMap := adt2.MakeEmptyMap(adt2.WrapStore(ctx, store))

	var inClaim power0.Claim
	if err = inMap.ForEach(&inClaim, func(key string) error {
		// look up seal proof type from miner actor
		a, err := address.NewFromString(key)
		if err != nil {
			return err
		}
		minerActor, err := m.actorsIn.GetActor(ctx, address.Address(a))
		if err != nil {
			return err
		}
		var minerState miner0.State
		if err := store.Get(ctx, minerActor.Head, &minerState); err != nil {
			return err
		}
		info, err := minerState.GetInfo(adt0.WrapStore(ctx, store))
		if err != nil {
			return err
		}

		outClaim := power2.Claim{
			SealProofType:   info.SealProofType,
			RawBytePower:    inClaim.RawBytePower,
			QualityAdjPower: inClaim.QualityAdjPower,
		}
		return outMap.Put(StringKey(key), &outClaim)
	}); err != nil {
		return cid.Undef, err
	}

	return outMap.Root()
}
