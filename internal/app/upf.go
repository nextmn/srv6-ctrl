// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package app

import (
	"context"

	"github.com/nextmn/srv6-ctrl/internal/config"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/message"
)

func NewPFCPNode(conf *config.CtrlConfig) *pfcp_networking.PFCPEntityUP {
	return pfcp_networking.NewPFCPEntityUP(conf.PFCPAddress.String(), conf.PFCPAddress.String())
}
func PFCPServerAddHooks(s *pfcp_networking.PFCPEntityUP, pusher *RulesPusher) error {
	if err := s.AddHandler(message.MsgTypeSessionEstablishmentRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionEstablishmentRequestHandler(ctx, msg)
		if err == nil {
			go s.LogPFCPRules()
			pusher.updateRoutersRules(ctx, message.MsgTypeSessionEstablishmentRequest, msg, s)
		}
		return out, err
	}); err != nil {
		return err
	}
	if err := s.AddHandler(message.MsgTypeSessionModificationRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionModificationRequestHandler(ctx, msg)
		if err == nil {
			go s.LogPFCPRules()
			pusher.updateRoutersRules(ctx, message.MsgTypeSessionModificationRequest, msg, s)
		}
		return out, err
	}); err != nil {
		return err
	}
	return nil
}

func NewHttpServer(conf *config.CtrlConfig, srv6Srv *pfcp_networking.PFCPEntityUP) *HttpServerEntity {
	HTTPServer := NewHttpServerEntity(conf.Control.BindAddr, srv6Srv)
	return HTTPServer
}

func StartPFCPServer(ctx context.Context, srv *pfcp_networking.PFCPEntityUP) {
	go func(ctx context.Context, srv *pfcp_networking.PFCPEntityUP) {
		logrus.Info("Starting PFCP Server")
		if err := srv.ListenAndServeContext(ctx); err != nil {
			logrus.WithError(err).Error("PFCP Server Shutdown")
		}
	}(ctx, srv)
}
