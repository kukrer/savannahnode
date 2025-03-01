// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package greader

import (
	"context"
	"errors"
	"io"

	readerpb "github.com/kukrer/savannahnode/proto/pb/io/reader"
)

var _ io.Reader = &Client{}

// Client is a reader that talks over RPC.
type Client struct{ client readerpb.ReaderClient }

// NewClient returns a reader connected to a remote reader
func NewClient(client readerpb.ReaderClient) *Client {
	return &Client{client: client}
}

func (c *Client) Read(p []byte) (int, error) {
	resp, err := c.client.Read(context.Background(), &readerpb.ReadRequest{
		Length: int32(len(p)),
	})
	if err != nil {
		return 0, err
	}

	copy(p, resp.Read)

	if resp.Errored {
		err = errors.New(resp.Error)
	}
	return len(resp.Read), err
}
