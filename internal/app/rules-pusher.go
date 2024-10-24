// Copyright 2024 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
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

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"

	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"
	"github.com/nextmn/go-pfcp-networking/pfcputil"
	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/rfc9433/encoding"
	"github.com/nextmn/srv6-ctrl/internal/config"
)

const UserAgent = "go-github-nextmn-srv6-ctrl"

type RulesPusher struct {
	uplink   []config.Rule
	downlink []config.Rule
	ues      sync.Map
}

type ueInfos struct {
	UplinkTeid   uint32
	DownlinkTeid uint32
	Gnb          string
	Pushed       bool
	sync.Mutex
}

func NewRulesPusher(config *config.CtrlConfig) *RulesPusher {
	return &RulesPusher{
		uplink:   config.Uplink,
		downlink: config.Downlink,
		ues:      sync.Map{},
	}
}

func (pusher *RulesPusher) pushSingleRule(client http.Client, uri string, data []byte) error {
	req, err := http.NewRequest(http.MethodPost, uri+"/rules", bytes.NewBuffer(data))
	if err != nil {
		logrus.WithError(err).Error("could not create http request")
		return err
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		logrus.WithError(err).Error("Could not push rules: server not responding")
		return fmt.Errorf("Could not push rules: server not responding")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		logrus.WithError(err).Error("HTTP Bad Request")
		return fmt.Errorf("HTTP Bad request")
	} else if resp.StatusCode >= 500 {
		logrus.WithError(err).Error("HTTP internal error")
		return fmt.Errorf("HTTP internal error")
	}
	//else if resp.StatusCode == 201{
	//OK: store resource
	//_ := resp.Header.Get("Location")
	//}
	return nil
}

func (pusher *RulesPusher) pushRTRRule(ue_ip string) error {
	i, ok := pusher.ues.Load(ue_ip)
	infos := i.(*ueInfos)
	infos.Lock()
	defer infos.Unlock()
	if infos.Pushed {
		return nil // already pushed, nothing to do
	}
	infos.Pushed = true
	if !ok {
		return fmt.Errorf("UE not in ue list")
	}
	service_ip := "10.4.0.1"
	logrus.WithFields(logrus.Fields{
		"ue-ip":         ue_ip,
		"gnb-ip":        infos.Gnb,
		"teid-downlink": infos.DownlinkTeid,
		"teid-uplink":   infos.UplinkTeid,
		"service-ip":    service_ip,
	}).Info("Pushing Router Rules")
	ue_addr := netip.MustParseAddr(ue_ip)           // FIXME: don't trust user input => ParseAddr
	gnb_addr := netip.MustParseAddr(infos.Gnb)      // FIXME: don't trust user input => ParseAddr
	service_addr := netip.MustParseAddr(service_ip) // FIXME: don't trust user input => ParseAddr

	client := http.Client{}
	var wg sync.WaitGroup

	for _, r := range pusher.uplink {
		//TODO: add ArgMobSession
		srh, err := jsonapi.NewSRH(r.SegmentsList)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"segments-list": r.SegmentsList,
			}).WithError(err).Error("Creation of SRH uplink failed")
			return err
		}
		rule := jsonapi.Rule{
			Enabled: r.Enabled,
			Type:    "uplink",
			Match: jsonapi.Match{
				Header: &jsonapi.GtpHeader{
					OuterIpSrc: gnb_addr,
					Teid:       infos.UplinkTeid,
					InnerIpSrc: &ue_addr,
				},
				Payload: &jsonapi.Payload{
					Dst: service_addr,
				},
			},
			Action: jsonapi.Action{
				SRH: *srh,
			},
		}
		rule_json, err := json.Marshal(rule)
		if err != nil {
			logrus.WithError(err).Error("Could not marshal json")
			return err
		}
		wg.Add(1)
		go func() error {
			defer wg.Done()
			return pusher.pushSingleRule(client, r.ControlURI, rule_json)
		}()

	}

	for _, r := range pusher.downlink {
		if len(r.SegmentsList) == 0 {
			logrus.Error("Empty segments list for downlink")
			return fmt.Errorf("Empty segments list for downlink")
		}
		segList := make([]string, len(r.SegmentsList))
		copy(segList, r.SegmentsList)
		prefix, err := netip.ParsePrefix(r.SegmentsList[0])
		if err != nil {
			return err
		}
		dst := encoding.NewMGTP4IPv6Dst(prefix, gnb_addr.As4(), encoding.NewArgsMobSession(0, false, false, infos.DownlinkTeid))
		dstB, err := dst.Marshal()
		if err != nil {
			return err
		}
		dstIp, ok := netip.AddrFromSlice(dstB)
		if !ok {
			return fmt.Errorf("could not convert MGTP4IPv6Dst to netip.Addr")
		}
		segList[0] = dstIp.String()

		srh, err := jsonapi.NewSRH(segList)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"segments-list": r.SegmentsList,
			}).WithError(err).Error("Creation of SRH downlink failed")
			return err
		}
		rule := jsonapi.Rule{
			Enabled: true,
			Type:    "downlink",
			Match: jsonapi.Match{
				Payload: &jsonapi.Payload{
					Dst: ue_addr,
				},
			},
			Action: jsonapi.Action{
				SRH: *srh,
			},
		}
		rule_json, err := json.Marshal(rule)
		if err != nil {
			logrus.WithError(err).Error("Could not marshal json")
			return err
		}
		wg.Add(1)
		go func() error {
			defer wg.Done()
			return pusher.pushSingleRule(client, r.ControlURI, rule_json)
		}()

	}
	wg.Wait()
	return nil
}

func (pusher *RulesPusher) updateRoutersRules(ctx context.Context, msgType pfcputil.MessageType, message pfcp_networking.ReceivedMessage, e *pfcp_networking.PFCPEntityUP) {
	logrus.Debug("Into updateRoutersRules")
	var wg0 sync.WaitGroup
	for _, session := range e.GetPFCPSessions() {
		logrus.Debug("In for loop…")
		wg0.Add(1)
		go func() error {
			defer wg0.Done()
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
					if ue, loaded := pusher.ues.LoadOrStore(ue_ipv4, &ueInfos{
						UplinkTeid: fteid.TEID,
					}); loaded {
						logrus.WithFields(logrus.Fields{
							"teid-uplink": fteid.TEID,
							"ue-ipv4":     ue_ipv4,
						}).Debug("Updating UeInfos")

						ue.(*ueInfos).Lock()
						ue.(*ueInfos).UplinkTeid = fteid.TEID
						ue.(*ueInfos).Unlock()
					} else if logrus.IsLevelEnabled(logrus.DebugLevel) {
						logrus.WithFields(logrus.Fields{
							"teid-uplink": fteid.TEID,
							"ue-ipv4":     ue_ipv4,
						}).Debug("Adding new ue to UeInfos")
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
						if ue, loaded := pusher.ues.LoadOrStore(ue_ipv4, &ueInfos{
							DownlinkTeid: teid_downlink,
							Gnb:          gnb_ipv4,
						}); loaded {
							logrus.WithFields(logrus.Fields{
								"gnb-ipv4":      gnb_ipv4,
								"teid-downlink": teid_downlink,
								"ue-ipv4":       ue_ipv4,
							}).Debug("Updating UeInfos")
							ue.(*ueInfos).Lock()
							ue.(*ueInfos).Gnb = gnb_ipv4
							ue.(*ueInfos).DownlinkTeid = teid_downlink
							ue.(*ueInfos).Unlock()
						} else if logrus.IsLevelEnabled(logrus.DebugLevel) {
							logrus.WithFields(logrus.Fields{
								"gnb-ipv4":      gnb_ipv4,
								"teid-downlink": teid_downlink,
								"ue-ipv4":       ue_ipv4,
							}).Debug("Adding new ue to UeInfos")
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
	wg0.Wait()
	var wg sync.WaitGroup
	pusher.ues.Range(func(ip any, ue any) bool {
		if ue.(*ueInfos).DownlinkTeid == 0 {
			// no set yet => session will be modified
			logrus.WithFields(logrus.Fields{
				"ue-ipv4": ip,
			}).Debug("Downlink TEID is null")
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
			pusher.pushRTRRule(ip.(string))
			// TODO: check pushRTRRule return code and send pfcp error on failure
		}()
		return true
	})
	wg.Wait()
	logrus.Debug("Exit updateRoutersRules")
}
