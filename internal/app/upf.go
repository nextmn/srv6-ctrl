// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/nextmn/srv6-ctrl/internal/config"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	"github.com/nextmn/go-pfcp-networking/pfcputil"
	"github.com/nextmn/json-api/jsonapi"

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

const UserAgent = "go-github-nextmn-srv6-ctrl"

func isReady(name string, uri string) bool {
	client := http.Client{}
	req, err := http.NewRequest(http.MethodGet, uri+"/status", nil)
	if err != nil {
		logrus.WithError(err).Error("Error while creating http get request")
		return false
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Charset", "utf-8")
	resp, err := client.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{"remote-server": name}).WithError(err).Info("Remote server is not ready: waiting…")
		time.Sleep(500 * time.Millisecond)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		logrus.WithFields(logrus.Fields{"remote-server": name}).WithError(err).Info("Remote server is not ready: waiting…")
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
	logrus.WithFields(logrus.Fields{
		"ue-ip":         ue_ip,
		"gnb-ip":        gnb_ip,
		"teid-downlink": teid_downlink,
	}).Info("Pushing Router Rule")

	// FIXME: Temporary hack: wait for routers to be ready
	waitUntilReady("srgw0", srgw_uri)
	waitUntilReady("r0", edgertr0)
	waitUntilReady("r1", edgertr1)

	prefix_ue, err := netip.MustParseAddr(ue_ip).Prefix(32) // FIXME: don't trust input => ParseAddr
	if err != nil {
		logrus.WithFields(logrus.Fields{"ue-ip": ue_ip}).WithError(err).Error("Wrong prefix for UE")
		return
	}
	prefix_gnb, err := netip.MustParseAddr(gnb_ip).Prefix(32) // FIXME: don't trust user input => ParseAddr
	if err != nil {
		logrus.WithFields(logrus.Fields{"gnb-ip": gnb_ip}).WithError(err).Error("Wrong prefix for Gnb")
		return
	}

	// FIXME: don't hardcode!
	srh_downlink := ""
	srh_uplink_1 := "fc00:2:1::" // FIXME
	srh_uplink_2 := "fc00:3:1::" // FIXME
	rr := "fc00:4:1::"           //FIXME
	if teid_downlink != 1 {
		logrus.WithFields(logrus.Fields{
			"hardcoded-teid-downlink": 1,
			"actual-teid-downlink":    teid_downlink,
		}).Error("downlink TEID different than hardcoded one! It's time to write more code :(")
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
		logrus.WithFields(logrus.Fields{"gnb-ip": gnb_ip}).Error("Wrong gnb ip")
	}
	nh_downlink, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop downlink failed")
		return
	}

	nh_uplink1, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop uplink 1 failed")
		return
	}
	nh_uplink2, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop uplink 2 failed")
		return
	}
	srh_downlink_json, err := jsonapi.NewSRH([]string{srh_downlink})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH downlink failed")
		return
	}
	srh_uplink1_json, err := jsonapi.NewSRH([]string{srh_uplink_1})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH uplink 1 failed")
		return
	}
	srh_uplink2_json, err := jsonapi.NewSRH([]string{srh_uplink_2})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH uplink 2 failed")
		return
	}

	data_edge := jsonapi.Rule{
		Enabled: true,
		Type:    "downlink",
		Match:   jsonapi.Match{UEIpPrefix: prefix_ue},
		Action: jsonapi.Action{
			NextHop: *nh_downlink,
			SRH:     *srh_downlink_json,
		},
	}

	data_gw1 := jsonapi.Rule{
		Enabled: true,
		Type:    "uplink",
		Match: jsonapi.Match{
			UEIpPrefix:  prefix_ue,
			GNBIpPrefix: prefix_gnb, // TODO
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
		Type:    "uplink",
		Match: jsonapi.Match{
			UEIpPrefix:  prefix_ue,
			GNBIpPrefix: prefix_gnb, // TODO
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
	client := http.Client{}

	// TODO: retry on timeout failure
	req, err := http.NewRequest(http.MethodPost, srgw_uri+"/rules", bytes.NewBuffer(json_data_gw1))
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := client.Do(req)
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
	req, err = http.NewRequest(http.MethodPost, srgw_uri+"/rules", bytes.NewBuffer(json_data_gw2))
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err = client.Do(req)
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
	req, err = http.NewRequest(http.MethodPost, edgertr0+"/rules", bytes.NewBuffer(json_data_edge))
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err = client.Do(req)
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
	req, err = http.NewRequest(http.MethodPost, edgertr1+"/rules", bytes.NewBuffer(json_data_edge))
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err = client.Do(req)
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
	logrus.Debug("Into updateRoutersRules")
	e.LogPFCPRules()
	for _, session := range e.GetPFCPSessions() {
		logrus.Debug("In for loop…")
		session.RLock()
		defer session.RUnlock()
		for _, pdrid := range session.GetSortedPDRIDs() {
			pdr, err := session.GetPDR(pdrid)
			if err != nil {
				logrus.WithError(err).Debug("skip: error getting PDR")
				continue
			}
			farid, err := pdr.FARID()
			if err != nil {
				logrus.WithError(err).Debug("skip: error getting FARid")
				continue
			}
			if source_iface, err := pdr.SourceInterface(); (err != nil) || ((source_iface != ie.SrcInterfaceSGiLANN6LAN) && (source_iface != ie.SrcInterfaceCore)) {
				if err == nil {
					logrus.WithFields(logrus.Fields{"source-iface": source_iface}).Debug("skip: wrong source-iface")
				} else {
					logrus.WithError(err).Debug("skip: error getting source-iface")
				}
				continue
			}
			ue_ip_addr, err := pdr.UEIPAddress()
			if err != nil {
				logrus.WithError(err).Debug("skip: error getting ueipaddr")
				continue
			}

			// FIXME: temporary hack, no IPv6 support
			ue_ipv4 := ue_ip_addr.IPv4Address.String()

			far, err := session.GetFAR(farid)
			if err != nil {
				logrus.WithError(err).Debug("skip: error getting far")
				continue
			}
			ForwardingParametersIe := far.ForwardingParameters()
			if ohc, err := ForwardingParametersIe.OuterHeaderCreation(); err == nil {
				// FIXME: temporary hack, no IPv6 support
				gnb_ipv4 := ohc.IPv4Address.String()
				teid_downlink := ohc.TEID
				logrus.WithFields(logrus.Fields{
					"ue-ipv4":       ue_ipv4,
					"gnb-ipv4":      gnb_ipv4,
					"teid-downlink": teid_downlink,
				}).Debug("PushRTRRule")
				go pushRTRRule(ue_ipv4, gnb_ipv4, teid_downlink)
			} else {
				logrus.WithError(err).Debug("skip: error getting ohc")
				continue
			}
		}
	}
	logrus.Debug("Exit updateRoutersRules")
}

func NewPFCPNode(conf *config.CtrlConfig) *pfcp_networking.PFCPEntityUP {
	PFCPServer := pfcp_networking.NewPFCPEntityUP(conf.PFCPAddress, conf.PFCPAddress)
	PFCPServer.AddHandler(message.MsgTypeSessionEstablishmentRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionEstablishmentRequestHandler(ctx, msg)
		go updateRoutersRules(message.MsgTypeSessionEstablishmentRequest, msg, PFCPServer)
		return out, err
	})
	PFCPServer.AddHandler(message.MsgTypeSessionModificationRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionModificationRequestHandler(ctx, msg)
		go updateRoutersRules(message.MsgTypeSessionModificationRequest, msg, PFCPServer)
		return out, err
	})
	return PFCPServer
}

func NewHttpServer(conf *config.CtrlConfig) *HttpServerEntity {
	port := "80" // default http port
	if conf.HTTPPort != nil {
		port = *conf.HTTPPort
	}
	HTTPServer := NewHttpServerEntity(conf.HTTPAddress, port)
	return HTTPServer
}

func StartPFCPServer(ctx context.Context, srv *pfcp_networking.PFCPEntityUP) {
	go func() {
		logrus.Info("Starting PFCP Server")
		if err := srv.ListenAndServeContext(ctx); err != nil {
			logrus.WithError(err).Error("PFCP Server Shutdown")
		}
	}()
}
