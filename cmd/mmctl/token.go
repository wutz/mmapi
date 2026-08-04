package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Access tokens are mmapi's own concept — a GPFS cluster has no equivalent —
// but the commands follow the mm naming so they sit alongside the rest:
// mmcrtoken creates, mmlstoken lists, mmdeltoken removes.

func cmdMmcrtoken() *command {
	return &command{
		name:    "mmcrtoken",
		summary: "Create an mmapi access token (mmapi extension)",
		usage: `Usage:
  mmcrtoken Device[,Device...]

The token grants access to the listed file systems, including every fileset
inside them. Requires MMAPI_ADMIN_TOKEN.`,
		spec: optSpec{},
		run:  runMmcrtoken,
	}
}

func runMmcrtoken(o options) error {
	if len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}

	var devices []string
	for _, operand := range o.operands {
		devices = append(devices, splitList(operand)...)
	}

	body, err := json.Marshal(map[string]any{"allowedFs": devices})
	if err != nil {
		return err
	}
	data, err := adminRequest("POST", "/api/v1/tokens", string(body))
	if err != nil {
		return err
	}

	var token struct {
		ID        string   `json:"id"`
		Secret    string   `json:"secret"`
		AllowedFS []string `json:"allowedFs"`
	}
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	fmt.Printf("Token %s created successfully.\n", token.ID)
	fmt.Printf("Secret: %s\n", token.Secret)
	return nil
}

func cmdMmlstoken() *command {
	return &command{
		name:    "mmlstoken",
		summary: "List mmapi access tokens (mmapi extension)",
		usage: `Usage:
  mmlstoken [-Y]

Requires MMAPI_ADMIN_TOKEN. Token secrets are never listed.`,
		spec: optSpec{"-Y": flagOpt},
		run:  runMmlstoken,
	}
}

func runMmlstoken(o options) error {
	if len(o.operands) > 0 {
		return usagef("Incorrect operand: %s", o.operands[0])
	}

	data, err := adminRequest("GET", "/api/v1/tokens", "")
	if err != nil {
		return err
	}
	var tokens []struct {
		ID        string   `json:"id"`
		AllowedFS []string `json:"allowedFs"`
	}
	if err := json.Unmarshal(data, &tokens); err != nil {
		return err
	}

	if o.has("-Y") {
		y := newYRecord("mmlstoken", "", "tokenId", "allowedFilesystems")
		y.printHeader()
		for _, t := range tokens {
			y.printRow(t.ID, strings.Join(t.AllowedFS, ","))
		}
		return nil
	}

	fmt.Println("Access tokens in mmapi:")
	fmt.Printf("%-38s%s\n", "Id", "Allowed file systems")
	for _, t := range tokens {
		fmt.Printf("%-38s%s\n", t.ID, strings.Join(t.AllowedFS, ","))
	}
	return nil
}

func cmdMmdeltoken() *command {
	return &command{
		name:    "mmdeltoken",
		summary: "Delete an mmapi access token (mmapi extension)",
		usage: `Usage:
  mmdeltoken TokenId

Requires MMAPI_ADMIN_TOKEN.`,
		spec: optSpec{},
		run:  runMmdeltoken,
	}
}

func runMmdeltoken(o options) error {
	if len(o.operands) == 0 {
		return usagef("Missing arguments.")
	}

	if _, err := adminRequest("DELETE", "/api/v1/tokens/"+o.operands[0], ""); err != nil {
		return err
	}
	fmt.Printf("Token %s deleted successfully.\n", o.operands[0])
	return nil
}
