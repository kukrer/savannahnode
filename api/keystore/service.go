// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"fmt"
	"net/http"

	"github.com/kukrer/savannahnode/api"
	"github.com/kukrer/savannahnode/database/manager"
	"github.com/kukrer/savannahnode/database/memdb"
	"github.com/kukrer/savannahnode/utils/formatting"
	"github.com/kukrer/savannahnode/utils/logging"
	"github.com/kukrer/savannahnode/version"
)

type service struct {
	ks *keystore
}

func (s *service) CreateUser(_ *http.Request, args *api.UserPass, _ *api.EmptyReply) error {
	s.ks.log.Debug("Keystore: CreateUser called",
		logging.UserString("username", args.Username),
	)

	return s.ks.CreateUser(args.Username, args.Password)
}

func (s *service) DeleteUser(_ *http.Request, args *api.UserPass, _ *api.EmptyReply) error {
	s.ks.log.Debug("Keystore: DeleteUser called",
		logging.UserString("username", args.Username),
	)

	return s.ks.DeleteUser(args.Username, args.Password)
}

type ListUsersReply struct {
	Users []string `json:"users"`
}

func (s *service) ListUsers(_ *http.Request, args *struct{}, reply *ListUsersReply) error {
	s.ks.log.Debug("Keystore: ListUsers called")

	var err error
	reply.Users, err = s.ks.ListUsers()
	return err
}

type ImportUserArgs struct {
	// The username and password of the user being imported
	api.UserPass
	// The string representation of the user
	User string `json:"user"`
	// The encoding of [User] ("hex")
	Encoding formatting.Encoding `json:"encoding"`
}

func (s *service) ImportUser(r *http.Request, args *ImportUserArgs, _ *api.EmptyReply) error {
	s.ks.log.Debug("Keystore: ImportUser called",
		logging.UserString("username", args.Username),
	)

	// Decode the user from string to bytes
	user, err := formatting.Decode(args.Encoding, args.User)
	if err != nil {
		return fmt.Errorf("couldn't decode 'user' to bytes: %w", err)
	}

	return s.ks.ImportUser(args.Username, args.Password, user)
}

type ExportUserArgs struct {
	// The username and password
	api.UserPass
	// The encoding for the exported user ("hex")
	Encoding formatting.Encoding `json:"encoding"`
}

type ExportUserReply struct {
	// String representation of the user
	User string `json:"user"`
	// The encoding for the exported user ("hex")
	Encoding formatting.Encoding `json:"encoding"`
}

func (s *service) ExportUser(_ *http.Request, args *ExportUserArgs, reply *ExportUserReply) error {
	s.ks.log.Debug("Keystore: ExportUser called",
		logging.UserString("username", args.Username),
	)

	userBytes, err := s.ks.ExportUser(args.Username, args.Password)
	if err != nil {
		return err
	}

	// Encode the user from bytes to string
	reply.User, err = formatting.Encode(args.Encoding, userBytes)
	if err != nil {
		return fmt.Errorf("couldn't encode user to string: %w", err)
	}
	reply.Encoding = args.Encoding
	return nil
}

// CreateTestKeystore returns a new keystore that can be utilized for testing
func CreateTestKeystore() (Keystore, error) {
	dbManager, err := manager.NewManagerFromDBs([]*manager.VersionedDatabase{
		{
			Database: memdb.New(),
			Version:  version.Semantic1_0_0,
		},
	})
	if err != nil {
		return nil, err
	}
	return New(logging.NoLog{}, dbManager), nil
}
