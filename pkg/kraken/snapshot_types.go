package kraken

import (
	"bytes"
	"fmt"
	"text/tabwriter"
)

// BookRequest represents a subscription request for the order book
type BookRequest struct {
	Method string     `json:"method"` // Always "subscribe"
	Params BookParams `json:"params"`
	ReqID  int        `json:"req_id,omitempty"` // Optional client request identifier
}

// BookParams represents the parameters for a book subscription
type BookParams struct {
	Channel  string   `json:"channel"`            // Always "book"
	Symbol   []string `json:"symbol"`             // List of currency pairs
	Depth    int      `json:"depth,omitempty"`    // Optional: 10, 25, 100, 500, 1000 (default: 10)
	Snapshot bool     `json:"snapshot,omitempty"` // Default: true
}

// BookResponse represents the subscription acknowledgment
type BookResponse struct {
	Method  string     `json:"method"` // Always "subscribe"
	Result  BookResult `json:"result"`
	Success bool       `json:"success"`
	TimeIn  string     `json:"time_in"`
	TimeOut string     `json:"time_out"`
	Error   string     `json:"error,omitempty"`
}

// BookResult represents the result field in the subscription response
type BookResult struct {
	Channel  string   `json:"channel"`  // Always "book"
	Symbol   string   `json:"symbol"`   // The currency pair
	Depth    int      `json:"depth"`    // Confirmed depth level
	Snapshot bool     `json:"snapshot"` // Confirmed snapshot flag
	Warnings []string `json:"warnings,omitempty"`
}

// BookSnapshot represents the initial snapshot of the order book
type BookSnapshot struct {
	Channel string     `json:"channel"` // Always "book"
	Type    string     `json:"type"`    // "snapshot"
	Data    []BookData `json:"data"`
}

type BookData struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int         `json:"checksum"`
	Timestamp string      `json:"timestamp,omitempty"`
}

type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

func (b *BookSnapshot) PrettyPrint() string {
	if len(b.Data) == 0 {
		return "Empty order book"
	}

	book := b.Data[0]
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', tabwriter.TabIndent)

	// Print header
	fmt.Fprintf(w, "\n🏦 Order Book for %s (Checksum: %d)\n", book.Symbol, book.Checksum)
	fmt.Fprintf(w, "\n%s\t\t|\t%s\n", "BIDS", "ASKS")
	fmt.Fprintf(w, "%s\t|\t%s\n", "Price\tQuantity", "Price\tQuantity")
	fmt.Fprintf(w, "%s\t|\t%s\n", "-----\t--------", "-----\t--------")

	// Determine max length for parallel printing
	maxLen := len(book.Bids)
	if len(book.Asks) > maxLen {
		maxLen = len(book.Asks)
	}

	// Print bids and asks side by side
	for i := 0; i < maxLen; i++ {
		var bidStr, askStr string

		if i < len(book.Bids) {
			bid := book.Bids[i]
			bidStr = fmt.Sprintf("%.4f\t%.4f", bid.Price, bid.Qty)
		} else {
			bidStr = "\t"
		}

		if i < len(book.Asks) {
			ask := book.Asks[i]
			askStr = fmt.Sprintf("%.4f\t%.4f", ask.Price, ask.Qty)
		} else {
			askStr = "\t"
		}

		fmt.Fprintf(w, "%s\t|\t%s\n", bidStr, askStr)
	}

	w.Flush()
	return buf.String()
}

type SnapshotUpdate struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

type BookUnsubscribe struct {
	Method string     `json:"method"` // Always "unsubscribe"
	Params BookParams `json:"params"` // Always { channel: "book", symbol: [pair] }
}

type BookUnsubscribeAck struct {
	Method  string                `json:"method"`  // Always "unsubscribe"
	Result  BookUnsubscribeResult `json:"result"`  // Always { channel: "book", snapshot: true, symbol: "pair"}
	Success bool                  `json:"success"` // True or false
	TimeIn  string                `json:"time_in"`
	TimeOut string                `json:"time_out"`
}

type BookUnsubscribeResult struct {
	Channel  string `json:"channel"` // Always "book"
	Depth    int    `json:"depth"`
	Snapshot bool   `json:"snapshot"` // Always true
	Symbol   string `json:"symbol"`
}
