// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package x

import (
	"errors"
	"fmt"

	stdcontext "context"

	"github.com/kukrer/savannahnode/database"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/crypto"
	"github.com/kukrer/savannahnode/utils/hashing"
	"github.com/kukrer/savannahnode/vms/avm/fxs"
	"github.com/kukrer/savannahnode/vms/avm/txs"
	"github.com/kukrer/savannahnode/vms/components/avax"
	"github.com/kukrer/savannahnode/vms/components/verify"
	"github.com/kukrer/savannahnode/vms/nftfx"
	"github.com/kukrer/savannahnode/vms/propertyfx"
	"github.com/kukrer/savannahnode/vms/secp256k1fx"
)

var (
	errUnknownTxType         = errors.New("unknown tx type")
	errUnknownInputType      = errors.New("unknown input type")
	errUnknownOpType         = errors.New("unknown operation type")
	errInvalidNumUTXOsInOp   = errors.New("invalid number of UTXOs in operation")
	errUnknownCredentialType = errors.New("unknown credential type")
	errUnknownOutputType     = errors.New("unknown output type")
	errInvalidUTXOSigIndex   = errors.New("invalid UTXO signature index")

	emptySig [crypto.SECP256K1RSigLen]byte

	_ Signer = &signer{}
)

type Signer interface {
	SignUnsigned(ctx stdcontext.Context, tx txs.UnsignedTx) (*txs.Tx, error)
	Sign(ctx stdcontext.Context, tx *txs.Tx) error
}

type SignerBackend interface {
	GetUTXO(ctx stdcontext.Context, chainID, utxoID ids.ID) (*avax.UTXO, error)
}

type signer struct {
	kc      *secp256k1fx.Keychain
	backend SignerBackend
}

func NewSigner(kc *secp256k1fx.Keychain, backend SignerBackend) Signer {
	return &signer{
		kc:      kc,
		backend: backend,
	}
}

func (s *signer) SignUnsigned(ctx stdcontext.Context, utx txs.UnsignedTx) (*txs.Tx, error) {
	tx := &txs.Tx{Unsigned: utx}
	return tx, s.Sign(ctx, tx)
}

// TODO: implement txs.Visitor here
func (s *signer) Sign(ctx stdcontext.Context, tx *txs.Tx) error {
	switch utx := tx.Unsigned.(type) {
	case *txs.BaseTx:
		return s.signBaseTx(ctx, tx, utx)
	case *txs.CreateAssetTx:
		return s.signCreateAssetTx(ctx, tx, utx)
	case *txs.OperationTx:
		return s.signOperationTx(ctx, tx, utx)
	case *txs.ImportTx:
		return s.signImportTx(ctx, tx, utx)
	case *txs.ExportTx:
		return s.signExportTx(ctx, tx, utx)
	default:
		return fmt.Errorf("%w: %T", errUnknownTxType, tx.Unsigned)
	}
}

func (s *signer) signBaseTx(ctx stdcontext.Context, tx *txs.Tx, utx *txs.BaseTx) error {
	txCreds, txSigners, err := s.getSigners(ctx, utx.BlockchainID, utx.Ins)
	if err != nil {
		return err
	}
	return s.sign(tx, txCreds, txSigners)
}

func (s *signer) signCreateAssetTx(ctx stdcontext.Context, tx *txs.Tx, utx *txs.CreateAssetTx) error {
	txCreds, txSigners, err := s.getSigners(ctx, utx.BlockchainID, utx.Ins)
	if err != nil {
		return err
	}
	return s.sign(tx, txCreds, txSigners)
}

func (s *signer) signOperationTx(ctx stdcontext.Context, tx *txs.Tx, utx *txs.OperationTx) error {
	txCreds, txSigners, err := s.getSigners(ctx, utx.BlockchainID, utx.Ins)
	if err != nil {
		return err
	}
	txOpsCreds, txOpsSigners, err := s.getOpsSigners(ctx, utx.BlockchainID, utx.Ops)
	if err != nil {
		return err
	}
	txCreds = append(txCreds, txOpsCreds...)
	txSigners = append(txSigners, txOpsSigners...)
	return s.sign(tx, txCreds, txSigners)
}

func (s *signer) signImportTx(ctx stdcontext.Context, tx *txs.Tx, utx *txs.ImportTx) error {
	txCreds, txSigners, err := s.getSigners(ctx, utx.BlockchainID, utx.Ins)
	if err != nil {
		return err
	}
	txImportCreds, txImportSigners, err := s.getSigners(ctx, utx.SourceChain, utx.ImportedIns)
	if err != nil {
		return err
	}
	txCreds = append(txCreds, txImportCreds...)
	txSigners = append(txSigners, txImportSigners...)
	return s.sign(tx, txCreds, txSigners)
}

func (s *signer) signExportTx(ctx stdcontext.Context, tx *txs.Tx, utx *txs.ExportTx) error {
	txCreds, txSigners, err := s.getSigners(ctx, utx.BlockchainID, utx.Ins)
	if err != nil {
		return err
	}
	return s.sign(tx, txCreds, txSigners)
}

func (s *signer) getSigners(ctx stdcontext.Context, sourceChainID ids.ID, ins []*avax.TransferableInput) ([]verify.Verifiable, [][]*crypto.PrivateKeySECP256K1R, error) {
	txCreds := make([]verify.Verifiable, len(ins))
	txSigners := make([][]*crypto.PrivateKeySECP256K1R, len(ins))
	for credIndex, transferInput := range ins {
		txCreds[credIndex] = &secp256k1fx.Credential{}
		input, ok := transferInput.In.(*secp256k1fx.TransferInput)
		if !ok {
			return nil, nil, errUnknownInputType
		}

		inputSigners := make([]*crypto.PrivateKeySECP256K1R, len(input.SigIndices))
		txSigners[credIndex] = inputSigners

		utxoID := transferInput.InputID()
		utxo, err := s.backend.GetUTXO(ctx, sourceChainID, utxoID)
		if err == database.ErrNotFound {
			// If we don't have access to the UTXO, then we can't sign this
			// transaction. However, we can attempt to partially sign it.
			continue
		}
		if err != nil {
			return nil, nil, err
		}

		out, ok := utxo.Out.(*secp256k1fx.TransferOutput)
		if !ok {
			return nil, nil, errUnknownOutputType
		}

		for sigIndex, addrIndex := range input.SigIndices {
			if addrIndex >= uint32(len(out.Addrs)) {
				return nil, nil, errInvalidUTXOSigIndex
			}

			addr := out.Addrs[addrIndex]
			key, ok := s.kc.Get(addr)
			if !ok {
				// If we don't have access to the key, then we can't sign this
				// transaction. However, we can attempt to partially sign it.
				continue
			}
			inputSigners[sigIndex] = key
		}
	}
	return txCreds, txSigners, nil
}

func (s *signer) getOpsSigners(ctx stdcontext.Context, sourceChainID ids.ID, ops []*txs.Operation) ([]verify.Verifiable, [][]*crypto.PrivateKeySECP256K1R, error) {
	txCreds := make([]verify.Verifiable, len(ops))
	txSigners := make([][]*crypto.PrivateKeySECP256K1R, len(ops))
	for credIndex, op := range ops {
		var input *secp256k1fx.Input
		switch op := op.Op.(type) {
		case *secp256k1fx.MintOperation:
			txCreds[credIndex] = &secp256k1fx.Credential{}
			input = &op.MintInput
		case *nftfx.MintOperation:
			txCreds[credIndex] = &nftfx.Credential{}
			input = &op.MintInput
		case *nftfx.TransferOperation:
			txCreds[credIndex] = &nftfx.Credential{}
			input = &op.Input
		case *propertyfx.MintOperation:
			txCreds[credIndex] = &propertyfx.Credential{}
			input = &op.MintInput
		case *propertyfx.BurnOperation:
			txCreds[credIndex] = &propertyfx.Credential{}
			input = &op.Input
		default:
			return nil, nil, errUnknownOpType
		}

		inputSigners := make([]*crypto.PrivateKeySECP256K1R, len(input.SigIndices))
		txSigners[credIndex] = inputSigners

		if len(op.UTXOIDs) != 1 {
			return nil, nil, errInvalidNumUTXOsInOp
		}
		utxoID := op.UTXOIDs[0].InputID()
		utxo, err := s.backend.GetUTXO(ctx, sourceChainID, utxoID)
		if err == database.ErrNotFound {
			// If we don't have access to the UTXO, then we can't sign this
			// transaction. However, we can attempt to partially sign it.
			continue
		}
		if err != nil {
			return nil, nil, err
		}

		var addrs []ids.ShortID
		switch out := utxo.Out.(type) {
		case *secp256k1fx.MintOutput:
			addrs = out.Addrs
		case *nftfx.MintOutput:
			addrs = out.Addrs
		case *nftfx.TransferOutput:
			addrs = out.Addrs
		case *propertyfx.MintOutput:
			addrs = out.Addrs
		case *propertyfx.OwnedOutput:
			addrs = out.Addrs
		default:
			return nil, nil, errUnknownOutputType
		}

		for sigIndex, addrIndex := range input.SigIndices {
			if addrIndex >= uint32(len(addrs)) {
				return nil, nil, errInvalidUTXOSigIndex
			}

			addr := addrs[addrIndex]
			key, ok := s.kc.Get(addr)
			if !ok {
				// If we don't have access to the key, then we can't sign this
				// transaction. However, we can attempt to partially sign it.
				continue
			}
			inputSigners[sigIndex] = key
		}
	}
	return txCreds, txSigners, nil
}

func (s *signer) sign(tx *txs.Tx, creds []verify.Verifiable, txSigners [][]*crypto.PrivateKeySECP256K1R) error {
	codec := Parser.Codec()
	unsignedBytes, err := codec.Marshal(txs.CodecVersion, &tx.Unsigned)
	if err != nil {
		return fmt.Errorf("couldn't marshal unsigned tx: %w", err)
	}
	unsignedHash := hashing.ComputeHash256(unsignedBytes)

	if expectedLen := len(txSigners); expectedLen != len(tx.Creds) {
		tx.Creds = make([]*fxs.FxCredential, expectedLen)
	}

	sigCache := make(map[ids.ShortID][crypto.SECP256K1RSigLen]byte)
	for credIndex, inputSigners := range txSigners {
		fxCred := tx.Creds[credIndex]
		if fxCred == nil {
			fxCred = &fxs.FxCredential{}
			tx.Creds[credIndex] = fxCred
		}
		credIntf := fxCred.Verifiable
		if credIntf == nil {
			credIntf = creds[credIndex]
			fxCred.Verifiable = credIntf
		}

		var cred *secp256k1fx.Credential
		switch credImpl := credIntf.(type) {
		case *secp256k1fx.Credential:
			cred = credImpl
		case *nftfx.Credential:
			cred = &credImpl.Credential
		case *propertyfx.Credential:
			cred = &credImpl.Credential
		default:
			return errUnknownCredentialType
		}

		if expectedLen := len(inputSigners); expectedLen != len(cred.Sigs) {
			cred.Sigs = make([][crypto.SECP256K1RSigLen]byte, expectedLen)
		}

		for sigIndex, signer := range inputSigners {
			if signer == nil {
				// If we don't have access to the key, then we can't sign this
				// transaction. However, we can attempt to partially sign it.
				continue
			}
			addr := signer.PublicKey().Address()
			if sig := cred.Sigs[sigIndex]; sig != emptySig {
				// If this signature has already been populated, we can just
				// copy the needed signature for the future.
				sigCache[addr] = sig
				continue
			}

			if sig, exists := sigCache[addr]; exists {
				// If this key has already produced a signature, we can just
				// copy the previous signature.
				cred.Sigs[sigIndex] = sig
				continue
			}

			sig, err := signer.SignHash(unsignedHash)
			if err != nil {
				return fmt.Errorf("problem signing tx: %w", err)
			}
			copy(cred.Sigs[sigIndex][:], sig)
			sigCache[addr] = cred.Sigs[sigIndex]
		}
	}

	signedBytes, err := codec.Marshal(txs.CodecVersion, tx)
	if err != nil {
		return fmt.Errorf("couldn't marshal tx: %w", err)
	}
	tx.Initialize(unsignedBytes, signedBytes)
	return nil
}
