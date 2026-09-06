package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Use the public result projection so human output cannot accidentally expose
// unexported or JSON-excluded fields. Keep every published field, including
// false and empty values: dry-run, recovery, and partial-result details matter.
func writeHumanResult(out io.Writer, command string, data any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "fulla %s\n", command)
	writeHumanValue(&text, value, "  ")
	_, err = io.WriteString(out, text.String())
	return err
}

func writeHumanValue(out *strings.Builder, value any, indent string) {
	switch value := value.(type) {
	case map[string]any:
		if len(value) == 0 {
			fmt.Fprintf(out, "%s(none)\n", indent)
			return
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			// Even metadata keys can originate in user-controlled maps.
			label := strconv.Quote(strings.ReplaceAll(key, "_", " "))
			fmt.Fprintf(out, "%s%s:", indent, label[1:len(label)-1])
			writeHumanField(out, value[key], indent)
		}
	case []any:
		if len(value) == 0 {
			fmt.Fprintf(out, "%s(none)\n", indent)
			return
		}
		for _, item := range value {
			fmt.Fprintf(out, "%s-", indent)
			writeHumanField(out, item, indent)
		}
	default:
		fmt.Fprintf(out, "%s%s\n", indent, humanScalar(value))
	}
}

func writeHumanField(out *strings.Builder, value any, indent string) {
	switch value.(type) {
	case map[string]any, []any:
		out.WriteByte('\n')
		writeHumanValue(out, value, indent+"  ")
	default:
		fmt.Fprintf(out, " %s\n", humanScalar(value))
	}
}

func humanScalar(value any) string {
	switch value := value.(type) {
	case string:
		// Preserve empty strings and escape terminal controls, including bidi
		// formatting, while leaving ordinary Unicode names readable.
		return strconv.Quote(value)
	case bool:
		if value {
			return "yes"
		}
		return "no"
	case nil:
		return "(not set)"
	default:
		return fmt.Sprint(value) // json.Number, kept exact by UseNumber.
	}
}
