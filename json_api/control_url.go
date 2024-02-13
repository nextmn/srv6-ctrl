// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package json_api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type ControlURI struct {
	url.URL
}

func (u *ControlURI) UnmarshalText(text []byte) error {
	if text[len(text)-1] == '/' {
		return fmt.Errorf("Control URI should not contains trailing slash.")
	}
	if a, err := url.ParseRequestURI(string(text[:])); err != nil {
		return err
	} else {
		u.URL = *a
	}
	return nil
}
func (u ControlURI) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}
