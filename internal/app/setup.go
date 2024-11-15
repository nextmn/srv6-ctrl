// Copyright 2024 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package app

import (
	"context"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"

	"github.com/nextmn/srv6-ctrl/internal/config"
)

type Setup struct {
	HTTPServer  *HttpServerEntity
	PFCPServer  *pfcp_networking.PFCPEntityUP
	RulesPusher *RulesPusher
}

func NewSetup(conf *config.CtrlConfig) Setup {
	return Setup{
		HTTPServer:  NewHttpServer(conf),
		PFCPServer:  NewPFCPNode(conf),
		RulesPusher: NewRulesPusher(conf),
	}
}

func (s Setup) Run(ctx context.Context) error {
	if err := PFCPServerAddHooks(s.PFCPServer, s.RulesPusher); err != nil {
		return err
	}
	StartPFCPServer(ctx, s.PFCPServer)
	if err := s.HTTPServer.Start(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		s.HTTPServer.Stop()
		s.PFCPServer.Close()
		return nil
	}
}
