// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"errors"
	"unicode"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/utils/constants"
	"github.com/kukrer/savannahnode/utils/units"
	"github.com/kukrer/savannahnode/vms/components/verify"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

const (
	MaxNameLen    = 128
	MaxGenesisLen = units.MiB
)

var (
	_ UnsignedTx             = &CreateChainTx{}
	_ secp256k1fx.UnsignedTx = &CreateChainTx{}

	ErrCantValidatePrimaryNetwork = errors.New("new blockchain can't be validated by primary network")

	errInvalidVMID             = errors.New("invalid VM ID")
	errFxIDsNotSortedAndUnique = errors.New("feature extensions IDs must be sorted and unique")
	errNameTooLong             = errors.New("name too long")
	errGenesisTooLong          = errors.New("genesis too long")
	errIllegalNameCharacter    = errors.New("illegal name character")
)

// CreateChainTx is an unsigned createChainTx
type CreateChainTx struct {
	// Metadata, inputs and outputs
	BaseTx `serialize:"true"`
	// ID of the Subnet that validates this blockchain
	SubnetID ids.ID `serialize:"true" json:"subnetID"`
	// A human readable name for the chain; need not be unique
	ChainName string `serialize:"true" json:"chainName"`
	// ID of the VM running on the new chain
	VMID ids.ID `serialize:"true" json:"vmID"`
	// IDs of the feature extensions running on the new chain
	FxIDs []ids.ID `serialize:"true" json:"fxIDs"`
	// Byte representation of genesis state of the new chain
	GenesisData []byte `serialize:"true" json:"genesisData"`
	// Authorizes this blockchain to be added to this subnet
	SubnetAuth verify.Verifiable `serialize:"true" json:"subnetAuthorization"`
}

func (tx *CreateChainTx) SyntacticVerify(ctx *snow.Context) error {
	switch {
	case tx == nil:
		return ErrNilTx
	case tx.SyntacticallyVerified: // already passed syntactic verification
		return nil
	case tx.SubnetID == constants.PrimaryNetworkID:
		return ErrCantValidatePrimaryNetwork
	case len(tx.ChainName) > MaxNameLen:
		return errNameTooLong
	case tx.VMID == ids.Empty:
		return errInvalidVMID
	case !ids.IsSortedAndUniqueIDs(tx.FxIDs):
		return errFxIDsNotSortedAndUnique
	case len(tx.GenesisData) > MaxGenesisLen:
		return errGenesisTooLong
	}

	for _, r := range tx.ChainName {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ') {
			return errIllegalNameCharacter
		}
	}

	if err := tx.BaseTx.SyntacticVerify(ctx); err != nil {
		return err
	}
	if err := tx.SubnetAuth.Verify(); err != nil {
		return err
	}

	tx.SyntacticallyVerified = true
	return nil
}

func (tx *CreateChainTx) Visit(visitor Visitor) error {
	return visitor.CreateChainTx(tx)
}
