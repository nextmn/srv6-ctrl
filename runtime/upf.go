// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package ctrl

import (
	"fmt"

	"github.com/gin-gonic/gin"
	pfcp_networking "github.com/nextmn/go-pfcp-networking/pfcp"
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

func createPFCPNode() error {
	if Ctrl.PFCPAddress == nil {
		return fmt.Errorf("Missing pfcp address")
	}
	PFCPServer = pfcp_networking.NewPFCPEntityUP(*Ctrl.PFCPAddress)
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
