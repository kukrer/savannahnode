// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/snow/choices"
	"github.com/kukrer/savannahnode/snow/consensus/snowman"
	"github.com/kukrer/savannahnode/snow/engine/common"
	"github.com/kukrer/savannahnode/snow/engine/snowman/block"
	"github.com/kukrer/savannahnode/utils/timer"
	"github.com/kukrer/savannahnode/version"
)

// Parameters for delaying bootstrapping to avoid potential CPU burns
const bootstrappingDelay = 10 * time.Second

var (
	_ common.BootstrapableEngine = &bootstrapper{}

	errUnexpectedTimeout = errors.New("unexpected timeout fired")
)

type bootstrapper struct {
	Config

	// list of NoOpsHandler for messages dropped by bootstrapper
	common.StateSummaryFrontierHandler
	common.AcceptedStateSummaryHandler
	common.PutHandler
	common.QueryHandler
	common.ChitsHandler
	common.AppHandler

	common.Bootstrapper
	common.Fetcher
	*metrics

	started bool

	// Greatest height of the blocks passed in ForceAccepted
	tipHeight uint64
	// Height of the last accepted block when bootstrapping starts
	startingHeight uint64
	// Number of blocks that were fetched on ForceAccepted
	initiallyFetched uint64
	// Time that ForceAccepted was last called
	startTime time.Time

	// number of state transitions executed
	executedStateTransitions int

	parser *parser

	awaitingTimeout bool

	// fetchFrom is the set of nodes that we can fetch the next container from.
	// When a container is fetched, the nodeID is removed from [fetchFrom] to
	// attempt to limit a single request to a peer at any given time. When the
	// response is received, either and Ancestors or an AncestorsFailed, the
	// nodeID will be added back to [fetchFrom] unless the Ancestors message is
	// empty. This is to attempt to prevent requesting containers from that peer
	// again.
	fetchFrom ids.NodeIDSet
}

func New(config Config, onFinished func(lastReqID uint32) error) (common.BootstrapableEngine, error) {
	metrics, err := newMetrics("bs", config.Ctx.Registerer)
	if err != nil {
		return nil, err
	}

	b := &bootstrapper{
		Config:                      config,
		metrics:                     metrics,
		StateSummaryFrontierHandler: common.NewNoOpStateSummaryFrontierHandler(config.Ctx.Log),
		AcceptedStateSummaryHandler: common.NewNoOpAcceptedStateSummaryHandler(config.Ctx.Log),
		PutHandler:                  common.NewNoOpPutHandler(config.Ctx.Log),
		QueryHandler:                common.NewNoOpQueryHandler(config.Ctx.Log),
		ChitsHandler:                common.NewNoOpChitsHandler(config.Ctx.Log),
		AppHandler:                  common.NewNoOpAppHandler(config.Ctx.Log),

		Fetcher: common.Fetcher{
			OnFinished: onFinished,
		},
		executedStateTransitions: math.MaxInt32,
	}

	b.parser = &parser{
		log:         config.Ctx.Log,
		numAccepted: b.numAccepted,
		numDropped:  b.numDropped,
		vm:          b.VM,
	}
	if err := b.Blocked.SetParser(b.parser); err != nil {
		return nil, err
	}

	config.Bootstrapable = b
	b.Bootstrapper = common.NewCommonBootstrapper(config.Config)

	return b, nil
}

func (b *bootstrapper) Start(startReqID uint32) error {
	b.Ctx.Log.Info("starting bootstrapper")

	b.Ctx.SetState(snow.Bootstrapping)
	if err := b.VM.SetState(snow.Bootstrapping); err != nil {
		return fmt.Errorf("failed to notify VM that bootstrapping has started: %w",
			err)
	}

	// Set the starting height
	lastAcceptedID, err := b.VM.LastAccepted()
	if err != nil {
		return fmt.Errorf("couldn't get last accepted ID: %w", err)
	}
	lastAccepted, err := b.VM.GetBlock(lastAcceptedID)
	if err != nil {
		return fmt.Errorf("couldn't get last accepted block: %w", err)
	}
	b.startingHeight = lastAccepted.Height()
	b.Config.SharedCfg.RequestID = startReqID

	if !b.StartupTracker.ShouldStart() {
		return nil
	}

	b.started = true
	return b.Startup()
}

// Ancestors handles the receipt of multiple containers. Should be received in
// response to a GetAncestors message to [nodeID] with request ID [requestID]
func (b *bootstrapper) Ancestors(nodeID ids.NodeID, requestID uint32, blks [][]byte) error {
	// Make sure this is in response to a request we made
	wantedBlkID, ok := b.OutstandingRequests.Remove(nodeID, requestID)
	if !ok { // this message isn't in response to a request we made
		b.Ctx.Log.Debug("received unexpected Ancestors",
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
		)
		return nil
	}

	lenBlks := len(blks)
	if lenBlks == 0 {
		b.Ctx.Log.Debug("received Ancestors with no block",
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
		)

		b.markUnavailable(nodeID)

		// Send another request for this
		return b.fetch(wantedBlkID)
	}

	// This node has responded - so add it back into the set
	b.fetchFrom.Add(nodeID)

	if lenBlks > b.Config.AncestorsMaxContainersReceived {
		blks = blks[:b.Config.AncestorsMaxContainersReceived]
		b.Ctx.Log.Debug("ignoring containers in Ancestors",
			zap.Int("numContainers", lenBlks-b.Config.AncestorsMaxContainersReceived),
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
		)
	}

	blocks, err := block.BatchedParseBlock(b.VM, blks)
	if err != nil { // the provided blocks couldn't be parsed
		b.Ctx.Log.Debug("failed to parse blocks in Ancestors",
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
			zap.Error(err),
		)
		return b.fetch(wantedBlkID)
	}

	if len(blocks) == 0 {
		b.Ctx.Log.Debug("parsing blocks returned an empty set of blocks",
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
		)
		return b.fetch(wantedBlkID)
	}

	requestedBlock := blocks[0]
	if actualID := requestedBlock.ID(); actualID != wantedBlkID {
		b.Ctx.Log.Debug("first block is not the requested block",
			zap.Stringer("expectedBlkID", wantedBlkID),
			zap.Stringer("blkID", actualID),
		)
		return b.fetch(wantedBlkID)
	}

	blockSet := make(map[ids.ID]snowman.Block, len(blocks))
	for _, block := range blocks[1:] {
		blockSet[block.ID()] = block
	}
	return b.process(requestedBlock, blockSet)
}

func (b *bootstrapper) GetAncestorsFailed(nodeID ids.NodeID, requestID uint32) error {
	blkID, ok := b.OutstandingRequests.Remove(nodeID, requestID)
	if !ok {
		b.Ctx.Log.Debug("unexpectedly called GetAncestorsFailed",
			zap.Stringer("nodeID", nodeID),
			zap.Uint32("requestID", requestID),
		)
		return nil
	}

	// This node timed out their request, so we can add them back to [fetchFrom]
	b.fetchFrom.Add(nodeID)

	// Send another request for this
	return b.fetch(blkID)
}

func (b *bootstrapper) Connected(nodeID ids.NodeID, nodeVersion *version.Application) error {
	if err := b.VM.Connected(nodeID, nodeVersion); err != nil {
		return err
	}

	if err := b.StartupTracker.Connected(nodeID, nodeVersion); err != nil {
		return err
	}
	// Ensure fetchFrom reflects proper validator list
	if b.Beacons.Contains(nodeID) {
		b.fetchFrom.Add(nodeID)
	}

	if b.started || !b.StartupTracker.ShouldStart() {
		return nil
	}

	b.started = true
	return b.Startup()
}

func (b *bootstrapper) Disconnected(nodeID ids.NodeID) error {
	if err := b.VM.Disconnected(nodeID); err != nil {
		return err
	}

	if err := b.StartupTracker.Disconnected(nodeID); err != nil {
		return err
	}

	b.markUnavailable(nodeID)
	return nil
}

func (b *bootstrapper) Timeout() error {
	if !b.awaitingTimeout {
		return errUnexpectedTimeout
	}
	b.awaitingTimeout = false

	if !b.Config.Subnet.IsBootstrapped() {
		return b.Restart(true)
	}
	b.fetchETA.Set(0)
	return b.OnFinished(b.Config.SharedCfg.RequestID)
}

func (b *bootstrapper) Gossip() error { return nil }

func (b *bootstrapper) Shutdown() error {
	b.Ctx.Log.Info("shutting down bootstrapper")
	return b.VM.Shutdown()
}

func (b *bootstrapper) Notify(common.Message) error { return nil }

func (b *bootstrapper) HealthCheck() (interface{}, error) {
	vmIntf, vmErr := b.VM.HealthCheck()
	intf := map[string]interface{}{
		"consensus": struct{}{},
		"vm":        vmIntf,
	}
	return intf, vmErr
}

func (b *bootstrapper) GetVM() common.VM { return b.VM }

func (b *bootstrapper) ForceAccepted(acceptedContainerIDs []ids.ID) error {
	pendingContainerIDs := b.Blocked.MissingIDs()

	// Initialize the fetch from set to the currently preferred peers
	b.fetchFrom = b.StartupTracker.PreferredPeers()

	// Append the list of accepted container IDs to pendingContainerIDs to ensure
	// we iterate over every container that must be traversed.
	pendingContainerIDs = append(pendingContainerIDs, acceptedContainerIDs...)
	toProcess := make([]snowman.Block, 0, len(pendingContainerIDs))
	b.Ctx.Log.Debug("starting bootstrapping",
		zap.Int("numPendingBlocks", len(pendingContainerIDs)),
		zap.Int("numAcceptedBlocks", len(acceptedContainerIDs)),
	)
	for _, blkID := range pendingContainerIDs {
		b.Blocked.AddMissingID(blkID)

		// TODO: if `GetBlock` returns an error other than
		// `database.ErrNotFound`, then the error should be propagated.
		blk, err := b.VM.GetBlock(blkID)
		if err != nil {
			if err := b.fetch(blkID); err != nil {
				return err
			}
			continue
		}
		toProcess = append(toProcess, blk)
	}

	b.initiallyFetched = b.Blocked.PendingJobs()
	b.startTime = time.Now()

	// Process received blocks
	for _, blk := range toProcess {
		if err := b.process(blk, nil); err != nil {
			return err
		}
	}

	return b.checkFinish()
}

// Get block [blkID] and its ancestors from a validator
func (b *bootstrapper) fetch(blkID ids.ID) error {
	// Make sure we haven't already requested this block
	if b.OutstandingRequests.Contains(blkID) {
		return nil
	}

	// Make sure we don't already have this block
	if _, err := b.VM.GetBlock(blkID); err == nil {
		return b.checkFinish()
	}

	validatorID, ok := b.fetchFrom.Peek()
	if !ok {
		return fmt.Errorf("dropping request for %s as there are no validators", blkID)
	}

	// We only allow one outbound request at a time from a node
	b.markUnavailable(validatorID)

	b.Config.SharedCfg.RequestID++

	b.OutstandingRequests.Add(validatorID, b.Config.SharedCfg.RequestID, blkID)
	b.Config.Sender.SendGetAncestors(validatorID, b.Config.SharedCfg.RequestID, blkID) // request block and ancestors
	return nil
}

// markUnavailable removes [nodeID] from the set of peers used to fetch
// ancestors. If the set becomes empty, it is reset to the currently preferred
// peers so bootstrapping can continue.
func (b *bootstrapper) markUnavailable(nodeID ids.NodeID) {
	b.fetchFrom.Remove(nodeID)

	// if [fetchFrom] has become empty, reset it to the currently preferred
	// peers
	if b.fetchFrom.Len() == 0 {
		b.fetchFrom = b.StartupTracker.PreferredPeers()
	}
}

func (b *bootstrapper) Clear() error {
	if err := b.Config.Blocked.Clear(); err != nil {
		return err
	}
	return b.Config.Blocked.Commit()
}

// process a series of consecutive blocks starting at [blk].
//
//   - blk is a block that is assumed to have been marked as acceptable by the
//     bootstrapping engine.
//   - processingBlocks is a set of blocks that can be used to lookup blocks.
//     This enables the engine to process multiple blocks without relying on the
//     VM to have stored blocks during `ParseBlock`.
//
// If [blk]'s height is <= the last accepted height, then it will be removed
// from the missingIDs set.
func (b *bootstrapper) process(blk snowman.Block, processingBlocks map[ids.ID]snowman.Block) error {
	for {
		blkID := blk.ID()
		if b.Halted() {
			// We must add in [blkID] to the set of missing IDs so that we are
			// guaranteed to continue processing from this state when the
			// bootstrapper is restarted.
			b.Blocked.AddMissingID(blkID)
			return b.Blocked.Commit()
		}

		b.Blocked.RemoveMissingID(blkID)

		status := blk.Status()
		// The status should never be rejected here - but we check to fail as
		// quickly as possible
		if status == choices.Rejected {
			return fmt.Errorf("bootstrapping wants to accept %s, however it was previously rejected", blkID)
		}

		blkHeight := blk.Height()
		if status == choices.Accepted || blkHeight <= b.startingHeight {
			// We can stop traversing, as we have reached the accepted frontier
			if err := b.Blocked.Commit(); err != nil {
				return err
			}
			return b.checkFinish()
		}

		// If this block is going to be accepted, make sure to update the
		// tipHeight for logging
		if blkHeight > b.tipHeight {
			b.tipHeight = blkHeight
		}

		pushed, err := b.Blocked.Push(&blockJob{
			parser:      b.parser,
			log:         b.Ctx.Log,
			numAccepted: b.numAccepted,
			numDropped:  b.numDropped,
			blk:         blk,
			vm:          b.VM,
		})
		if err != nil {
			return err
		}

		if !pushed {
			// We can stop traversing, as we have reached a block that we
			// previously pushed onto the jobs queue
			if err := b.Blocked.Commit(); err != nil {
				return err
			}
			return b.checkFinish()
		}

		// We added a new block to the queue, so track that it was fetched
		b.numFetched.Inc()

		// Periodically log progress
		blocksFetchedSoFar := b.Blocked.Jobs.PendingJobs()
		if blocksFetchedSoFar%common.StatusUpdateFrequency == 0 {
			totalBlocksToFetch := b.tipHeight - b.startingHeight
			eta := timer.EstimateETA(
				b.startTime,
				blocksFetchedSoFar-b.initiallyFetched, // Number of blocks we have fetched during this run
				totalBlocksToFetch-b.initiallyFetched, // Number of blocks we expect to fetch during this run
			)
			b.fetchETA.Set(float64(eta))

			if !b.Config.SharedCfg.Restarted {
				b.Ctx.Log.Info("fetching blocks",
					zap.Uint64("numFetchedBlocks", blocksFetchedSoFar),
					zap.Uint64("numTotalBlocks", totalBlocksToFetch),
					zap.Duration("eta", eta),
				)
			} else {
				b.Ctx.Log.Debug("fetching blocks",
					zap.Uint64("numFetchedBlocks", blocksFetchedSoFar),
					zap.Uint64("numTotalBlocks", totalBlocksToFetch),
					zap.Duration("eta", eta),
				)
			}
		}

		// Attempt to traverse to the next block
		parentID := blk.Parent()

		// First check if the parent is in the processing blocks set
		parent, ok := processingBlocks[parentID]
		if ok {
			blk = parent
			continue
		}

		// If the parent is not available in processing blocks, attempt to get
		// the block from the vm
		parent, err = b.VM.GetBlock(parentID)
		if err == nil {
			blk = parent
			continue
		}
		// TODO: report errors that aren't `database.ErrNotFound`

		// If the block wasn't able to be acquired immediately, attempt to fetch
		// it
		b.Blocked.AddMissingID(parentID)
		if err := b.fetch(parentID); err != nil {
			return err
		}

		if err := b.Blocked.Commit(); err != nil {
			return err
		}
		return b.checkFinish()
	}
}

// checkFinish repeatedly executes pending transactions and requests new frontier vertices until there aren't any new ones
// after which it finishes the bootstrap process
func (b *bootstrapper) checkFinish() error {
	if numPending := b.Blocked.NumMissingIDs(); numPending != 0 {
		return nil
	}

	if b.IsBootstrapped() || b.awaitingTimeout {
		return nil
	}

	if !b.Config.SharedCfg.Restarted {
		b.Ctx.Log.Info("executing blocks",
			zap.Uint64("numPendingJobs", b.Blocked.PendingJobs()),
		)
	} else {
		b.Ctx.Log.Debug("executing blocks",
			zap.Uint64("numPendingJobs", b.Blocked.PendingJobs()),
		)
	}

	executedBlocks, err := b.Blocked.ExecuteAll(
		b.Config.Ctx,
		b,
		b.Config.SharedCfg.Restarted,
		b.Ctx.ConsensusAcceptor,
		b.Ctx.DecisionAcceptor,
	)
	if err != nil || b.Halted() {
		return err
	}

	previouslyExecuted := b.executedStateTransitions
	b.executedStateTransitions = executedBlocks

	// Note that executedBlocks < c*previouslyExecuted ( 0 <= c < 1 ) is enforced
	// so that the bootstrapping process will terminate even as new blocks are
	// being issued.
	if b.Config.RetryBootstrap && executedBlocks > 0 && executedBlocks < previouslyExecuted/2 {
		return b.Restart(true)
	}

	// If there is an additional callback, notify them that this chain has been
	// synced.
	if b.Bootstrapped != nil {
		b.Bootstrapped()
	}

	// Notify the subnet that this chain is synced
	b.Config.Subnet.Bootstrapped(b.Ctx.ChainID)

	// If the subnet hasn't finished bootstrapping, this chain should remain
	// syncing.
	if !b.Config.Subnet.IsBootstrapped() {
		if !b.Config.SharedCfg.Restarted {
			b.Ctx.Log.Info("waiting for the remaining chains in this subnet to finish syncing")
		} else {
			b.Ctx.Log.Debug("waiting for the remaining chains in this subnet to finish syncing")
		}
		// Restart bootstrapping after [bootstrappingDelay] to keep up to date
		// on the latest tip.
		b.Config.Timer.RegisterTimeout(bootstrappingDelay)
		b.awaitingTimeout = true
		return nil
	}
	b.fetchETA.Set(0)
	return b.OnFinished(b.Config.SharedCfg.RequestID)
}
