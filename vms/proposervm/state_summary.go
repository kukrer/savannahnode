// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package proposervm

import (
	"github.com/kukrer/savannahnode/snow/engine/snowman/block"
	"github.com/kukrer/savannahnode/vms/proposervm/summary"
)

var _ block.StateSummary = &stateSummary{}

// stateSummary implements block.StateSummary by layering three objects:
// 1. [statelessSummary] carries all summary marshallable content along with
//    data immediately retrievable from it.
// 2. [innerSummary] reports the height of the summary as well as notifying the
//    inner vm of the summary's acceptance.
// 3. [block] is used to update the proposervm's last accepted block upon
//    Accept.
//
// Note: summary.StatelessSummary contains the data to build both [innerSummary]
//       and [block].
type stateSummary struct {
	summary.StateSummary

	// inner summary, retrieved via Parse
	innerSummary block.StateSummary

	// block associated with the summary
	block PostForkBlock

	vm *VM
}

func (s *stateSummary) Height() uint64 {
	return s.innerSummary.Height()
}

func (s *stateSummary) Accept() (bool, error) {
	// If we have already synced up to or past this state summary, we do not
	// want to sync to it.
	if s.vm.lastAcceptedHeight >= s.Height() {
		return false, nil
	}

	// set fork height first, before accepting proposerVM full block
	// which updates height index (among other indices)
	if err := s.vm.State.SetForkHeight(s.StateSummary.ForkHeight()); err != nil {
		return false, err
	}

	// We store the full proposerVM block associated with the summary
	// and update height index with it, so that state sync could resume
	// after a shutdown.
	if err := s.block.acceptOuterBlk(); err != nil {
		return false, err
	}

	// innerSummary.Accept may fail with the proposerVM block and index already
	// updated. The error would be treated as fatal and the chain would then be
	// repaired upon the VM restart.
	return s.innerSummary.Accept()
}
