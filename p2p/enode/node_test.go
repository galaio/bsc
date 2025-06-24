// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package enode

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
	"testing"
	"testing/quick"

	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pyRecord, _ = hex.DecodeString("f884b8407098ad865b00a582051940cb9cf36836572411a47278783077011599ed5cd16b76f2635f4e234738f30813a89eb9137e3e3df5266e3a1f11df72ecf1145ccb9c01826964827634826970847f00000189736563703235366b31a103ca634cae0d49acb401d8a4c6b6fe8c55b70d115bf400769cc1400f3258cd31388375647082765f")

// TestPythonInterop checks that we can decode and verify a record produced by the Python
// implementation.
func TestPythonInterop(t *testing.T) {
	var r enr.Record
	if err := rlp.DecodeBytes(pyRecord, &r); err != nil {
		t.Fatalf("can't decode: %v", err)
	}
	n, err := New(ValidSchemes, &r)
	if err != nil {
		t.Fatalf("can't verify record: %v", err)
	}

	var (
		wantID  = HexID("a448f24c6d18e575453db13171562b71999873db5b286df957af199ec94617f7")
		wantSeq = uint64(1)
		wantIP  = enr.IPv4{127, 0, 0, 1}
		wantUDP = enr.UDP(30303)
	)
	if n.Seq() != wantSeq {
		t.Errorf("wrong seq: got %d, want %d", n.Seq(), wantSeq)
	}
	if n.ID() != wantID {
		t.Errorf("wrong id: got %x, want %x", n.ID(), wantID)
	}
	want := map[enr.Entry]interface{}{new(enr.IPv4): &wantIP, new(enr.UDP): &wantUDP}
	for k, v := range want {
		desc := fmt.Sprintf("loading key %q", k.ENRKey())
		if assert.NoError(t, n.Load(k), desc) {
			assert.Equal(t, k, v, desc)
		}
	}
}

func TestNodeEndpoints(t *testing.T) {
	id := HexID("00000000000000806ad9b61fa5ae014307ebdc964253adcd9f2c0a392aa11abc")
	type endpointTest struct {
		name     string
		node     *Node
		wantIP   netip.Addr
		wantUDP  int
		wantTCP  int
		wantQUIC int
		wantDNS  string
	}
	tests := []endpointTest{
		{
			name: "no-addr",
			node: func() *Node {
				var r enr.Record
				return SignNull(&r, id)
			}(),
		},
		{
			name: "udp-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.UDP(9000))
				return SignNull(&r, id)
			}(),
			wantUDP: 9000,
		},
		{
			name: "tcp-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.TCP(9000))
				return SignNull(&r, id)
			}(),
			wantTCP: 9000,
		},
		{
			name: "quic-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.QUIC(9000))
				return SignNull(&r, id)
			}(),
		},
		{
			name: "quic6-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.QUIC6(9000))
				return SignNull(&r, id)
			}(),
		},
		{
			name: "ipv4-only-loopback",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("127.0.0.1")))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("127.0.0.1"),
		},
		{
			name: "ipv4-only-unspecified",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("0.0.0.0")))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("0.0.0.0"),
		},
		{
			name: "ipv4-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("99.22.33.1")))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("99.22.33.1"),
		},
		{
			name: "ipv6-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("2001::ff00:0042:8329"),
		},
		{
			name: "ipv4-loopback-and-ipv6-global",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("127.0.0.1")))
				r.Set(enr.UDP(30304))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				r.Set(enr.UDP6(30306))
				return SignNull(&r, id)
			}(),
			wantIP:  netip.MustParseAddr("2001::ff00:0042:8329"),
			wantUDP: 30306,
		},
		{
			name: "ipv4-unspecified-and-ipv6-loopback",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("0.0.0.0")))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("::1")))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("::1"),
		},
		{
			name: "ipv4-private-and-ipv6-global",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("192.168.2.2")))
				r.Set(enr.UDP(30304))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				r.Set(enr.UDP6(30306))
				return SignNull(&r, id)
			}(),
			wantIP:  netip.MustParseAddr("2001::ff00:0042:8329"),
			wantUDP: 30306,
		},
		{
			name: "ipv4-local-and-ipv6-global",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("169.254.2.6")))
				r.Set(enr.UDP(30304))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				r.Set(enr.UDP6(30306))
				return SignNull(&r, id)
			}(),
			wantIP:  netip.MustParseAddr("2001::ff00:0042:8329"),
			wantUDP: 30306,
		},
		{
			name: "ipv4-private-and-ipv6-private",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("192.168.2.2")))
				r.Set(enr.UDP(30304))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("fd00::abcd:1")))
				r.Set(enr.UDP6(30306))
				return SignNull(&r, id)
			}(),
			wantIP:  netip.MustParseAddr("192.168.2.2"),
			wantUDP: 30304,
		},
		{
			name: "ipv4-private-and-ipv6-link-local",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("192.168.2.2")))
				r.Set(enr.UDP(30304))
				r.Set(enr.IPv6Addr(netip.MustParseAddr("fe80::1")))
				r.Set(enr.UDP6(30306))
				return SignNull(&r, id)
			}(),
			wantIP:  netip.MustParseAddr("192.168.2.2"),
			wantUDP: 30304,
		},
		{
			name: "ipv4-quic",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("99.22.33.1")))
				r.Set(enr.QUIC(9001))
				return SignNull(&r, id)
			}(),
			wantIP:   netip.MustParseAddr("99.22.33.1"),
			wantQUIC: 9001,
		},
		{ // Because the node is IPv4, the quic6 entry won't be loaded.
			name: "ipv4-quic6",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv4Addr(netip.MustParseAddr("99.22.33.1")))
				r.Set(enr.QUIC6(9001))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("99.22.33.1"),
		},
		{
			name: "ipv6-quic",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				r.Set(enr.QUIC(9001))
				return SignNull(&r, id)
			}(),
			wantIP: netip.MustParseAddr("2001::ff00:0042:8329"),
		},
		{
			name: "ipv6-quic6",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.IPv6Addr(netip.MustParseAddr("2001::ff00:0042:8329")))
				r.Set(enr.QUIC6(9001))
				return SignNull(&r, id)
			}(),
			wantIP:   netip.MustParseAddr("2001::ff00:0042:8329"),
			wantQUIC: 9001,
		},
		{
			name: "dns-only",
			node: func() *Node {
				var r enr.Record
				r.Set(enr.UDP(30303))
				r.Set(enr.TCP(30303))
				n := SignNull(&r, id).WithHostname("example.com")
				return n
			}(),
			wantTCP: 30303,
			wantUDP: 30303,
			wantDNS: "example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.wantIP != test.node.IPAddr() {
				t.Errorf("node has wrong IP %v, want %v", test.node.IPAddr(), test.wantIP)
			}
			if test.wantUDP != test.node.UDP() {
				t.Errorf("node has wrong UDP port %d, want %d", test.node.UDP(), test.wantUDP)
			}
			if test.wantTCP != test.node.TCP() {
				t.Errorf("node has wrong TCP port %d, want %d", test.node.TCP(), test.wantTCP)
			}
			if quic, _ := test.node.QUICEndpoint(); test.wantQUIC != int(quic.Port()) {
				t.Errorf("node has wrong QUIC port %d, want %d", quic.Port(), test.wantQUIC)
			}
			if test.wantDNS != test.node.Hostname() {
				t.Errorf("node has wrong DNS name %s, want %s", test.node.Hostname(), test.wantDNS)
			}
		})
	}
}

func TestHexID(t *testing.T) {
	ref := ID{0, 0, 0, 0, 0, 0, 0, 128, 106, 217, 182, 31, 165, 174, 1, 67, 7, 235, 220, 150, 66, 83, 173, 205, 159, 44, 10, 57, 42, 161, 26, 188}
	id1 := HexID("0x00000000000000806ad9b61fa5ae014307ebdc964253adcd9f2c0a392aa11abc")
	id2 := HexID("00000000000000806ad9b61fa5ae014307ebdc964253adcd9f2c0a392aa11abc")

	if id1 != ref {
		t.Errorf("wrong id1\ngot  %v\nwant %v", id1[:], ref[:])
	}
	if id2 != ref {
		t.Errorf("wrong id2\ngot  %v\nwant %v", id2[:], ref[:])
	}
}

func TestID_textEncoding(t *testing.T) {
	ref := ID{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x20,
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x30,
		0x31, 0x32,
	}
	hex := "0102030405060708091011121314151617181920212223242526272829303132"

	text, err := ref.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(text, []byte(hex)) {
		t.Fatalf("text encoding did not match\nexpected: %s\ngot:      %s", hex, text)
	}

	id := new(ID)
	if err := id.UnmarshalText(text); err != nil {
		t.Fatal(err)
	}
	if *id != ref {
		t.Fatalf("text decoding did not match\nexpected: %s\ngot:      %s", ref, id)
	}
}

func TestID_distcmp(t *testing.T) {
	distcmpBig := func(target, a, b ID) int {
		tbig := new(big.Int).SetBytes(target[:])
		abig := new(big.Int).SetBytes(a[:])
		bbig := new(big.Int).SetBytes(b[:])
		return new(big.Int).Xor(tbig, abig).Cmp(new(big.Int).Xor(tbig, bbig))
	}
	if err := quick.CheckEqual(DistCmp, distcmpBig, nil); err != nil {
		t.Error(err)
	}
}

// The random tests is likely to miss the case where a and b are equal,
// this test checks it explicitly.
func TestID_distcmpEqual(t *testing.T) {
	base := ID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	x := ID{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
	if DistCmp(base, x, x) != 0 {
		t.Errorf("DistCmp(base, x, x) != 0")
	}
}

func TestID_logdist(t *testing.T) {
	logdistBig := func(a, b ID) int {
		abig, bbig := new(big.Int).SetBytes(a[:]), new(big.Int).SetBytes(b[:])
		return new(big.Int).Xor(abig, bbig).BitLen()
	}
	if err := quick.CheckEqual(LogDist, logdistBig, nil); err != nil {
		t.Error(err)
	}
}

// The random tests is likely to miss the case where a and b are equal,
// this test checks it explicitly.
func TestID_logdistEqual(t *testing.T) {
	x := ID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if LogDist(x, x) != 0 {
		t.Errorf("LogDist(x, x) != 0")
	}
}

func TestV4NodeIDFromPublicKey(t *testing.T) {
	tests := []struct {
		url string
	}{
		{
			url: "enode://69a90b35164ef862185d9f4d2c5eff79b92acd1360574c0edf36044055dc766d87285a820233ae5700e11c9ba06ce1cf23c1c68a4556121109776ce2a3990bba@127.0.0.1:30311",
		},
		{
			url: "enode://df1e8eb59e42cad3c4551b2a53e31a7e55a2fdde1287babd1e94b0836550b489ba16c40932e4dacb16cba346bd442c432265a299c4aca63ee7bb0f832b9f45eb@127.0.0.1:30311",
		},
		{
			url: "enr:-Je4QEeZoiY8OrUxlckLuU8leuuSfnrveD8PdUnCvHavJJzcGQn7nKikWNV_ZxbRtPt6si-tQNwT_aIYpksq3O2Hzblbg2V0aMfGhPCv0OOAgmlkgnY0gmlwhIe10fKJc2VjcDI1NmsxoQNGM7sxj2Wuen_j9kqzHEmjqSxct1UxxbA9kwl7ha7NRIN0Y3CCdl-DdWRwgnZf",
		},
		{
			url: "enr:-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8",
		},
	}
	for _, test := range tests {
		node, err := Parse(ValidSchemes, test.url)
		require.NoError(t, err)
		enodeUrl := node.URLv4()
		parts := strings.Split(enodeUrl, "@")
		raw, _ := hex.DecodeString(parts[0][8:])
		assert.Equal(t, node.id, V4NodeIDFromPublicKey(raw), node.ID())
	}
}

func TestV4NodeIDFromPublicKey_V4(t *testing.T) {
	keys := []string{
		"8db6f0c2269136605fe9f9ec445a8c2e86cdbec78a240de2b9900b4dc1abfdec3d7612810fd97ff502c64bfb2932c61d50dd7090c46234c86265da4dfe94691c",
		"3ade48a1778fcb7611a0ad8a7e1e4ff9691c16b8b9ff63baaf1172558c668e169a15454c80da59948e4c6f0eecd4e2f70d3f5edab29b5c12ebc7516d878b7c0d",
		"5d39d4493d53ab36aabcbe351ab91557dd7271ba1474e03ed95f0670e91520b279db8a7059caac5cd040456c0e569f1d7c91e65102d5e890b1deaa457230cceb",
		"479dd5cfcc7ec95921dacfd3d60fb94602bf5195df4420141b9598c5331c43e3e268a4190b915a72171162963b57127fdc22048bf2ee3e3f9341123de80a909b",
		"95c7332bcfd0e0621a49880489bf0194d4ca1d9f03a8da2a7b345124e9de4892647633ccb408f56b0fec4da5715523c403c38d1f34c32ba282b09e9ff2336abf",
		"3ea627218d86486ce4ab0490019a40b4a0de201b3d0fdcd30d84d8996e2246ac338c25429526c2b14a541f7a469d1a3777002d11c6c2fc1203e71ba99a1b54c2",
		"0eff59ec0128a61c479589e6bc4c2134854684985a3d485502ee03e38e59f3aaac4df9400e365f39930a80f41850476e5fc6e9eca2abc52c31f1438f896aec6d",
		"0ffec3cc7252bd7057192888c54a6176e64997eb76b1161d688bef1322a4af618b892a9c2932244f48ae5fe6e4dcfd8a8bea0d7284b558098d488040d031b41b",
		"25e570cf6ac81cd60ddbbabe65edfb96e1515d94b6c29901bc00ff10f6bd941e98ccc511ce98cc0e0924da4e2748685e4d7defbcdd60d362dde92ddf73bef22d"}
	for _, key := range keys {
		raw, _ := hex.DecodeString(key)
		id := V4NodeIDFromPublicKey(raw)
		t.Logf("%s", id)
	}
}

func TestNodeID_UnmarshalText(t *testing.T) {
	tests := []struct {
		pubkey string
		id     string
	}{
		{
			pubkey: "69a90b35164ef862185d9f4d2c5eff79b92acd1360574c0edf36044055dc766d87285a820233ae5700e11c9ba06ce1cf23c1c68a4556121109776ce2a3990bba",
			id:     "7248fb6372d6ef91b4929f35b7738e7d1e23033bc4a158cf01b01377970102e0",
		},
		{
			pubkey: "df1e8eb59e42cad3c4551b2a53e31a7e55a2fdde1287babd1e94b0836550b489ba16c40932e4dacb16cba346bd442c432265a299c4aca63ee7bb0f832b9f45eb",
			id:     "477bb318d8bc6b2a040b5de9313b4edc8e978a5ecbf8fd75f5590e2a07fa21fc",
		},
		{
			pubkey: "4633bb318f65ae7a7fe3f64ab31c49a3a92c5cb75531c5b03d93097b85aecd44acda3a032c8a315d27f01e1fe6c5b297622d78875f72d48f1d0c1635af04fb1d",
			id:     "009eef756b05e36100f55bc3a7b88031daeb83a3ebb66c4ccef0677ec477df3e",
		},
		{
			pubkey: "ca634cae0d49acb401d8a4c6b6fe8c55b70d115bf400769cc1400f3258cd31387574077f301b421bc84df7266c44e9e6d569fc56be00812904767bf5ccd1fc7f",
			id:     "a448f24c6d18e575453db13171562b71999873db5b286df957af199ec94617f7",
		},
	}
	for _, test := range tests {
		var id1, id2 ID
		id1.UnmarshalText([]byte(test.id))
		id2.UnmarshalText([]byte(test.pubkey))
		assert.Equal(t, id1, id2)
	}
}
