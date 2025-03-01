// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beacon

import (
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/ips"
)

var _ Beacon = &beacon{}

type Beacon interface {
	ID() ids.NodeID
	IP() ips.IPPort
}

type beacon struct {
	id ids.NodeID
	ip ips.IPPort
}

func New(id ids.NodeID, ip ips.IPPort) Beacon {
	return &beacon{
		id: id,
		ip: ip,
	}
}

func (b *beacon) ID() ids.NodeID { return b.id }
func (b *beacon) IP() ips.IPPort { return b.ip }
