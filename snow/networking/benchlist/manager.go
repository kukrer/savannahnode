// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package benchlist

import (
	"errors"
	"sync"
	"time"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/snow/validators"
	"github.com/kukrer/savannahnode/utils/constants"
)

var (
	errUnknownValidators = errors.New("unknown validator set for provided chain")

	_ Manager = &manager{}
)

// Manager provides an interface for a benchlist to register whether
// queries have been successful or unsuccessful and place validators with
// consistently failing queries on a benchlist to prevent waiting up to
// the full network timeout for their responses.
type Manager interface {
	// RegisterResponse registers that we receive a request response from [nodeID]
	// regarding [chainID] within the timeout
	RegisterResponse(chainID ids.ID, nodeID ids.NodeID)
	// RegisterFailure registers that a request to [nodeID] regarding
	// [chainID] timed out
	RegisterFailure(chainID ids.ID, nodeID ids.NodeID)
	// RegisterChain registers a new chain with metrics under [namespace]
	RegisterChain(ctx *snow.ConsensusContext) error
	// IsBenched returns true if messages to [nodeID] regarding chain [chainID]
	// should not be sent over the network and should immediately fail.
	// Returns false if such messages should be sent, or if the chain is unknown.
	IsBenched(nodeID ids.NodeID, chainID ids.ID) bool
	// GetBenched returns an array of chainIDs where the specified
	// [nodeID] is benched. If called on an id.ShortID that does
	// not map to a validator, it will return an empty array.
	GetBenched(nodeID ids.NodeID) []ids.ID
}

// Config defines the configuration for a benchlist
type Config struct {
	Benchable              Benchable          `json:"-"`
	Validators             validators.Manager `json:"-"`
	StakingEnabled         bool               `json:"-"`
	Threshold              int                `json:"threshold"`
	MinimumFailingDuration time.Duration      `json:"minimumFailingDuration"`
	Duration               time.Duration      `json:"duration"`
	MaxPortion             float64            `json:"maxPortion"`
}

type manager struct {
	config *Config
	// Chain ID --> benchlist for that chain.
	// Each benchlist is safe for concurrent access.
	chainBenchlists map[ids.ID]Benchlist

	lock sync.RWMutex
}

// NewManager returns a manager for chain-specific query benchlisting
func NewManager(config *Config) Manager {
	// If the maximum portion of validators allowed to be benchlisted
	// is 0, return the no-op benchlist
	if config.MaxPortion <= 0 {
		return NewNoBenchlist()
	}
	return &manager{
		config:          config,
		chainBenchlists: make(map[ids.ID]Benchlist),
	}
}

// IsBenched returns true if messages to [nodeID] regarding [chainID]
// should not be sent over the network and should immediately fail.
func (m *manager) IsBenched(nodeID ids.NodeID, chainID ids.ID) bool {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return false
	}
	isBenched := benchlist.IsBenched(nodeID)
	return isBenched
}

// GetBenched returns an array of chainIDs where the specified
// [nodeID] is benched. If called on an id.ShortID that does
// not map to a validator, it will return an empty array.
func (m *manager) GetBenched(nodeID ids.NodeID) []ids.ID {
	m.lock.RLock()
	defer m.lock.RUnlock()

	benched := []ids.ID{}
	for chainID, benchlist := range m.chainBenchlists {
		if !benchlist.IsBenched(nodeID) {
			continue
		}
		benched = append(benched, chainID)
	}
	return benched
}

func (m *manager) RegisterChain(ctx *snow.ConsensusContext) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, exists := m.chainBenchlists[ctx.ChainID]; exists {
		return nil
	}

	var (
		vdrs validators.Set
		ok   bool
	)
	if m.config.StakingEnabled {
		vdrs, ok = m.config.Validators.GetValidators(ctx.SubnetID)
	} else {
		// If staking is disabled, everyone validates every chain
		vdrs, ok = m.config.Validators.GetValidators(constants.PrimaryNetworkID)
	}
	if !ok {
		return errUnknownValidators
	}

	benchlist, err := NewBenchlist(
		ctx.ChainID,
		ctx.Log,
		m.config.Benchable,
		vdrs,
		m.config.Threshold,
		m.config.MinimumFailingDuration,
		m.config.Duration,
		m.config.MaxPortion,
		ctx.Registerer,
	)
	if err != nil {
		return err
	}

	m.chainBenchlists[ctx.ChainID] = benchlist
	return nil
}

func (m *manager) RegisterResponse(chainID ids.ID, nodeID ids.NodeID) {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return
	}
	benchlist.RegisterResponse(nodeID)
}

func (m *manager) RegisterFailure(chainID ids.ID, nodeID ids.NodeID) {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return
	}
	benchlist.RegisterFailure(nodeID)
}

type noBenchlist struct{}

// NewNoBenchlist returns an empty benchlist that will never stop any queries
func NewNoBenchlist() Manager { return &noBenchlist{} }

func (noBenchlist) RegisterChain(*snow.ConsensusContext) error { return nil }
func (noBenchlist) RegisterResponse(ids.ID, ids.NodeID)        {}
func (noBenchlist) RegisterFailure(ids.ID, ids.NodeID)         {}
func (noBenchlist) IsBenched(ids.NodeID, ids.ID) bool          { return false }
func (noBenchlist) GetBenched(ids.NodeID) []ids.ID             { return []ids.ID{} }
