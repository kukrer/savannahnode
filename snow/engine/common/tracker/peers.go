// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tracker

import (
	"sync"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow/validators"
	"github.com/kukrer/savannahnode/version"
)

var _ Peers = &peers{}

type Peers interface {
	validators.SetCallbackListener
	validators.Connector

	// ConnectedWeight returns the currently connected stake weight
	ConnectedWeight() uint64
	// PreferredPeers returns the currently connected validators. If there are
	// no currently connected validators then it will return the currently
	// connected peers.
	PreferredPeers() ids.NodeIDSet
}

type peers struct {
	lock sync.RWMutex
	// validators maps nodeIDs to their current stake weight
	validators map[ids.NodeID]uint64
	// connectedWeight contains the sum of all connected validator weights
	connectedWeight uint64
	// connectedValidators is the set of currently connected peers with a
	// non-zero stake weight
	connectedValidators ids.NodeIDSet
	// connectedPeers is the set of all connected peers
	connectedPeers ids.NodeIDSet
}

func NewPeers() Peers {
	return &peers{
		validators: make(map[ids.NodeID]uint64),
	}
}

func (p *peers) OnValidatorAdded(nodeID ids.NodeID, weight uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.validators[nodeID] = weight
	if p.connectedPeers.Contains(nodeID) {
		p.connectedWeight += weight
		p.connectedValidators.Add(nodeID)
	}
}

func (p *peers) OnValidatorRemoved(nodeID ids.NodeID, weight uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	delete(p.validators, nodeID)
	if p.connectedPeers.Contains(nodeID) {
		p.connectedWeight -= weight
		p.connectedValidators.Remove(nodeID)
	}
}

func (p *peers) OnValidatorWeightChanged(nodeID ids.NodeID, oldWeight, newWeight uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.validators[nodeID] = newWeight
	if p.connectedPeers.Contains(nodeID) {
		p.connectedWeight -= oldWeight
		p.connectedWeight += newWeight
	}
}

func (p *peers) Connected(nodeID ids.NodeID, _ *version.Application) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if weight, ok := p.validators[nodeID]; ok {
		p.connectedWeight += weight
		p.connectedValidators.Add(nodeID)
	}
	p.connectedPeers.Add(nodeID)
	return nil
}

func (p *peers) Disconnected(nodeID ids.NodeID) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if weight, ok := p.validators[nodeID]; ok {
		p.connectedWeight -= weight
		p.connectedValidators.Remove(nodeID)
	}
	p.connectedPeers.Remove(nodeID)
	return nil
}

func (p *peers) ConnectedWeight() uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.connectedWeight
}

func (p *peers) PreferredPeers() ids.NodeIDSet {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.connectedValidators.Len() == 0 {
		connectedPeers := ids.NewNodeIDSet(p.connectedPeers.Len())
		connectedPeers.Union(p.connectedPeers)
		return connectedPeers
	}

	connectedValidators := ids.NewNodeIDSet(p.connectedValidators.Len())
	connectedValidators.Union(p.connectedValidators)
	return connectedValidators
}
