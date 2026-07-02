package server

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// SVG uploads are the one image type we accept that is XML/script-capable rather
// than inert raster bytes. An uploaded SVG is served from our own origin (opening
// it directly is a full script-execution context) and is also inlined via v-html
// on the public player board (the theme "board flourish"). So a raw SVG is a
// stored-XSS vector. sanitizeSVG neutralizes it server-side at upload time.
//
// Approach: parse the markup and re-serialize it, keeping only safe tokens.
// It is an allowlist for *structure* (we re-emit what we parsed) with a denylist
// for the known script vectors:
//   - <script> and <foreignObject> elements are dropped with their whole subtree
//   - SMIL animation elements (<animate>/<set>/…) that target href or an event
//     attribute are dropped (they can set javascript: at runtime)
//   - event-handler attributes (any name starting with "on") are removed
//   - href/xlink:href/src are kept only when they are safe local fragment refs
//     ("#id") — javascript:, data:, and external URLs are removed
//   - style attributes containing javascript: or expression() are removed
//   - comments, processing instructions, and DTD/DOCTYPE directives are dropped
//
// Namespaces are flattened to literal prefixes on output so Go's xml encoder
// re-emits xlink:href etc. verbatim rather than mangling the namespace map.
//
// Returns the cleaned bytes and true when the input parsed and had an <svg> root;
// false means the upload is not a usable SVG and should be rejected.
func sanitizeSVG(raw []byte) ([]byte, bool) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity // predefined + HTML entities; no external entity expansion

	var out bytes.Buffer
	enc := xml.NewEncoder(&out)

	sawSVG := false
	skipDepth := 0 // >0 while inside a dropped element's subtree

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if isDroppedSVGElement(t.Name.Local) || isDangerousAnimation(t) {
				skipDepth = 1
				continue
			}
			if strings.EqualFold(t.Name.Local, "svg") {
				sawSVG = true
			}
			el := xml.StartElement{Name: flattenXMLName(t.Name), Attr: sanitizeSVGAttrs(t.Attr)}
			if err := enc.EncodeToken(el); err != nil {
				return nil, false
			}
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if err := enc.EncodeToken(xml.EndElement{Name: flattenXMLName(t.Name)}); err != nil {
				return nil, false
			}
		case xml.CharData:
			if skipDepth > 0 {
				continue
			}
			if err := enc.EncodeToken(t); err != nil {
				return nil, false
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// Dropped: comments are noise, and PIs (<?xml-stylesheet?>) and DTD
			// directives can pull in external resources / define entities.
			continue
		}
	}

	if err := enc.Flush(); err != nil {
		return nil, false
	}
	if !sawSVG {
		return nil, false
	}
	return out.Bytes(), true
}

// isDroppedSVGElement reports whether an element (and its subtree) must be removed
// wholesale. <script> is obvious; <foreignObject> can embed arbitrary HTML.
func isDroppedSVGElement(local string) bool {
	switch strings.ToLower(local) {
	case "script", "foreignobject":
		return true
	}
	return false
}

// isDangerousAnimation reports whether a SMIL animation element targets a URL or
// event attribute, e.g. <set attributeName="href" to="javascript:alert(1)"/>,
// which would inject script at animation time. Transform/motion animations that
// don't target href/on* are left alone.
func isDangerousAnimation(e xml.StartElement) bool {
	switch strings.ToLower(e.Name.Local) {
	case "set", "animate", "animatetransform", "animatemotion":
	default:
		return false
	}
	for _, a := range e.Attr {
		if strings.EqualFold(a.Name.Local, "attributeName") {
			v := strings.ToLower(strings.TrimSpace(a.Value))
			if v == "href" || strings.HasPrefix(v, "on") {
				return true
			}
		}
	}
	return false
}

// sanitizeSVGAttrs drops event-handler, unsafe-reference, and dangerous-style
// attributes and flattens the remaining names for verbatim re-encoding.
func sanitizeSVGAttrs(attrs []xml.Attr) []xml.Attr {
	cleaned := make([]xml.Attr, 0, len(attrs))
	for _, a := range attrs {
		local := strings.ToLower(a.Name.Local)
		if strings.HasPrefix(local, "on") { // onload, onclick, onmouseover, …
			continue
		}
		switch local {
		case "href", "src":
			if !isSafeSVGRef(a.Value) {
				continue
			}
		case "style":
			if hasDangerousCSS(a.Value) {
				continue
			}
		}
		cleaned = append(cleaned, xml.Attr{Name: flattenXMLName(a.Name), Value: a.Value})
	}
	return cleaned
}

// isSafeSVGRef reports whether an href/src value is a safe local fragment
// reference (e.g. "#gradient1"). Everything else — javascript:, data:, and
// absolute/relative external URLs — is rejected so the attribute is dropped.
func isSafeSVGRef(v string) bool {
	return strings.HasPrefix(strings.TrimSpace(v), "#")
}

// hasDangerousCSS reports whether an inline style value carries a script vector.
func hasDangerousCSS(v string) bool {
	l := strings.ToLower(v)
	return strings.Contains(l, "javascript:") || strings.Contains(l, "expression(")
}

// flattenXMLName collapses a namespaced xml.Name into a single literal Local name
// (clearing Space) so the encoder writes it verbatim instead of synthesizing new
// namespace prefixes. Known SVG namespaces get their conventional prefix; xmlns
// declarations are preserved literally.
func flattenXMLName(n xml.Name) xml.Name {
	switch n.Space {
	case "":
		return xml.Name{Local: n.Local}
	case "xmlns":
		return xml.Name{Local: "xmlns:" + n.Local}
	case "http://www.w3.org/1999/xlink":
		return xml.Name{Local: "xlink:" + n.Local}
	default:
		// Default SVG namespace and anything else: emit the bare local name.
		return xml.Name{Local: n.Local}
	}
}
