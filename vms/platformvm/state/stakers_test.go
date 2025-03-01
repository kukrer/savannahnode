// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/database"
	"github.com/kukrer/savannahnode/ids"
)

func TestBaseStakersPruning(t *testing.T) {
	require := require.New(t)
	staker := newTestStaker()
	delegator := newTestStaker()
	delegator.SubnetID = staker.SubnetID
	delegator.NodeID = staker.NodeID

	v := newBaseStakers()

	v.PutValidator(staker)

	_, err := v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)

	v.PutDelegator(delegator)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)

	v.DeleteValidator(staker)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.ErrorIs(err, database.ErrNotFound)

	v.DeleteDelegator(delegator)

	require.Empty(v.validators)

	v.PutValidator(staker)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)

	v.PutDelegator(delegator)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)

	v.DeleteDelegator(delegator)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)

	v.DeleteValidator(staker)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.ErrorIs(err, database.ErrNotFound)

	require.Empty(v.validators)
}

func TestBaseStakersValidator(t *testing.T) {
	require := require.New(t)
	staker := newTestStaker()
	delegator := newTestStaker()

	v := newBaseStakers()

	v.PutDelegator(delegator)

	_, err := v.GetValidator(ids.GenerateTestID(), delegator.NodeID)
	require.ErrorIs(err, database.ErrNotFound)

	_, err = v.GetValidator(delegator.SubnetID, ids.GenerateTestNodeID())
	require.ErrorIs(err, database.ErrNotFound)

	_, err = v.GetValidator(delegator.SubnetID, delegator.NodeID)
	require.ErrorIs(err, database.ErrNotFound)

	stakerIterator := v.GetStakerIterator()
	assertIteratorsEqual(t, NewSliceIterator(delegator), stakerIterator)

	v.PutValidator(staker)

	returnedStaker, err := v.GetValidator(staker.SubnetID, staker.NodeID)
	require.NoError(err)
	require.Equal(staker, returnedStaker)

	v.DeleteDelegator(delegator)

	stakerIterator = v.GetStakerIterator()
	assertIteratorsEqual(t, NewSliceIterator(staker), stakerIterator)

	v.DeleteValidator(staker)

	_, err = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.ErrorIs(err, database.ErrNotFound)

	stakerIterator = v.GetStakerIterator()
	assertIteratorsEqual(t, EmptyIterator, stakerIterator)
}

func TestBaseStakersDelegator(t *testing.T) {
	staker := newTestStaker()
	delegator := newTestStaker()

	v := newBaseStakers()

	delegatorIterator := v.GetDelegatorIterator(delegator.SubnetID, delegator.NodeID)
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)

	v.PutDelegator(delegator)

	delegatorIterator = v.GetDelegatorIterator(delegator.SubnetID, ids.GenerateTestNodeID())
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)

	delegatorIterator = v.GetDelegatorIterator(delegator.SubnetID, delegator.NodeID)
	assertIteratorsEqual(t, NewSliceIterator(delegator), delegatorIterator)

	v.DeleteDelegator(delegator)

	delegatorIterator = v.GetDelegatorIterator(delegator.SubnetID, delegator.NodeID)
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)

	v.PutValidator(staker)

	v.PutDelegator(delegator)
	v.DeleteDelegator(delegator)

	delegatorIterator = v.GetDelegatorIterator(staker.SubnetID, staker.NodeID)
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)
}

func TestDiffStakersValidator(t *testing.T) {
	require := require.New(t)
	staker := newTestStaker()
	delegator := newTestStaker()

	v := diffStakers{}

	v.PutDelegator(delegator)

	_, ok := v.GetValidator(ids.GenerateTestID(), delegator.NodeID)
	require.False(ok)

	_, ok = v.GetValidator(delegator.SubnetID, ids.GenerateTestNodeID())
	require.False(ok)

	_, ok = v.GetValidator(delegator.SubnetID, delegator.NodeID)
	require.False(ok)

	stakerIterator := v.GetStakerIterator(EmptyIterator)
	assertIteratorsEqual(t, NewSliceIterator(delegator), stakerIterator)

	v.PutValidator(staker)

	returnedStaker, ok := v.GetValidator(staker.SubnetID, staker.NodeID)
	require.True(ok)
	require.Equal(staker, returnedStaker)

	v.DeleteValidator(staker)

	returnedStaker, ok = v.GetValidator(staker.SubnetID, staker.NodeID)
	require.True(ok)
	require.Nil(returnedStaker)

	stakerIterator = v.GetStakerIterator(EmptyIterator)
	assertIteratorsEqual(t, NewSliceIterator(delegator), stakerIterator)
}

func TestDiffStakersDelegator(t *testing.T) {
	staker := newTestStaker()
	delegator := newTestStaker()

	v := diffStakers{}

	v.PutValidator(staker)

	delegatorIterator := v.GetDelegatorIterator(EmptyIterator, ids.GenerateTestID(), delegator.NodeID)
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)

	v.PutDelegator(delegator)

	delegatorIterator = v.GetDelegatorIterator(EmptyIterator, delegator.SubnetID, delegator.NodeID)
	assertIteratorsEqual(t, NewSliceIterator(delegator), delegatorIterator)

	v.DeleteDelegator(delegator)

	delegatorIterator = v.GetDelegatorIterator(EmptyIterator, ids.GenerateTestID(), delegator.NodeID)
	assertIteratorsEqual(t, EmptyIterator, delegatorIterator)
}

func newTestStaker() *Staker {
	startTime := time.Now().Round(time.Second)
	endTime := startTime.Add(28 * 24 * time.Hour)
	return &Staker{
		TxID:            ids.GenerateTestID(),
		NodeID:          ids.GenerateTestNodeID(),
		SubnetID:        ids.GenerateTestID(),
		Weight:          1,
		StartTime:       startTime,
		EndTime:         endTime,
		PotentialReward: 1,

		NextTime: endTime,
		Priority: PrimaryNetworkDelegatorCurrentPriority,
	}
}

func assertIteratorsEqual(t *testing.T, expected, actual StakerIterator) {
	t.Helper()

	for expected.Next() {
		require.True(t, actual.Next())

		expectedStaker := expected.Value()
		actualStaker := actual.Value()

		require.Equal(t, expectedStaker, actualStaker)
	}
	require.False(t, actual.Next())

	expected.Release()
	actual.Release()
}
