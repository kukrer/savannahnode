// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"fmt"
	"testing"

	"github.com/kukrer/savannahnode/ids"
	"github.com/golang/mock/gomock"
)

func TestSendMixedQuery(t *testing.T) {
	type test struct {
		senderF   func() *MockSender
		vdrs      []ids.NodeID
		numPushTo int
	}
	reqID := uint32(1337)
	containerID := ids.GenerateTestID()
	containerBytes := []byte{'y', 'e', 'e', 't'}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	vdr1, vdr2, vdr3 := ids.GenerateTestNodeID(), ids.GenerateTestNodeID(), ids.GenerateTestNodeID()
	tests := []test{
		{
			senderF: func() *MockSender {
				s := NewMockSender(ctrl)
				s.EXPECT().SendPushQuery(
					ids.NodeIDSet{vdr1: struct{}{}, vdr2: struct{}{}, vdr3: struct{}{}},
					reqID,
					containerID,
					containerBytes,
				).Times(1)
				s.EXPECT().SendPullQuery(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Times(0)
				return s
			},
			vdrs:      []ids.NodeID{vdr1, vdr2, vdr3},
			numPushTo: 3,
		},
		{
			senderF: func() *MockSender {
				s := NewMockSender(ctrl)
				s.EXPECT().SendPushQuery(
					ids.NodeIDSet{vdr1: struct{}{}},
					reqID,
					containerID,
					containerBytes,
				).Times(1)
				s.EXPECT().SendPullQuery(
					ids.NodeIDSet{vdr2: struct{}{}, vdr3: struct{}{}},
					reqID,
					containerID,
				).Times(1)
				return s
			},
			vdrs:      []ids.NodeID{vdr1, vdr2, vdr3},
			numPushTo: 1,
		},
		{
			senderF: func() *MockSender {
				s := NewMockSender(ctrl)
				s.EXPECT().SendPushQuery(
					ids.NodeIDSet{vdr1: struct{}{}, vdr2: struct{}{}},
					reqID,
					containerID,
					containerBytes,
				).Times(1)
				s.EXPECT().SendPullQuery(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Times(0)
				return s
			},
			vdrs:      []ids.NodeID{vdr1, vdr2},
			numPushTo: 2,
		},
		{
			senderF: func() *MockSender {
				s := NewMockSender(ctrl)
				s.EXPECT().SendPushQuery(
					gomock.Any(),
					reqID,
					containerID,
					containerBytes,
				).Times(0)
				s.EXPECT().SendPullQuery(
					ids.NodeIDSet{vdr1: struct{}{}},
					reqID,
					containerID,
				).Times(1)
				return s
			},
			vdrs:      []ids.NodeID{vdr1},
			numPushTo: 0,
		},
		{
			senderF: func() *MockSender {
				s := NewMockSender(ctrl)
				s.EXPECT().SendPushQuery(
					ids.NodeIDSet{vdr1: struct{}{}, vdr2: struct{}{}},
					reqID,
					containerID,
					containerBytes,
				).Times(1)
				s.EXPECT().SendPullQuery(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Times(0)
				return s
			},
			vdrs:      []ids.NodeID{vdr1, vdr2},
			numPushTo: 4,
		},
	}

	for _, tt := range tests {
		t.Run(
			fmt.Sprintf("numPushTo: %d, numVdrs: %d", tt.numPushTo, len(tt.vdrs)),
			func(t *testing.T) {
				sender := tt.senderF()
				SendMixedQuery(
					sender,
					tt.vdrs,
					tt.numPushTo,
					reqID,
					containerID,
					containerBytes,
				)
			},
		)
	}
}
