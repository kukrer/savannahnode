// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"fmt"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"

	sbcon "github.com/kukrer/savannahnode/snow/consensus/snowball"
)

// Consensus is a snowball instance deciding between an unbounded number of
// non-transitive conflicts. After performing a network sample of k nodes, you
// should call collect with the responses.
type Consensus interface {
	fmt.Stringer

	// Takes in the context, alpha, betaVirtuous, and betaRogue
	Initialize(*snow.ConsensusContext, sbcon.Parameters) error

	// Returns the parameters that describe this snowstorm instance
	Parameters() sbcon.Parameters

	// Returns true if transaction <Tx> is virtuous.
	// That is, no transaction has been added that conflicts with <Tx>
	IsVirtuous(Tx) bool

	// Adds a new transaction to vote on. Returns if a critical error has
	// occurred.
	Add(Tx) error

	// Remove a transaction from the set of currently processing txs. It is
	// assumed that the provided transaction ID is currently processing.
	Remove(ids.ID) error

	// Returns true iff transaction <Tx> has been added
	Issued(Tx) bool

	// Returns the set of virtuous transactions
	// that have not yet been accepted or rejected
	Virtuous() ids.Set

	// Returns the currently preferred transactions to be finalized
	Preferences() ids.Set

	// Return the current virtuous transactions that are being voted on.
	VirtuousVoting() ids.Set

	// Returns the set of transactions conflicting with <Tx>
	Conflicts(Tx) ids.Set

	// Collects the results of a network poll. Assumes all transactions
	// have been previously added. Returns true if any statuses or preferences
	// changed. Returns if a critical error has occurred.
	RecordPoll(ids.Bag) (bool, error)

	// Returns true iff all remaining transactions are rogue. Note, it is
	// possible that after returning quiesce, a new decision may be added such
	// that this instance should no longer quiesce.
	Quiesce() bool

	// Returns true iff all added transactions have been finalized. Note, it is
	// possible that after returning finalized, a new decision may be added such
	// that this instance is no longer finalized.
	Finalized() bool

	// HealthCheck returns information about the consensus health.
	HealthCheck() (interface{}, error)
}
