// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/utils/crypto"
	"github.com/kukrer/savannahnode/utils/timer/mockable"
	"github.com/kukrer/savannahnode/vms/components/avax"
	"github.com/kukrer/savannahnode/vms/platformvm/validator"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

func TestAddSubnetValidatorTxSyntacticVerify(t *testing.T) {
	require := require.New(t)
	clk := mockable.Clock{}
	ctx := snow.DefaultContextTest()
	signers := [][]*crypto.PrivateKeySECP256K1R{preFundedKeys}

	var (
		stx                  *Tx
		addSubnetValidatorTx *AddSubnetValidatorTx
		err                  error
	)

	// Case : signed tx is nil
	require.ErrorIs(stx.SyntacticVerify(ctx), errNilSignedTx)

	// Case : unsigned tx is nil
	require.ErrorIs(addSubnetValidatorTx.SyntacticVerify(ctx), ErrNilTx)

	validatorWeight := uint64(2022)
	subnetID := ids.ID{'s', 'u', 'b', 'n', 'e', 't', 'I', 'D'}
	inputs := []*avax.TransferableInput{{
		UTXOID: avax.UTXOID{
			TxID:        ids.ID{'t', 'x', 'I', 'D'},
			OutputIndex: 2,
		},
		Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 't'}},
		In: &secp256k1fx.TransferInput{
			Amt:   uint64(5678),
			Input: secp256k1fx.Input{SigIndices: []uint32{0}},
		},
	}}
	outputs := []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 't'}},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(1234),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
			},
		},
	}}
	subnetAuth := &secp256k1fx.Input{
		SigIndices: []uint32{0, 1},
	}
	addSubnetValidatorTx = &AddSubnetValidatorTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    ctx.NetworkID,
			BlockchainID: ctx.ChainID,
			Ins:          inputs,
			Outs:         outputs,
			Memo:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}},
		Validator: validator.SubnetValidator{
			Validator: validator.Validator{
				NodeID: ctx.NodeID,
				Start:  uint64(clk.Time().Unix()),
				End:    uint64(clk.Time().Add(time.Hour).Unix()),
				Wght:   validatorWeight,
			},
			Subnet: subnetID,
		},
		SubnetAuth: subnetAuth,
	}

	// Case: valid tx
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	require.NoError(stx.SyntacticVerify(ctx))

	// Case: Wrong network ID
	addSubnetValidatorTx.SyntacticallyVerified = false
	addSubnetValidatorTx.NetworkID++
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	err = stx.SyntacticVerify(ctx)
	require.Error(err)
	addSubnetValidatorTx.NetworkID--

	// Case: Missing Subnet ID
	addSubnetValidatorTx.SyntacticallyVerified = false
	addSubnetValidatorTx.Validator.Subnet = ids.Empty
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	err = stx.SyntacticVerify(ctx)
	require.Error(err)
	addSubnetValidatorTx.Validator.Subnet = subnetID

	// Case: No weight
	addSubnetValidatorTx.SyntacticallyVerified = false
	addSubnetValidatorTx.Validator.Wght = 0
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	err = stx.SyntacticVerify(ctx)
	require.Error(err)
	addSubnetValidatorTx.Validator.Wght = validatorWeight

	// Case: Subnet auth indices not unique
	addSubnetValidatorTx.SyntacticallyVerified = false
	input := addSubnetValidatorTx.SubnetAuth.(*secp256k1fx.Input)
	input.SigIndices[0] = input.SigIndices[1]
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	err = stx.SyntacticVerify(ctx)
	require.Error(err)
}

func TestAddSubnetValidatorMarshal(t *testing.T) {
	require := require.New(t)
	clk := mockable.Clock{}
	ctx := snow.DefaultContextTest()
	signers := [][]*crypto.PrivateKeySECP256K1R{preFundedKeys}

	var (
		stx                  *Tx
		addSubnetValidatorTx *AddSubnetValidatorTx
		err                  error
	)

	// create a valid tx
	validatorWeight := uint64(2022)
	subnetID := ids.ID{'s', 'u', 'b', 'n', 'e', 't', 'I', 'D'}
	inputs := []*avax.TransferableInput{{
		UTXOID: avax.UTXOID{
			TxID:        ids.ID{'t', 'x', 'I', 'D'},
			OutputIndex: 2,
		},
		Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 't'}},
		In: &secp256k1fx.TransferInput{
			Amt:   uint64(5678),
			Input: secp256k1fx.Input{SigIndices: []uint32{0}},
		},
	}}
	outputs := []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 't'}},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(1234),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
			},
		},
	}}
	subnetAuth := &secp256k1fx.Input{
		SigIndices: []uint32{0, 1},
	}
	addSubnetValidatorTx = &AddSubnetValidatorTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    ctx.NetworkID,
			BlockchainID: ctx.ChainID,
			Ins:          inputs,
			Outs:         outputs,
			Memo:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}},
		Validator: validator.SubnetValidator{
			Validator: validator.Validator{
				NodeID: ctx.NodeID,
				Start:  uint64(clk.Time().Unix()),
				End:    uint64(clk.Time().Add(time.Hour).Unix()),
				Wght:   validatorWeight,
			},
			Subnet: subnetID,
		},
		SubnetAuth: subnetAuth,
	}

	// Case: valid tx
	stx, err = NewSigned(addSubnetValidatorTx, Codec, signers)
	require.NoError(err)
	require.NoError(stx.SyntacticVerify(ctx))

	txBytes, err := Codec.Marshal(Version, stx)
	require.NoError(err)

	parsedTx, err := Parse(Codec, txBytes)
	require.NoError(err)

	require.NoError(parsedTx.SyntacticVerify(ctx))
	require.Equal(stx, parsedTx)
}
