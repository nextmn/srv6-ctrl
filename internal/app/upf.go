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
	"sync"

	"github.com/nextmn/srv6-ctrl/internal/config"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"
	"github.com/nextmn/go-pfcp-networking/pfcputil"
	"github.com/nextmn/json-api/jsonapi"

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

const UserAgent = "go-github-nextmn-srv6-ctrl"

func pushSingleRule(client http.Client, uri string, data []byte) error {
	req, err := http.NewRequest(http.MethodPost, uri+"/rules", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println(err)
		return err
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("Could not push rules: server not responding")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		fmt.Printf("HTTP Bad Request\n")
		return fmt.Errorf("HTTP Bad request")
	} else if resp.StatusCode >= 500 {
		fmt.Printf("Router server error: internal error\n")
		return fmt.Errorf("HTTP internal error")
	}
	//else if resp.StatusCode == 201{
	//OK: store resource
	//_ := resp.Header.Get("Location")
	//}
	return nil
}

func pushRTRRule(ue_ip string, gnb_ip string, teid_downlink uint32, teid_uplink uint32) error {
	srgw_uri := "http://[fd00:0:0:0:2:8000:0:2]:8080" //FIXME: dont use hardcoded value
	edgertr0 := "http://[fd00:0:0:0:2:8000:0:4]:8080" //FIXME: dont use hardcoded value
	edgertr1 := "http://[fd00:0:0:0:2:8000:0:5]:8080" //FIXME: dont use hardcoded value
	logrus.WithFields(logrus.Fields{
		"ue-ip":         ue_ip,
		"gnb-ip":        gnb_ip,
		"teid-downlink": teid_downlink,
		"teid-uplink":   teid_uplink,
	}).Info("Pushing Router Rule")

	ue_addr := netip.MustParseAddr(ue_ip)           // FIXME: don't trust input => ParseAddr
	gnb_addr := netip.MustParseAddr(gnb_ip)         // FIXME: don't trust user input => ParseAddr
	service_addr := netip.MustParseAddr("10.4.0.1") // FIXME: don't trust user input => ParseAddr

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
		return fmt.Errorf("Not implemented with this teid")
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
		return fmt.Errorf("Not implemented with this gnb ip")
	}
	nh_downlink, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop downlink failed")
		return err
	}

	nh_uplink1, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop uplink 1 failed")
		return err
	}
	nh_uplink2, err := jsonapi.NewNextHop(rr)
	if err != nil {
		logrus.WithError(err).Error("Creation of NextHop uplink 2 failed")
		return err
	}
	srh_downlink_json, err := jsonapi.NewSRH([]string{srh_downlink})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH downlink failed")
		return err
	}
	srh_uplink1_json, err := jsonapi.NewSRH([]string{srh_uplink_1})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH uplink 1 failed")
		return err
	}
	srh_uplink2_json, err := jsonapi.NewSRH([]string{srh_uplink_2})
	if err != nil {
		logrus.WithError(err).Error("Creation of SRH uplink 2 failed")
		return err
	}

	data_edge := jsonapi.Rule{
		Enabled: true,
		Type:    "downlink",
		Match: jsonapi.Match{
			Payload: &jsonapi.Payload{
				Dst: ue_addr,
			},
		},
		Action: jsonapi.Action{
			NextHop: *nh_downlink,
			SRH:     *srh_downlink_json,
		},
	}

	data_gw1 := jsonapi.Rule{
		Enabled: true,
		Type:    "uplink",
		Match: jsonapi.Match{
			Header: &jsonapi.GtpHeader{
				OuterIpSrc: gnb_addr, // TODO
				Teid:       teid_uplink,
				InnerIpSrc: &ue_addr,
			},
			Payload: &jsonapi.Payload{
				Dst: service_addr,
			},
		},
		Action: jsonapi.Action{
			NextHop: *nh_uplink1,
			SRH:     *srh_uplink1_json,
		},
	}
	json_data_gw1, err := json.Marshal(data_gw1)
	if err != nil {
		fmt.Println(err)
		return err
	}
	data_gw2 := jsonapi.Rule{
		Enabled: false,
		Type:    "uplink",
		Match: jsonapi.Match{
			Header: &jsonapi.GtpHeader{
				OuterIpSrc: gnb_addr, // TODO
				Teid:       teid_uplink,
				InnerIpSrc: &ue_addr,
			},
			Payload: &jsonapi.Payload{
				Dst: service_addr,
			},
		},
		Action: jsonapi.Action{
			NextHop: *nh_uplink2,
			SRH:     *srh_uplink2_json,
		},
	}
	json_data_gw2, err := json.Marshal(data_gw2)
	if err != nil {
		fmt.Println(err)
		return err
	}
	json_data_edge, err := json.Marshal(data_edge)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//FIXME: dont send to every node, only to relevant ones
	client := http.Client{}
	var wg sync.WaitGroup

	wg.Add(1)
	go func() error {
		defer wg.Done()
		return pushSingleRule(client, srgw_uri, json_data_gw1)
	}()

	wg.Add(1)
	go func() error {
		defer wg.Done()
		return pushSingleRule(client, srgw_uri, json_data_gw2)
	}()

	wg.Add(1)
	go func() error {
		defer wg.Done()
		return pushSingleRule(client, edgertr0, json_data_edge)
	}()

	wg.Add(1)
	go func() error {
		defer wg.Done()
		return pushSingleRule(client, edgertr1, json_data_edge)
	}()
	wg.Wait()
	return nil
}

type ueInfos struct {
	UplinkTeid   uint32
	DownlinkTeid uint32
	Gnb          string
}

func updateRoutersRules(ctx context.Context, msgType pfcputil.MessageType, message pfcp_networking.ReceivedMessage, e *pfcp_networking.PFCPEntityUP) {
	logrus.Debug("Into updateRoutersRules")
	ues := sync.Map{}
	for _, session := range e.GetPFCPSessions() {
		logrus.Debug("In for loop…")
		go func() error {
			session.RLock()
			defer session.RUnlock()
			session.ForeachUnsortedPDR(func(pdr pfcpapi.PDRInterface) error {
				farid, err := pdr.FARID()
				if err != nil {
					logrus.WithError(err).Debug("skip: error getting FARid")
					return nil
				}
				ue_ip_addr, err := pdr.UEIPAddress()
				if err != nil {
					logrus.WithError(err).Debug("skip: error getting ueipaddr")
					return nil
				}

				// FIXME: temporary hack, no IPv6 support
				ue_ipv4 := ue_ip_addr.IPv4Address.String()
				if source_iface, err := pdr.SourceInterface(); err != nil {
					logrus.WithError(err).Debug("skip: error getting source-iface")
					return nil
				} else if source_iface == ie.SrcInterfaceAccess {
					fteid, err := pdr.FTEID()
					if err != nil {
						logrus.WithError(err).Debug("skip: no fteid")
						return nil
					}
					if ue, loaded := ues.LoadOrStore(ue_ipv4, &ueInfos{
						UplinkTeid: fteid.TEID,
					}); loaded {
						ue.(*ueInfos).UplinkTeid = fteid.TEID
					}

				} else if (source_iface == ie.SrcInterfaceCore) || (source_iface == ie.SrcInterfaceSGiLANN6LAN) {
					far, err := session.GetFAR(farid)
					if err != nil {
						logrus.WithError(err).Debug("skip: error getting far")
						return nil
					}
					ForwardingParametersIe, err := far.ForwardingParameters()
					if err != nil {
						// no forwarding prameters (maybe because hasn't FORW ?)
						return nil
					}
					if ohc, err := ForwardingParametersIe.OuterHeaderCreation(); err == nil {
						// FIXME: temporary hack, no IPv6 support
						gnb_ipv4 := ohc.IPv4Address.String()
						teid_downlink := ohc.TEID
						if ue, loaded := ues.LoadOrStore(ue_ipv4, &ueInfos{
							DownlinkTeid: teid_downlink,
							Gnb:          gnb_ipv4,
						}); loaded {
							ue.(*ueInfos).Gnb = gnb_ipv4
							ue.(*ueInfos).DownlinkTeid = teid_downlink
						}

					} else {
						logrus.WithError(err).Debug("skip: error getting ohc")
						return nil
					}
				} else {
					return nil
				}
				return nil
			})
			return nil
		}()
	}
	var wg sync.WaitGroup
	ues.Range(func(ip any, ue any) bool {
		if ue.(*ueInfos).DownlinkTeid == 0 {
			// no set yet => session will be modified
			return true
		}
		logrus.WithFields(logrus.Fields{
			"ue-ipv4":       ip,
			"gnb-ipv4":      ue.(*ueInfos).Gnb,
			"teid-downlink": ue.(*ueInfos).DownlinkTeid,
			"teid-uplink":   ue.(*ueInfos).UplinkTeid,
		}).Debug("PushRTRRule")
		wg.Add(1)
		go func() {
			defer wg.Done()
			pushRTRRule(ip.(string), ue.(*ueInfos).Gnb, ue.(*ueInfos).DownlinkTeid, ue.(*ueInfos).UplinkTeid)
			// TODO: check pushRTRRule return code and send pfcp error on failure
		}()
		return true
	})
	wg.Wait()
	logrus.Debug("Exit updateRoutersRules")
}

func NewPFCPNode(conf *config.CtrlConfig) *pfcp_networking.PFCPEntityUP {
	return pfcp_networking.NewPFCPEntityUP(conf.PFCPAddress.String(), conf.PFCPAddress.String())
}
func PFCPServerAddHooks(s *pfcp_networking.PFCPEntityUP) error {
	if err := s.AddHandler(message.MsgTypeSessionEstablishmentRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionEstablishmentRequestHandler(ctx, msg)
		if err == nil {
			go s.LogPFCPRules()
			updateRoutersRules(ctx, message.MsgTypeSessionEstablishmentRequest, msg, s)
		}
		return out, err
	}); err != nil {
		return err
	}
	if err := s.AddHandler(message.MsgTypeSessionModificationRequest, func(ctx context.Context, msg pfcp_networking.ReceivedMessage) (*pfcp_networking.OutcomingMessage, error) {
		out, err := pfcp_networking.DefaultSessionModificationRequestHandler(ctx, msg)
		if err == nil {
			go s.LogPFCPRules()
			updateRoutersRules(ctx, message.MsgTypeSessionModificationRequest, msg, s)
		}
		return out, err
	}); err != nil {
		return err
	}
	return nil
}

func NewHttpServer(conf *config.CtrlConfig) *HttpServerEntity {
	port := "80" // default http port
	if conf.HTTPPort != nil {
		port = *conf.HTTPPort
	}
	HTTPServer := NewHttpServerEntity(conf.HTTPAddress.String(), port)
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
