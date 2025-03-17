package bsc

import (
	"bytes"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Constants to match up protocol versions and messages
const (
	Bsc1 = 1
)

// ProtocolName is the official short name of the `bsc` protocol used during
// devp2p capability negotiation.
const ProtocolName = "bsc"

// ProtocolVersions are the supported versions of the `bsc` protocol (first
// is primary).
var ProtocolVersions = []uint{Bsc1}

// protocolLengths are the number of implemented message corresponding to
// different protocol versions.
var protocolLengths = map[uint]uint64{Bsc1: 2}

// maxMessageSize is the maximum cap on the size of a protocol message.
const maxMessageSize = 10 * 1024 * 1024

const (
	BscCapMsg                  = 0x00 // bsc capability msg used upon handshake
	VotesMsg                   = 0x01
	GetBlocksByRangeMsg        = 0x02 // it can request (Head-n, Head] range blocks from remote peer
	RangeBlocksMsg             = 0x03 // the replied blocks from remote peer
	GetAuthorizationForPeerMsg = 0x04 // sentry node can request authorization from validator to join the public network
	PeerAuthorizationMsg       = 0x05 // the replied auth from validator
)

var defaultExtra = []byte{0x00}

var (
	errNoBscCapMsg             = errors.New("no bsc capability message")
	errMsgTooLarge             = errors.New("message too long")
	errDecode                  = errors.New("invalid message")
	errInvalidMsgCode          = errors.New("invalid message code")
	errProtocolVersionMismatch = errors.New("protocol version mismatch")
)

// Packet represents a p2p message in the `bsc` protocol.
type Packet interface {
	Name() string // Name returns a string corresponding to the message type.
	Kind() byte   // Kind returns the message type.
}

// BscCapPacket is the network packet for bsc capability message.
type BscCapPacket struct {
	ProtocolVersion uint
	Extra           rlp.RawValue // for extension
}

func (*BscCapPacket) Name() string { return "BscCap" }
func (*BscCapPacket) Kind() byte   { return BscCapMsg }

const (
	CapExtraDefaultVer      = 0
	CapExtraWithIdentityVer = 1
)

type ExtraWithIdentity struct {
	list []*NodeIdentity
}

type NodeIdentity struct {
	ChainID     uint64
	ForkID      [4]byte
	GenesisHash common.Hash
	PeerID      string
	NodeVersion string
	Extend      []byte
	CreateTime  uint64   // a UTC unix timestamp when it creates
	V, R, S     *big.Int // signature values
}

func EncodeCapExtra(ver byte, val interface{}) ([]byte, error) {
	if val == nil {
		return nil, errors.New("input nil value")
	}
	var buf bytes.Buffer
	buf.WriteByte(ver)
	if err := rlp.Encode(&buf, val); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeExtraWithIdentity(enc []byte) (*ExtraWithIdentity, error) {
	if len(enc) <= 1 {
		return nil, errors.New("too short TxDAG bytes")
	}

	if enc[0] != CapExtraWithIdentityVer {
		return nil, errors.New("wrong ver")
	}

	extra := new(ExtraWithIdentity)
	if err := rlp.DecodeBytes(enc[1:], extra); err != nil {
		return nil, err
	}
	return extra, nil
}

// VotesPacket is the network packet for votes record.
type VotesPacket struct {
	Votes []*types.VoteEnvelope
}

func (*VotesPacket) Name() string { return "Votes" }
func (*VotesPacket) Kind() byte   { return VotesMsg }

type GetAuthorizationForPeerPacket struct {
	RequestId   uint64
	ChainID     uint64
	ForkID      [4]byte
	GenesisHash common.Hash
	PeerID      string
	NodeVersion string
	Extend      []byte
}

func (*GetAuthorizationForPeerPacket) Name() string { return "GetAuthorizationForPeer" }
func (*GetAuthorizationForPeerPacket) Kind() byte   { return GetAuthorizationForPeerMsg }

type PeerAuthorizationPacket struct {
	RequestId uint64
	Auth      *NodeIdentity
}

func (*PeerAuthorizationPacket) Name() string { return "PeerAuthorization" }
func (*PeerAuthorizationPacket) Kind() byte   { return PeerAuthorizationMsg }

type GetBlocksByRangePacket struct {
	RequestId   uint64
	StartHeight uint64 // The start block height expected to be obtained from
	Count       uint64 // Get the number of blocks from the start
}

func (*GetBlocksByRangePacket) Name() string { return "GetBlocksByRange" }
func (*GetBlocksByRangePacket) Kind() byte   { return GetBlocksByRangeMsg }

type RangeBlocksPacket struct {
	RequestId uint64
	Blocks    []*types.Block
}

func (*RangeBlocksPacket) Name() string { return "RangeBlocks" }
func (*RangeBlocksPacket) Kind() byte   { return RangeBlocksMsg }
