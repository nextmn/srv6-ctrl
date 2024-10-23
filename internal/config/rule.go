// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package config

type Rule struct {
	ControlURI   string   `yaml:"control-uri"` // e.g. http://srgw.local:8080
	Enabled      bool     `yaml:"enabled"`
	SegmentsList []string `yaml:"segments-list"` // Segment[0] is the ultimate node, Segment[n-1] is the next hop ; Segment[0] can be a prefix (for downlink)
}
