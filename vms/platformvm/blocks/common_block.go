// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocks

import (
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/hashing"
)

// CommonBlock contains fields and methods common to all blocks in this VM.
type CommonBlock struct {
	PrntID ids.ID `serialize:"true" json:"parentID"` // parent's ID
	Hght   uint64 `serialize:"true" json:"height"`   // This block's height. The genesis block is at height 0.

	id    ids.ID
	bytes []byte
}

func (b *CommonBlock) initialize(bytes []byte) {
	b.id = hashing.ComputeHash256Array(bytes)
	b.bytes = bytes
}

func (b *CommonBlock) ID() ids.ID     { return b.id }
func (b *CommonBlock) Parent() ids.ID { return b.PrntID }
func (b *CommonBlock) Bytes() []byte  { return b.bytes }
func (b *CommonBlock) Height() uint64 { return b.Hght }
