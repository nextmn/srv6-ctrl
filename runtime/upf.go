// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package ctrl

import (
	"fmt"

	pfcp_networking "github.com/louisroyer/go-pfcp-networking/pfcp"
)

var Ctrl *CtrlConfig
var PFCPServer *pfcp_networking.PFCPEntityUP

func Run() error {
	err := createPFCPNode()
	if err != nil {
		return err
	}
	for {
		select {}
	}
	return nil
}

func createPFCPNode() error {
	if Ctrl.PFCPAddress == nil {
		return fmt.Errorf("Missing pfcp address")
	}
	PFCPServer = pfcp_networking.NewPFCPEntityUP(*Ctrl.PFCPAddress)
	PFCPServer.Start()
	return nil
}

func Exit() error {
	return nil
}
