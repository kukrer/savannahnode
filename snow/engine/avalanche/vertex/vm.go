// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow/consensus/snowstorm"
	"github.com/kukrer/savannahnode/snow/engine/common"
)

// DAGVM defines the minimum functionality that an avalanche VM must
// implement
type DAGVM interface {
	common.VM
	Getter

	// Return any transactions that have not been sent to consensus yet
	PendingTxs() []snowstorm.Tx

	// Convert a stream of bytes to a transaction or return an error
	ParseTx(tx []byte) (snowstorm.Tx, error)
}

// Getter defines the functionality for fetching a tx/block by its ID.
type Getter interface {
	// Retrieve a transaction that was submitted previously
	GetTx(ids.ID) (snowstorm.Tx, error)
}
