// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"errors"
	"fmt"

	"github.com/kukrer/savannahnode/codec"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/utils/crypto"
	"github.com/kukrer/savannahnode/utils/hashing"
	"github.com/kukrer/savannahnode/vms/components/avax"
	"github.com/kukrer/savannahnode/vms/components/verify"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

var (
	errNilSignedTx            = errors.New("nil signed tx is not valid")
	errSignedTxNotInitialized = errors.New("signed tx was never initialized and is not valid")
)

// Tx is a signed transaction
type Tx struct {
	// The body of this transaction
	Unsigned UnsignedTx `serialize:"true" json:"unsignedTx"`

	// The credentials of this transaction
	Creds []verify.Verifiable `serialize:"true" json:"credentials"`

	id    ids.ID
	bytes []byte
}

func NewSigned(
	unsigned UnsignedTx,
	c codec.Manager,
	signers [][]*crypto.PrivateKeySECP256K1R,
) (*Tx, error) {
	res := &Tx{Unsigned: unsigned}
	return res, res.Sign(c, signers)
}

// Parse signed tx starting from its byte representation
func Parse(c codec.Manager, signedBytes []byte) (*Tx, error) {
	tx := &Tx{}
	if _, err := c.Unmarshal(signedBytes, tx); err != nil {
		return nil, fmt.Errorf("couldn't parse tx: %w", err)
	}
	unsignedBytes, err := c.Marshal(Version, &tx.Unsigned)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal UnsignedTx: %w", err)
	}
	tx.Initialize(unsignedBytes, signedBytes)
	return tx, nil
}

func (tx *Tx) Initialize(unsignedBytes, signedBytes []byte) {
	tx.Unsigned.Initialize(unsignedBytes)

	tx.bytes = signedBytes
	tx.id = hashing.ComputeHash256Array(signedBytes)
}

func (tx *Tx) Bytes() []byte { return tx.bytes }
func (tx *Tx) ID() ids.ID    { return tx.id }

// UTXOs returns the UTXOs transaction is producing.
func (tx *Tx) UTXOs() []*avax.UTXO {
	outs := tx.Unsigned.Outputs()
	utxos := make([]*avax.UTXO, len(outs))
	for i, out := range outs {
		utxos[i] = &avax.UTXO{
			UTXOID: avax.UTXOID{
				TxID:        tx.id,
				OutputIndex: uint32(i),
			},
			Asset: avax.Asset{ID: out.AssetID()},
			Out:   out.Out,
		}
	}
	return utxos
}

func (tx *Tx) SyntacticVerify(ctx *snow.Context) error {
	switch {
	case tx == nil:
		return errNilSignedTx
	case tx.id == ids.Empty:
		return errSignedTxNotInitialized
	default:
		return tx.Unsigned.SyntacticVerify(ctx)
	}
}

// Sign this transaction with the provided signers
func (tx *Tx) Sign(c codec.Manager, signers [][]*crypto.PrivateKeySECP256K1R) error {
	unsignedBytes, err := c.Marshal(Version, &tx.Unsigned)
	if err != nil {
		return fmt.Errorf("couldn't marshal UnsignedTx: %w", err)
	}

	// Attach credentials
	hash := hashing.ComputeHash256(unsignedBytes)
	for _, keys := range signers {
		cred := &secp256k1fx.Credential{
			Sigs: make([][crypto.SECP256K1RSigLen]byte, len(keys)),
		}
		for i, key := range keys {
			sig, err := key.SignHash(hash) // Sign hash
			if err != nil {
				return fmt.Errorf("problem generating credential: %w", err)
			}
			copy(cred.Sigs[i][:], sig)
		}
		tx.Creds = append(tx.Creds, cred) // Attach credential
	}

	signedBytes, err := c.Marshal(Version, tx)
	if err != nil {
		return fmt.Errorf("couldn't marshal ProposalTx: %w", err)
	}
	tx.Initialize(unsignedBytes, signedBytes)
	return nil
}
