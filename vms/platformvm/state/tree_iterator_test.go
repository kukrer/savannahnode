// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"testing"
	"time"

	"github.com/google/btree"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/ids"
)

func TestTreeIterator(t *testing.T) {
	require := require.New(t)
	stakers := []*Staker{
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(0, 0),
		},
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(1, 0),
		},
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(2, 0),
		},
	}

	tree := btree.New(defaultTreeDegree)
	for _, staker := range stakers {
		require.Nil(tree.ReplaceOrInsert(staker))
	}

	it := NewTreeIterator(tree)
	for _, staker := range stakers {
		require.True(it.Next())
		require.Equal(staker, it.Value())
	}
	require.False(it.Next())
	it.Release()
}

func TestTreeIteratorNil(t *testing.T) {
	require := require.New(t)
	it := NewTreeIterator(nil)
	require.False(it.Next())
	it.Release()
}

func TestTreeIteratorEarlyRelease(t *testing.T) {
	require := require.New(t)
	stakers := []*Staker{
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(0, 0),
		},
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(1, 0),
		},
		{
			TxID:     ids.GenerateTestID(),
			NextTime: time.Unix(2, 0),
		},
	}

	tree := btree.New(defaultTreeDegree)
	for _, staker := range stakers {
		require.Nil(tree.ReplaceOrInsert(staker))
	}

	it := NewTreeIterator(tree)
	require.True(it.Next())
	it.Release()
	require.False(it.Next())
}
