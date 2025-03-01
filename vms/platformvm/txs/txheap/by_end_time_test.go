// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txheap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/vms/platformvm/txs"
	"github.com/kukrer/savannahnode/vms/platformvm/validator"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

func TestByStopTime(t *testing.T) {
	require := require.New(t)

	txHeap := NewByEndTime()

	baseTime := time.Now()

	utx0 := &txs.AddValidatorTx{
		Validator: validator.Validator{
			NodeID: ids.NodeID{0},
			Start:  uint64(baseTime.Unix()),
			End:    uint64(baseTime.Unix()) + 1,
		},
		RewardsOwner: &secp256k1fx.OutputOwners{},
	}
	tx0 := &txs.Tx{Unsigned: utx0}
	err := tx0.Sign(txs.Codec, nil)
	require.NoError(err)

	utx1 := &txs.AddValidatorTx{
		Validator: validator.Validator{
			NodeID: ids.NodeID{1},
			Start:  uint64(baseTime.Unix()),
			End:    uint64(baseTime.Unix()) + 2,
		},
		RewardsOwner: &secp256k1fx.OutputOwners{},
	}
	tx1 := &txs.Tx{Unsigned: utx1}
	err = tx1.Sign(txs.Codec, nil)
	require.NoError(err)

	utx2 := &txs.AddValidatorTx{
		Validator: validator.Validator{
			NodeID: ids.NodeID{1},
			Start:  uint64(baseTime.Unix()),
			End:    uint64(baseTime.Unix()) + 3,
		},
		RewardsOwner: &secp256k1fx.OutputOwners{},
	}
	tx2 := &txs.Tx{Unsigned: utx2}
	err = tx2.Sign(txs.Codec, nil)
	require.NoError(err)

	txHeap.Add(tx2)
	require.Equal(utx2.EndTime(), txHeap.Timestamp())

	txHeap.Add(tx1)
	require.Equal(utx1.EndTime(), txHeap.Timestamp())

	txHeap.Add(tx0)
	require.Equal(utx0.EndTime(), txHeap.Timestamp())
	require.Equal(tx0, txHeap.Peek())
}
