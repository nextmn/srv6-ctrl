// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package json_api

import (
	"fmt"
	"net/netip"
)

type Locator struct {
	netip.Prefix
}

func (l *Locator) UnmarshalText(text []byte) error {
	if err := l.Prefix.UnmarshalText(text); err != nil {
		return err
	}
	if !l.Addr().Is6() {
		return fmt.Errorf("Locator must be an IPv6 prefix")
	}
	return nil
}

func (l Locator) Overlaps(o Locator) bool {
	return l.Prefix.Overlaps(o.Prefix)
}
