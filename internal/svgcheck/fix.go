package svgcheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type FixOptions struct {
	Target string
	Unsafe bool
}

type FixResult struct {
	SVG     []byte
	Changes []string
}

func Fix(input []byte, opts FixOptions) (FixResult, error) {
	if _, err := Check(input, opts.Target); err != nil {
		return FixResult{}, err
	}

	out := input
	changes := []string{}

	meta, err := inspect(out)
	if err != nil {
		return FixResult{}, err
	}

	if !meta.HasXMLNS {
		updated, changed := addRootAttribute(out, `xmlns="http://www.w3.org/2000/svg"`)
		if changed {
			out = updated
			changes = append(changes, "added SVG namespace")
		}
	}

	meta, _ = inspect(out)
	if meta.ViewBox == "" && meta.WidthPixels > 0 && meta.HeightPixels > 0 {
		viewBox := fmt.Sprintf(`viewBox="0 0 %s %s"`, trimFloat(meta.WidthPixels), trimFloat(meta.HeightPixels))
		updated, changed := addRootAttribute(out, viewBox)
		if changed {
			out = updated
			changes = append(changes, "added viewBox derived from width and height")
		}
	}

	if opts.Unsafe {
		updated, removedScripts := removeElementByName(out, "script")
		if removedScripts > 0 {
			out = updated
			changes = append(changes, fmt.Sprintf("removed %d script element(s)", removedScripts))
		}
		updated, removedAttrs := removeEventHandlerAttrs(out)
		if removedAttrs > 0 {
			out = updated
			changes = append(changes, fmt.Sprintf("removed %d event handler attribute(s)", removedAttrs))
		}
	}

	return FixResult{SVG: out, Changes: changes}, nil
}

func addRootAttribute(input []byte, attr string) ([]byte, bool) {
	loc := bytes.Index(bytes.ToLower(input), []byte("<svg"))
	if loc == -1 {
		return input, false
	}
	insertAt := loc + len("<svg")
	if insertAt >= len(input) {
		return input, false
	}

	prefix := " " + attr
	if input[insertAt] != '>' {
		prefix += " "
	}

	out := append([]byte{}, input[:insertAt]...)
	out = append(out, []byte(prefix)...)
	out = append(out, input[insertAt:]...)
	return out, true
}

func removeElementByName(input []byte, localName string) ([]byte, int) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	removed := 0
	skipDepth := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return input, 0
		}

		if skipDepth > 0 {
			switch tok := token.(type) {
			case xml.StartElement:
				if strings.EqualFold(tok.Name.Local, localName) {
					skipDepth++
				}
			case xml.EndElement:
				skipDepth--
			}
			continue
		}

		if start, ok := token.(xml.StartElement); ok && strings.EqualFold(start.Name.Local, localName) {
			removed++
			skipDepth = 1
			continue
		}

		if err := encoder.EncodeToken(token); err != nil {
			return input, 0
		}
	}

	if err := encoder.Flush(); err != nil {
		return input, 0
	}
	return out.Bytes(), removed
}

var eventAttrPattern = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*')`)

func removeEventHandlerAttrs(input []byte) ([]byte, int) {
	matches := eventAttrPattern.FindAllIndex(input, -1)
	if len(matches) == 0 {
		return input, 0
	}
	return eventAttrPattern.ReplaceAll(input, nil), len(matches)
}

func trimFloat(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%.0f", v)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}
