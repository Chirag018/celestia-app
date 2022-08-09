package shares

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/celestiaorg/nmt/namespace"
	"github.com/tendermint/tendermint/pkg/consts"
	coretypes "github.com/tendermint/tendermint/types"
)

// MessageShareSplitter lazily splits messages into shares that will eventually be
// included in a data square. It also has methods to help progressively count
// how many shares the messages written take up.
type MessageShareSplitter struct {
	shares [][]NamespacedShare
	count  int
}

func NewMessageShareSplitter() *MessageShareSplitter {
	return &MessageShareSplitter{}
}

// Write adds the delimited data to the underlying contiguous shares.
func (msw *MessageShareSplitter) Write(msg coretypes.Message) {
	rawMsg, err := msg.MarshalDelimited()
	if err != nil {
		panic(fmt.Sprintf("app accepted a Message that can not be encoded %#v", msg))
	}
	newShares := make([]NamespacedShare, 0)
	newShares = AppendToShares(newShares, msg.NamespaceID, rawMsg)
	msw.shares = append(msw.shares, newShares)
	msw.count += len(newShares)
}

// WriteNamespacedPaddedShares adds empty shares using the namespace of the last written share.
// This is useful to follow the message layout rules. It assumes that at least
// one share has already been written, if not it panics.
func (msw *MessageShareSplitter) WriteNamespacedPaddedShares(count int) {
	if len(msw.shares) == 0 {
		panic("Cannot write empty namespaced shares on an empty MessageShareSplitter")
	}
	if count == 0 {
		return
	}
	lastMessage := msw.shares[len(msw.shares)-1]
	msw.shares = append(msw.shares, namespacedPaddedShares(lastMessage[0].ID, count))
	msw.count += count
}

// Export finalizes and returns the underlying contiguous shares.
func (msw *MessageShareSplitter) Export() NamespacedShares {
	msw.sortMsgs()
	shares := make([]NamespacedShare, msw.count)
	cursor := 0
	for _, messageShares := range msw.shares {
		for _, share := range messageShares {
			shares[cursor] = share
			cursor++
		}
	}
	return shares
}

// note: as an optimization we can probably get rid of this if we just add
// checks each time we write.
func (msw *MessageShareSplitter) sortMsgs() {
	sort.SliceStable(msw.shares, func(i, j int) bool {
		return bytes.Compare(msw.shares[i][0].ID, msw.shares[j][0].ID) < 0
	})
}

// Count returns the current number of shares that will be made if exporting.
func (msw *MessageShareSplitter) Count() int {
	return msw.count
}

// appendToShares appends raw data as shares.
// Used for messages.
func AppendToShares(shares []NamespacedShare, nid namespace.ID, rawData []byte) []NamespacedShare {
	if len(rawData) <= consts.MsgShareSize {
		rawShare := append(append(
			make([]byte, 0, len(nid)+len(rawData)),
			nid...),
			rawData...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, consts.ShareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
	} else { // len(rawData) > MsgShareSize
		shares = append(shares, splitMessage(rawData, nid)...)
	}
	return shares
}

// splitMessage breaks the data in a message into the minimum number of
// namespaced shares
func splitMessage(rawData []byte, nid namespace.ID) NamespacedShares {
	shares := make([]NamespacedShare, 0)
	firstRawShare := append(append(
		make([]byte, 0, consts.ShareSize),
		nid...),
		rawData[:consts.MsgShareSize]...,
	)
	shares = append(shares, NamespacedShare{firstRawShare, nid})
	rawData = rawData[consts.MsgShareSize:]
	for len(rawData) > 0 {
		shareSizeOrLen := min(consts.MsgShareSize, len(rawData))
		rawShare := append(append(
			make([]byte, 0, consts.ShareSize),
			nid...),
			rawData[:shareSizeOrLen]...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, consts.ShareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
		rawData = rawData[shareSizeOrLen:]
	}
	return shares
}
