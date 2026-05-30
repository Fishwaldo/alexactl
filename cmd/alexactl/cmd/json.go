// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import "encoding/json"

func jsonMarshalIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
func jsonUnmarshal(b []byte, v any) error     { return json.Unmarshal(b, v) }
