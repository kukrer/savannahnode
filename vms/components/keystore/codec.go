// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"math"

	"github.com/kukrer/savannahnode/codec"
	"github.com/kukrer/savannahnode/codec/linearcodec"
	"github.com/kukrer/savannahnode/utils/wrappers"
)

const (
	// CodecVersion is the current default codec version
	CodecVersion = 0
)

// Codecs do serialization and deserialization
var (
	Codec       codec.Manager
	LegacyCodec codec.Manager
)

func init() {
	c := linearcodec.NewDefault()
	Codec = codec.NewDefaultManager()
	lc := linearcodec.NewCustomMaxLength(math.MaxUint32)
	LegacyCodec = codec.NewManager(math.MaxInt32)

	errs := wrappers.Errs{}
	errs.Add(
		Codec.RegisterCodec(CodecVersion, c),
		LegacyCodec.RegisterCodec(CodecVersion, lc),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
}
