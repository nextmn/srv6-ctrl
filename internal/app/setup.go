// Copyright 2024 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package app

import (
	"context"
	"os"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"

	"github.com/nextmn/srv6-ctrl/internal/config"
)

type Setup struct {
	HTTPServer *HttpServerEntity
	PFCPServer *pfcp_networking.PFCPEntityUP
}

func NewSetup(conf *config.CtrlConfig) Setup {
	return Setup{
		HTTPServer: NewHttpServer(conf),
		PFCPServer: NewPFCPNode(conf),
	}
}

func (s Setup) Run(ctx context.Context) error {
	s.PFCPServer.Start()
	s.HTTPServer.Start()
	select {
	case <-ctx.Done():
		s.HTTPServer.Stop()
		// TODO: stop pfcp node
		os.Exit(0) // currently, we will force exit instead
		return nil
	}
}