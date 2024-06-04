// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package ctrl

import (
	"fmt"

	"github.com/gin-gonic/gin"
	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcputil "github.com/nextmn/go-pfcp-networking/pfcputil"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

var Ctrl *CtrlConfig
var PFCPServer *pfcp_networking.PFCPEntityUP
var HTTPServer *HttpServerEntity

func Run() error {
	// setup
	if Ctrl.Debug != nil && *Ctrl.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	// pfcp
	if err := createPFCPNode(); err != nil {
		return err
	}
	// http
	if err := createHttpServer(); err != nil {
		return err
	}
	for {
		select {}
	}
	return nil
}

func pushRTRRule(ue_ip string, gnb_ip string, teid_downlink uint32) {
}

func updateRoutersRules(msgType pfcputil.MessageType, message pfcp_networking.ReceivedMessage, e *pfcp_networking.PFCPEntityUP) {
	for _, session := range e.GetPFCPSessions() {
		session.RLock()
		defer session.RUnlock()
		for _, pdrid := range session.GetSortedPDRIDs() {
			pdr, err := session.GetPDR(pdrid)
			if err != nil {
				continue
			}
			farid, err := pdr.FARID()
			if err != nil {
				continue
			}
			if source_iface, err := pdr.SourceInterface(); (err != nil) || (source_iface != ie.SrcInterfaceCore) {
				continue
			}
			ue_ip_addr, err := pdr.UEIPAddress()
			if err != nil {
				continue
			}

			// FIXME: temporary hack, no IPv6 support
			ue_ipv4 := ue_ip_addr.IPv4Address.String()

			far, err := session.GetFAR(farid)
			if err != nil {
				continue
			}
			ForwardingParametersIe := far.ForwardingParameters()
			if ohc, err := ForwardingParametersIe.OuterHeaderCreation(); err == nil {
				// FIXME: temporary hack, no IPv6 support
				gnb_ipv4 := ohc.IPv4Address.String()
				teid_downlink := ohc.TEID
				go pushRTRRule(ue_ipv4, gnb_ipv4, teid_downlink)
			} else {
				continue
			}
		}
	}
}

func createPFCPNode() error {
	if Ctrl.PFCPAddress == nil {
		return fmt.Errorf("Missing pfcp address")
	}
	PFCPServer = pfcp_networking.NewPFCPEntityUP(*Ctrl.PFCPAddress)
	PFCPServer.AddHandler(message.MsgTypeSessionEstablishmentRequest, func(msg pfcp_networking.ReceivedMessage) error {
		err := pfcp_networking.DefaultSessionEstablishmentRequestHandler(msg)
		go updateRoutersRules(message.MsgTypeSessionEstablishmentRequest, msg, PFCPServer)
		return err
	})
	PFCPServer.AddHandler(message.MsgTypeSessionModificationRequest, func(msg pfcp_networking.ReceivedMessage) error {
		err := pfcp_networking.DefaultSessionModificationRequestHandler(msg)
		go updateRoutersRules(message.MsgTypeSessionModificationRequest, msg, PFCPServer)
		return err
	})

	PFCPServer.Start()
	return nil
}

func createHttpServer() error {
	if Ctrl.HTTPAddress == nil {
		return fmt.Errorf("Missing http address")
	}
	port := "80" // default http port
	if Ctrl.HTTPPort != nil {
		port = *Ctrl.HTTPPort
	}
	HTTPServer = NewHttpServerEntity(*Ctrl.HTTPAddress, port)
	HTTPServer.Start()
	return nil
}

func Exit() error {
	// TODO: stop pfcp & http
	return nil
}
