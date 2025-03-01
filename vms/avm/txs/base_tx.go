// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"github.com/kukrer/savannahnode/codec"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/vms/components/avax"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

var (
	_ UnsignedTx             = &BaseTx{}
	_ secp256k1fx.UnsignedTx = &BaseTx{}
)

// BaseTx is the basis of all transactions.
type BaseTx struct {
	avax.BaseTx `serialize:"true"`

	bytes []byte
}

func (t *BaseTx) InitCtx(ctx *snow.Context) {
	for _, out := range t.Outs {
		out.InitCtx(ctx)
	}
}

func (t *BaseTx) Initialize(bytes []byte) {
	t.bytes = bytes
}

func (t *BaseTx) Bytes() []byte {
	return t.bytes
}

func (t *BaseTx) SyntacticVerify(
	ctx *snow.Context,
	c codec.Manager,
	txFeeAssetID ids.ID,
	txFee uint64,
	_ uint64,
	_ int,
) error {
	if t == nil {
		return errNilTx
	}

	if err := t.BaseTx.Verify(ctx); err != nil {
		return err
	}

	return avax.VerifyTx(
		txFee,
		txFeeAssetID,
		[][]*avax.TransferableInput{t.Ins},
		[][]*avax.TransferableOutput{t.Outs},
		c,
	)
}

func (t *BaseTx) Visit(v Visitor) error {
	return v.BaseTx(t)
}
