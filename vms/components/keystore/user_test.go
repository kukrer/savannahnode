// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/database/encdb"
	"github.com/kukrer/savannahnode/database/memdb"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/crypto"
)

// Test user password, must meet minimum complexity/length requirements
const testPassword = "ShaggyPassword1Zoinks!"

func TestUserClosedDB(t *testing.T) {
	require := require.New(t)

	db, err := encdb.New([]byte(testPassword), memdb.New())
	require.NoError(err)

	err = db.Close()
	require.NoError(err)

	u := NewUserFromDB(db)

	_, err = u.GetAddresses()
	require.Error(err, "closed db should have caused an error")

	_, err = u.GetKey(ids.ShortEmpty)
	require.Error(err, "closed db should have caused an error")

	_, err = GetKeychain(u, nil)
	require.Error(err, "closed db should have caused an error")

	factory := crypto.FactorySECP256K1R{}
	sk, err := factory.NewPrivateKey()
	require.NoError(err)

	err = u.PutKeys(sk.(*crypto.PrivateKeySECP256K1R))
	require.Error(err, "closed db should have caused an error")
}

func TestUser(t *testing.T) {
	require := require.New(t)

	db, err := encdb.New([]byte(testPassword), memdb.New())
	require.NoError(err)

	u := NewUserFromDB(db)

	addresses, err := u.GetAddresses()
	require.NoError(err)
	require.Empty(addresses, "new user shouldn't have address")

	factory := crypto.FactorySECP256K1R{}
	sk, err := factory.NewPrivateKey()
	require.NoError(err)

	err = u.PutKeys(sk.(*crypto.PrivateKeySECP256K1R))
	require.NoError(err)

	// Putting the same key multiple times should be a noop
	err = u.PutKeys(sk.(*crypto.PrivateKeySECP256K1R))
	require.NoError(err)

	addr := sk.PublicKey().Address()

	savedSk, err := u.GetKey(addr)
	require.NoError(err)
	require.Equal(sk.Bytes(), savedSk.Bytes(), "wrong key returned")

	addresses, err = u.GetAddresses()
	require.NoError(err)
	require.Len(addresses, 1, "address should have been added")

	savedAddr := addresses[0]
	require.Equal(addr, savedAddr, "saved address should match provided address")

	savedKeychain, err := GetKeychain(u, nil)
	require.NoError(err)
	require.Len(savedKeychain.Keys, 1, "key should have been added")
	require.Equal(sk.Bytes(), savedKeychain.Keys[0].Bytes(), "wrong key returned")
}
