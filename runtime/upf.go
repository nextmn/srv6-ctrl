// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package ctrl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcputil "github.com/nextmn/go-pfcp-networking/pfcputil"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"log"
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
	srgw_uri := "http://[fd00:0:0:0:2:8000:0:2]:8080" //FIXME: dont use hardcoded value
	log.Printf("Pushing Router Rule: %s %s %d", ue_ip, gnb_ip, teid_downlink)
	data := map[string]string{
		"ue_ip":         ue_ip,
		"gnb_ip":        gnb_ip,
		"teid_downlink": strconv.FormatUint(uint64(teid_downlink), 10), // FIXME: serialize using a struct to avoid useless conversion
	}
	json_data, err := json.Marshal(data)
	if err != nil {
		fmt.Println(err)
	}
	// TODO: retry on timeout failure
	resp, err := http.Post(srgw_uri+"/rules", "application/json", bytes.NewBuffer(json_data))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		fmt.Printf("HTTP Bad Request\n")
	} else if resp.StatusCode >= 500 {
		fmt.Printf("Router server error: internal error\n")
	}
	//else if resp.StatusCode == 201{
	//OK: store resource
	//_ := resp.Header.Get("Location")
	//}
}

func updateRoutersRules(msgType pfcputil.MessageType, message pfcp_networking.ReceivedMessage, e *pfcp_networking.PFCPEntityUP) {
	log.Printf("Into updateRoutersRules")
	e.PrintPFCPRules()
	for _, session := range e.GetPFCPSessions() {
		log.Printf("In for loop…")
		session.RLock()
		defer session.RUnlock()
		for _, pdrid := range session.GetSortedPDRIDs() {
			pdr, err := session.GetPDR(pdrid)
			if err != nil {
				log.Printf("error getting PDR: %s\n", err)
				continue
			}
			farid, err := pdr.FARID()
			if err != nil {
				log.Printf("error getting FARid: %s\n", err)
				continue
			}
			if source_iface, err := pdr.SourceInterface(); (err != nil) || (source_iface != ie.SrcInterfaceSGiLANN6LAN) {
				log.Printf("sourceiface: %s\n", err)
				continue
			}
			ue_ip_addr, err := pdr.UEIPAddress()
			if err != nil {
				log.Printf("error getting ueipaddr: %s\n", err)
				continue
			}

			// FIXME: temporary hack, no IPv6 support
			ue_ipv4 := ue_ip_addr.IPv4Address.String()

			far, err := session.GetFAR(farid)
			if err != nil {
				log.Printf("error getting far: %s\n", err)
				continue
			}
			ForwardingParametersIe := far.ForwardingParameters()
			if ohc, err := ForwardingParametersIe.OuterHeaderCreation(); err == nil {
				// FIXME: temporary hack, no IPv6 support
				gnb_ipv4 := ohc.IPv4Address.String()
				teid_downlink := ohc.TEID
				log.Printf("PushRTRRule\n")
				go pushRTRRule(ue_ipv4, gnb_ipv4, teid_downlink)
			} else {
				log.Printf("error getting ohc: %s\n", err)
				continue
			}
		}
	}
	log.Printf("Exit updateRoutersRules")
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
