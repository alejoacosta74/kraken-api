package client

import (
	"encoding/json"
	"fmt"
)

// prettyPrintJSON takes a JSON message as bytes and prints it in a formatted, human-readable form.
// This is primarily used for debugging and logging purposes.
//
// Parameters:
//   - msg: Raw JSON message as a byte slice
//
// The function:
//  1. Attempts to unmarshal the JSON into a generic map
//  2. If successful, formats it with proper indentation
//  3. Prints the formatted JSON to stdout
//
// If the JSON is invalid, it logs the raw message as-is with an error indication.
func (c *WebSocketClient) prettyPrintJSON(msg []byte) {
	var prettyMsg map[string]interface{}
	if err := json.Unmarshal(msg, &prettyMsg); err != nil {
		c.logger.Printf("Invalid JSON: %s\n", string(msg))
		return
	}
	formatted, _ := json.MarshalIndent(prettyMsg, "", "  ")
	fmt.Println(string(formatted))
}
