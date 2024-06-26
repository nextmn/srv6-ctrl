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
	"net/netip"
	"time"

	"github.com/gin-gonic/gin"
	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcputil "github.com/nextmn/go-pfcp-networking/pfcputil"
	jsonapi "github.com/nextmn/json-api/jsonapi"
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

func isReady(name string, uri string) bool {
	resp, err := http.Get(uri + "/status")
	if err != nil {
		log.Printf("%s is not ready: waiting…\n", name)
		time.Sleep(500 * time.Millisecond)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("%s is not ready: waiting…\n", name)
		time.Sleep(500 * time.Millisecond)
		return false
	} else {
		return true
	}
}

func waitUntilReady(name string, uri string) {
	for {
		if ok := isReady(name, uri); ok {
			return
		}
	}
}

func pushRTRRule(ue_ip string, gnb_ip string, teid_downlink uint32) {
	srgw_uri := "http://[fd00:0:0:0:2:8000:0:2]:8080" //FIXME: dont use hardcoded value
	edgertr0 := "http://[fd00:0:0:0:2:8000:0:4]:8080" //FIXME: dont use hardcoded value
	edgertr1 := "http://[fd00:0:0:0:2:8000:0:5]:8080" //FIXME: dont use hardcoded value
	log.Printf("Pushing Router Rule: %s %s %d", ue_ip, gnb_ip, teid_downlink)

	// FIXME: Temporary hack: wait for routers to be ready
	waitUntilReady("srgw0", srgw_uri)
	waitUntilReady("r0", edgertr0)
	waitUntilReady("r1", edgertr1)

	prefix_ue, err := netip.MustParseAddr(ue_ip).Prefix(32) // FIXME: don't trust input => ParseAddr
	if err != nil {
		log.Printf("Wrong prefix\n")
		return
	}
	// FIXME: don't hardcode!
	srh_downlink := ""
	srh_uplink_1 := "fc00:2:1::" // FIXME
	srh_uplink_2 := "fc00:3:1::" // FIXME
	if teid_downlink != 1 {
		log.Printf("downlink TEID different than hardcoded one! It's time to write more code :(")
		return
	}
	switch gnb_ip {
	case "10.1.4.129": // gnb1
		srh_downlink = "fc00:1:1:0A01:0481:0:0:0100"
		break

	case "10.1.4.130": // gnb2
		srh_downlink = "fc00:1:1:0A01:0482:0:0:0100"
		break
	default:
		log.Printf("Wrong gnb ip : %s\n", gnb_ip)
	}
	nh_downlink, err := jsonapi.NewNextHop(srh_downlink)
	if err != nil {
		log.Printf("err creation of NextHop downlink: %s\n", err)
		return
	}

	nh_uplink1, err := jsonapi.NewNextHop(srh_uplink_1)
	if err != nil {
		log.Printf("err creation of NextHop uplink1: %s\n", err)
		return
	}
	nh_uplink2, err := jsonapi.NewNextHop(srh_uplink_2)
	if err != nil {
		log.Printf("err creation of NextHop uplink2: %s\n", err)
		return
	}
	srh_downlink_json, err := jsonapi.NewSRH([]string{srh_downlink})
	if err != nil {
		log.Printf("err creation of SRH downlink: %s\n", err)
		return
	}
	srh_uplink1_json, err := jsonapi.NewSRH([]string{srh_uplink_1})
	if err != nil {
		log.Printf("err creation of SRH uplink1: %s\n", err)
		return
	}
	srh_uplink2_json, err := jsonapi.NewSRH([]string{srh_uplink_2})
	if err != nil {
		log.Printf("err creation of SRH uplink2: %s\n", err)
		return
	}

	data_edge := jsonapi.Rule{
		Enabled: false,
		Match:   jsonapi.Match{UEIpPrefix: prefix_ue},
		Action: jsonapi.Action{
			NextHop: *nh_downlink,
			SRH:     *srh_downlink_json,
		},
	}

	data_gw1 := jsonapi.Rule{
		Enabled: false,
		Match: jsonapi.Match{
			UEIpPrefix: prefix_ue,
			//	GNBIpPrefix: prefix_gnb, // TODO
		},
		Action: jsonapi.Action{
			NextHop: *nh_uplink1,
			SRH:     *srh_uplink1_json,
		},
	}
	json_data_gw1, err := json.Marshal(data_gw1)
	if err != nil {
		fmt.Println(err)
	}
	data_gw2 := jsonapi.Rule{
		Enabled: false,
		Match: jsonapi.Match{
			UEIpPrefix: prefix_ue,
			//	GNBIpPrefix: prefix_gnb, // TODO
		},
		Action: jsonapi.Action{
			NextHop: *nh_uplink2,
			SRH:     *srh_uplink2_json,
		},
	}
	json_data_gw2, err := json.Marshal(data_gw2)
	if err != nil {
		fmt.Println(err)
	}
	json_data_edge, err := json.Marshal(data_edge)
	if err != nil {
		fmt.Println(err)
	}

	//FIXME: dont send to every node, only to relevant ones

	// TODO: retry on timeout failure
	resp, err := http.Post(srgw_uri+"/rules", "application/json", bytes.NewBuffer(json_data_gw1))
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

	// TODO: retry on timeout failure
	resp, err = http.Post(srgw_uri+"/rules", "application/json", bytes.NewBuffer(json_data_gw2))
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

	// TODO: retry on timeout failure
	resp, err = http.Post(edgertr0+"/rules", "application/json", bytes.NewBuffer(json_data_edge))
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
	// TODO: retry on timeout failure
	resp, err = http.Post(edgertr1+"/rules", "application/json", bytes.NewBuffer(json_data_edge))
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
				log.Printf("skip: error getting PDR: %s\n", err)
				continue
			}
			farid, err := pdr.FARID()
			if err != nil {
				log.Printf("skip: error getting FARid: %s\n", err)
				continue
			}
			if source_iface, err := pdr.SourceInterface(); (err != nil) || (source_iface != ie.SrcInterfaceSGiLANN6LAN) {
				if err == nil {
					log.Println("skip: wrong sourceiface\n")
				} else {
					log.Printf("skip: sourceiface: %s\n", err)
				}
				continue
			}
			ue_ip_addr, err := pdr.UEIPAddress()
			if err != nil {
				log.Printf("skip: error getting ueipaddr: %s\n", err)
				continue
			}

			// FIXME: temporary hack, no IPv6 support
			ue_ipv4 := ue_ip_addr.IPv4Address.String()

			far, err := session.GetFAR(farid)
			if err != nil {
				log.Printf("skip: error getting far: %s\n", err)
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
				log.Printf("skip: error getting ohc: %s\n", err)
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
