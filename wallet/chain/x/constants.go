// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package x

import (
	"github.com/kukrer/savannahnode/vms/avm/fxs"
	"github.com/kukrer/savannahnode/vms/avm/txs"
	"github.com/kukrer/savannahnode/vms/nftfx"
	"github.com/kukrer/savannahnode/vms/propertyfx"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

const (
	SECP256K1FxIndex = 0
	NFTFxIndex       = 1
	PropertyFxIndex  = 2
)

// Parser to support serialization and deserialization
var Parser txs.Parser

func init() {
	var err error
	Parser, err = txs.NewParser([]fxs.Fx{
		&secp256k1fx.Fx{},
		&nftfx.Fx{},
		&propertyfx.Fx{},
	})
	if err != nil {
		panic(err)
	}
}
