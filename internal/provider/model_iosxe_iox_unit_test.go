// Copyright © 2023 Cisco Systems, Inc. and its affiliates.
// All rights reserved.
//
// Licensed under the Mozilla Public License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.mozilla.org/MPL/2.0/
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/netascode/xmldot"
)

// These tests exercise the IOx model without a device: they build the NETCONF payload and parse
// canned device replies, so they run with a plain `go test` (no TF_ACC, no .env).

func TestUnitIOxToBodyXML(t *testing.T) {
	tests := []struct {
		name         string
		data         IOx
		wantContains []string
		wantMissing  []string
	}{
		{
			name:         "enable true creates the empty iox container",
			data:         IOx{Enable: types.BoolValue(true)},
			wantContains: []string{"<iox"},
			wantMissing:  []string{"operation=\"remove\""},
		},
		{
			name:         "enable false removes the iox container",
			data:         IOx{Enable: types.BoolValue(false)},
			wantContains: []string{"<iox", "remove"},
		},
		{
			name:        "enable null writes nothing",
			data:        IOx{Enable: types.BoolNull()},
			wantMissing: []string{"<iox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.toBodyXML(context.Background(), tt.data)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("payload missing %q\npayload: %s", want, got)
				}
			}
			for _, bad := range tt.wantMissing {
				if strings.Contains(got, bad) {
					t.Errorf("payload unexpectedly contains %q\npayload: %s", bad, got)
				}
			}
		})
	}
}

// iosxIOxReply parses a body the same way go-netconf does for a real get-config reply
// (xmldot.Get(rawXML, "rpc-reply")), so the model code sees the same shape as in production.
func iosxIOxReply(data string) xmldot.Result {
	return xmldot.Get(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><data>`+data+`</data></rpc-reply>`, "rpc-reply")
}

func TestUnitIOxFromBodyXML(t *testing.T) {
	tests := []struct {
		name       string
		reply      string
		wantEnable bool
	}{
		{
			name:       "iox present and empty reads enable true",
			reply:      `<native xmlns="http://cisco.com/ns/yang/Cisco-IOS-XE-native"><iox/></native>`,
			wantEnable: true,
		},
		{
			name:       "iox absent reads enable false",
			reply:      `<native xmlns="http://cisco.com/ns/yang/Cisco-IOS-XE-native"></native>`,
			wantEnable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := iosxIOxReply(tt.reply)

			var m IOx
			m.fromBodyXML(context.Background(), res)
			if m.Enable.ValueBool() != tt.wantEnable {
				t.Errorf("fromBodyXML Enable = %v, want %v", m.Enable.ValueBool(), tt.wantEnable)
			}

			var d IOxData
			d.fromBodyXML(context.Background(), res)
			if d.Enable.ValueBool() != tt.wantEnable {
				t.Errorf("data source fromBodyXML Enable = %v, want %v", d.Enable.ValueBool(), tt.wantEnable)
			}
		})
	}
}

func TestUnitIOxUpdateFromBodyXMLKeepsNullEnable(t *testing.T) {
	// A config that never set `enable` must not start drifting just because iox exists on the device.
	res := iosxIOxReply(`<native xmlns="http://cisco.com/ns/yang/Cisco-IOS-XE-native"><iox/></native>`)
	m := IOx{Enable: types.BoolNull()}
	m.updateFromBodyXML(context.Background(), res)
	if !m.Enable.IsNull() {
		t.Errorf("updateFromBodyXML with null Enable = %v, want null", m.Enable)
	}

	// A config that set enable = true must read back true when iox exists, false when it does not.
	m = IOx{Enable: types.BoolValue(true)}
	m.updateFromBodyXML(context.Background(), res)
	if !m.Enable.ValueBool() {
		t.Errorf("updateFromBodyXML with iox present = %v, want true", m.Enable)
	}
	m = IOx{Enable: types.BoolValue(true)}
	m.updateFromBodyXML(context.Background(), iosxIOxReply(`<native xmlns="http://cisco.com/ns/yang/Cisco-IOS-XE-native"></native>`))
	if m.Enable.ValueBool() {
		t.Errorf("updateFromBodyXML with iox absent = %v, want false", m.Enable)
	}
}

func TestUnitIOxDeleteSections(t *testing.T) {
	ctx := context.Background()

	// Removing enable from config (state set, plan null) must emit a remove for the container.
	state := IOx{Enable: types.BoolValue(true)}
	plan := IOx{Enable: types.BoolNull()}
	got := plan.addDeletedItemsXML(ctx, state, "")
	if !strings.Contains(got, "iox") || !strings.Contains(got, "remove") {
		t.Errorf("addDeletedItemsXML did not remove iox: %s", got)
	}

	// delete_mode = "attributes" with enable set must remove the container.
	d := IOx{Enable: types.BoolValue(true)}
	got = d.addDeletePathsXML(ctx, "")
	if !strings.Contains(got, "iox") || !strings.Contains(got, "remove") {
		t.Errorf("addDeletePathsXML did not remove iox: %s", got)
	}

	// Nothing set means nothing to delete.
	e := IOx{Enable: types.BoolNull()}
	got = e.addDeletePathsXML(ctx, "")
	if strings.Contains(got, "iox") {
		t.Errorf("addDeletePathsXML with nothing set still mentions iox: %s", got)
	}
}
