// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/api"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/json"
	"github.com/kukrer/savannahnode/utils/rpc"
)

type mockClient struct {
	require        *require.Assertions
	expectedInData interface{}
}

func (mc *mockClient) SendRequest(
	ctx context.Context,
	method string,
	inData interface{},
	reply interface{},
	options ...rpc.Option,
) error {
	mc.require.Equal(inData, mc.expectedInData)
	return nil
}

func TestClientCreateAsset(t *testing.T) {
	require := require.New(t)
	client := client{}
	{
		// empty slices
		clientHolders := []*ClientHolder{}
		clientMinters := []ClientOwners{}
		clientFrom := []ids.ShortID{}
		clientChangeAddr := ids.GenerateTestShortID()
		serviceHolders := []*Holder{}
		serviceMinters := []Owners{}
		serviceFrom := []string{}
		serviceChangeAddr := clientChangeAddr.String()
		expectedInData := &CreateAssetArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				JSONFromAddrs:  api.JSONFromAddrs{From: serviceFrom},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: serviceChangeAddr},
			},
			InitialHolders: serviceHolders,
			MinterSets:     serviceMinters,
		}
		client.requester = &mockClient{
			require:        require,
			expectedInData: expectedInData,
		}
		_, err := client.CreateAsset(
			context.Background(),
			api.UserPass{},
			clientFrom,
			clientChangeAddr,
			"",
			"",
			0,
			clientHolders,
			clientMinters,
		)
		require.NoError(err)
	}
	{
		// non empty slices
		clientHolders := []*ClientHolder{
			{
				Amount:  11,
				Address: ids.GenerateTestShortID(),
			},
		}
		clientMinters := []ClientOwners{
			{
				Threshold: 22,
				Minters:   []ids.ShortID{ids.GenerateTestShortID()},
			},
		}
		clientFrom := []ids.ShortID{ids.GenerateTestShortID()}
		clientChangeAddr := ids.GenerateTestShortID()
		serviceHolders := []*Holder{
			{
				Amount:  json.Uint64(clientHolders[0].Amount),
				Address: clientHolders[0].Address.String(),
			},
		}
		serviceMinters := []Owners{
			{
				Threshold: json.Uint32(clientMinters[0].Threshold),
				Minters:   []string{clientMinters[0].Minters[0].String()},
			},
		}
		serviceFrom := []string{clientFrom[0].String()}
		serviceChangeAddr := clientChangeAddr.String()
		expectedInData := &CreateAssetArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				JSONFromAddrs:  api.JSONFromAddrs{From: serviceFrom},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: serviceChangeAddr},
			},
			InitialHolders: serviceHolders,
			MinterSets:     serviceMinters,
		}
		client.requester = &mockClient{
			require:        require,
			expectedInData: expectedInData,
		}
		_, err := client.CreateAsset(
			context.Background(),
			api.UserPass{},
			clientFrom,
			clientChangeAddr,
			"",
			"",
			0,
			clientHolders,
			clientMinters,
		)
		require.NoError(err)
	}
}

func TestClientCreateFixedCapAsset(t *testing.T) {
	require := require.New(t)
	client := client{}
	{
		// empty slices
		clientHolders := []*ClientHolder{}
		clientFrom := []ids.ShortID{}
		clientChangeAddr := ids.GenerateTestShortID()
		serviceHolders := []*Holder{}
		serviceFrom := []string{}
		serviceChangeAddr := clientChangeAddr.String()
		expectedInData := &CreateAssetArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				JSONFromAddrs:  api.JSONFromAddrs{From: serviceFrom},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: serviceChangeAddr},
			},
			InitialHolders: serviceHolders,
		}
		client.requester = &mockClient{
			require:        require,
			expectedInData: expectedInData,
		}
		_, err := client.CreateFixedCapAsset(
			context.Background(),
			api.UserPass{},
			clientFrom,
			clientChangeAddr,
			"",
			"",
			0,
			clientHolders,
		)
		require.NoError(err)
	}
	{
		// non empty slices
		clientHolders := []*ClientHolder{
			{
				Amount:  11,
				Address: ids.GenerateTestShortID(),
			},
		}
		clientFrom := []ids.ShortID{ids.GenerateTestShortID()}
		clientChangeAddr := ids.GenerateTestShortID()
		serviceHolders := []*Holder{
			{
				Amount:  json.Uint64(clientHolders[0].Amount),
				Address: clientHolders[0].Address.String(),
			},
		}
		serviceFrom := []string{clientFrom[0].String()}
		serviceChangeAddr := clientChangeAddr.String()
		expectedInData := &CreateAssetArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				JSONFromAddrs:  api.JSONFromAddrs{From: serviceFrom},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: serviceChangeAddr},
			},
			InitialHolders: serviceHolders,
		}
		client.requester = &mockClient{
			require:        require,
			expectedInData: expectedInData,
		}
		_, err := client.CreateFixedCapAsset(
			context.Background(),
			api.UserPass{},
			clientFrom,
			clientChangeAddr,
			"",
			"",
			0,
			clientHolders,
		)
		require.NoError(err)
	}
}
