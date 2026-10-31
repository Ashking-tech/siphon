package pdf

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"siphon/internal/siphonerr"
)

// Expect is the structural contract for a generated image PDF.
type Expect struct {
	Width    int
	Height   int
	Gray     bool
	HasAlpha bool
	HasICC   bool
}

// VerifyFile reads path and runs the structural verifier.
func VerifyFile(path string, exp Expect) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot read the generated PDF")
	}
	return Verify(data, exp)
}

type pdfObject struct {
	num    int
	dict   []byte
	stream []byte
}

// Verify parses the generated PDF structurally (spec §8.3). It rejects a
// truncated file, bad xref, wrong stream length, missing EOF, wrong page
// count, wrong page box, wrong image geometry, and an SMask/ICC mismatch.
func Verify(data []byte, exp Expect) error {
	trimmed := bytes.TrimRight(data, " \t\r\n")
	if len(trimmed) == 0 {
		return verifyErr("file is empty")
	}
	if !bytes.HasPrefix(trimmed, []byte("%PDF-1.7")) {
		return verifyErr("header is not %PDF-1.7")
	}
	if !bytes.HasSuffix(trimmed, []byte("%%EOF")) {
		return verifyErr("file does not end with %%EOF")
	}

	objects, err := parseObjects(trimmed)
	if err != nil {
		return err
	}
	catalogIDs := findObjects(objects, func(d []byte) bool { return hasName(d, "/Type", "Catalog") })
	if len(catalogIDs) != 1 {
		return verifyErr("expected exactly one catalog, found %d", len(catalogIDs))
	}
	catalog := objects[catalogIDs[0]]
	pagesID, ok := dictRef(catalog.dict, "/Pages")
	if !ok {
		return verifyErr("catalog has no /Pages reference")
	}
	pages, ok := objects[pagesID]
	if !ok {
		return verifyErr("catalog /Pages reference does not resolve")
	}
	if !hasName(pages.dict, "/Type", "Pages") {
		return verifyErr("catalog /Pages object is not a page tree")
	}
	count, ok := dictInt(pages.dict, "/Count")
	if !ok || count != 1 {
		return verifyErr("page tree /Count is not 1")
	}
	kids, err := dictRefs(pages.dict, "/Kids")
	if err != nil {
		return err
	}
	if len(kids) != 1 {
		return verifyErr("page tree has %d kids, want 1", len(kids))
	}
	page, ok := objects[kids[0]]
	if !ok {
		return verifyErr("page /Kids reference does not resolve")
	}
	if !hasName(page.dict, "/Type", "Page") {
		return verifyErr("kid is not a /Page")
	}
	parent, ok := dictRef(page.dict, "/Parent")
	if !ok || parent != pagesID {
		return verifyErr("page /Parent does not point at the page tree")
	}
	box, err := dictNumbers(page.dict, "/MediaBox")
	if err != nil {
		return err
	}
	if len(box) != 4 || box[2] <= box[0] || box[3] <= box[1] {
		return verifyErr("page /MediaBox is not positive")
	}

	imageID, ok := imageRef(page.dict)
	if !ok {
		return verifyErr("page has no /Im0 image XObject")
	}
	image, ok := objects[imageID]
	if !ok {
		return verifyErr("image XObject reference does not resolve")
	}
	iw, okW := dictInt(image.dict, "/Width")
	ih, okH := dictInt(image.dict, "/Height")
	if !okW || !okH || iw != exp.Width || ih != exp.Height {
		return verifyErr("image XObject is %dx%d, expected %dx%d", iw, ih, exp.Width, exp.Height)
	}
	bpc, ok := dictInt(image.dict, "/BitsPerComponent")
	if !ok || bpc != 8 {
		return verifyErr("image /BitsPerComponent is not 8")
	}
	iccRef, hasICC, err := colorSpace(image.dict)
	if err != nil {
		return err
	}
	if hasICC != exp.HasICC {
		return verifyErr("image ICCBased color space presence is wrong")
	}
	if hasICC {
		icc, ok := objects[iccRef]
		if !ok {
			return verifyErr("ICCBased profile reference does not resolve")
		}
		n, ok := dictInt(icc.dict, "/N")
		if !ok {
			return verifyErr("ICCBased profile has no /N")
		}
		wantN := 3
		if exp.Gray {
			wantN = 1
		}
		if n != wantN {
			return verifyErr("ICC profile /N is %d, expected %d", n, wantN)
		}
		if len(icc.stream) == 0 {
			return verifyErr("ICCBased profile stream is empty")
		}
	} else {
		if exp.Gray && !hasName(image.dict, "/ColorSpace", "DeviceGray") {
			return verifyErr("image ColorSpace is not DeviceGray")
		}
		if !exp.Gray && !hasName(image.dict, "/ColorSpace", "DeviceRGB") {
			return verifyErr("image ColorSpace is not DeviceRGB")
		}
	}
	filter, ok := dictName(image.dict, "/Filter")
	if !ok || (filter != "DCTDecode" && filter != "FlateDecode") {
		return verifyErr("image has an unexpected /Filter")
	}
	if len(image.stream) == 0 {
		return verifyErr("image stream is empty")
	}

	smaskID, hasSMask := dictRef(image.dict, "/SMask")
	if hasSMask != exp.HasAlpha {
		return verifyErr("SMask presence does not match the source alpha")
	}
	if hasSMask {
		smask, ok := objects[smaskID]
		if !ok {
			return verifyErr("SMask reference does not resolve")
		}
		sw, ok1 := dictInt(smask.dict, "/Width")
		sh, ok2 := dictInt(smask.dict, "/Height")
		if !ok1 || !ok2 || sw != exp.Width || sh != exp.Height {
			return verifyErr("SMask dimensions do not match the image")
		}
		if !hasName(smask.dict, "/ColorSpace", "DeviceGray") {
			return verifyErr("SMask is not DeviceGray")
		}
		if len(smask.stream) == 0 {
			return verifyErr("SMask stream is empty")
		}
	}
	return nil
}

func parseObjects(data []byte) (map[int]pdfObject, error) {
	start := bytes.LastIndex(data, []byte("startxref"))
	if start < 0 {
		return nil, verifyErr("missing startxref")
	}
	fields := strings.Fields(string(data[start+len("startxref"):]))
	if len(fields) == 0 {
		return nil, verifyErr("startxref has no offset")
	}
	off, err := strconv.Atoi(fields[0])
	if err != nil || off < 0 || off >= len(data) {
		return nil, verifyErr("startxref offset is invalid")
	}
	if !bytes.HasPrefix(data[off:], []byte("xref")) {
		return nil, verifyErr("startxref does not point at an xref table")
	}

	pos := off + len("xref")
	pos = skipSpace(data, pos)
	lineEnd := bytes.IndexByte(data[pos:], '\n')
	if lineEnd < 0 {
		return nil, verifyErr("truncated xref header")
	}
	header := strings.Fields(string(data[pos : pos+lineEnd]))
	pos += lineEnd + 1
	if len(header) != 2 {
		return nil, verifyErr("invalid xref subsection header")
	}
	first, err1 := strconv.Atoi(header[0])
	n, err2 := strconv.Atoi(header[1])
	if err1 != nil || err2 != nil || first != 0 || n < 1 || n > 1<<20 {
		return nil, verifyErr("invalid xref subsection")
	}
	if pos+n*20 > len(data) {
		return nil, verifyErr("truncated xref table")
	}

	offsets := make([]int, n)
	for i := 0; i < n; i++ {
		entry := data[pos+i*20 : pos+(i+1)*20]
		if len(entry) < 18 {
			return nil, verifyErr("truncated xref entry")
		}
		off, err := strconv.Atoi(strings.TrimSpace(string(entry[0:10])))
		if err != nil {
			return nil, verifyErr("invalid xref offset for object %d", i)
		}
		kind := entry[17]
		if i == 0 {
			if kind != 'f' {
				return nil, verifyErr("object 0 is not free")
			}
			continue
		}
		if kind == 'f' {
			return nil, verifyErr("object %d is unexpectedly free", i)
		}
		offsets[i] = off
	}

	objects := make(map[int]pdfObject, n-1)
	for num := 1; num < n; num++ {
		if offsets[num] <= 0 || offsets[num] >= len(data) {
			return nil, verifyErr("object %d has an invalid xref offset", num)
		}
		obj, err := parseObject(data, offsets[num], num)
		if err != nil {
			return nil, err
		}
		objects[num] = obj
	}
	return objects, nil
}

func parseObject(data []byte, off, wantNum int) (pdfObject, error) {
	rest := data[off:]
	objKeyword := bytes.Index(rest, []byte("obj"))
	if objKeyword < 0 {
		return pdfObject{}, verifyErr("object %d offset does not point at an object", wantNum)
	}
	head := strings.Fields(string(rest[:objKeyword]))
	if len(head) < 2 {
		return pdfObject{}, verifyErr("object %d has a malformed header", wantNum)
	}
	num, err := strconv.Atoi(head[0])
	if err != nil || num != wantNum {
		return pdfObject{}, verifyErr("xref offset for object %d resolves to object %s", wantNum, head[0])
	}
	bodyStart := off + objKeyword + len("obj")
	bodyStart = skipSpace(data, bodyStart)
	if bodyStart >= len(data) {
		return pdfObject{}, verifyErr("object %d is truncated", wantNum)
	}
	body := data[bodyStart:]

	streamAt := bytes.Index(body, []byte("stream"))
	endAt := bytes.Index(body, []byte("endobj"))
	if streamAt < 0 || (endAt >= 0 && endAt < streamAt) {
		if endAt < 0 {
			return pdfObject{}, verifyErr("object %d has no endobj", wantNum)
		}
		return pdfObject{num: wantNum, dict: bytes.TrimSpace(body[:endAt])}, nil
	}

	dict := bytes.TrimSpace(body[:streamAt])
	length, ok := dictInt(dict, "/Length")
	if !ok || length < 0 {
		return pdfObject{}, verifyErr("object %d stream has no direct /Length", wantNum)
	}
	p := bodyStart + streamAt + len("stream")
	if p < len(data) && data[p] == '\r' {
		p++
	}
	if p < len(data) && data[p] == '\n' {
		p++
	}
	if length > len(data)-p {
		return pdfObject{}, verifyErr("object %d stream length exceeds the file", wantNum)
	}
	stream := data[p : p+length]
	p += length
	p = skipSpace(data, p)
	if !bytes.HasPrefix(data[p:], []byte("endstream")) {
		return pdfObject{}, verifyErr("object %d stream /Length does not match its data", wantNum)
	}
	p += len("endstream")
	p = skipSpace(data, p)
	if !bytes.HasPrefix(data[p:], []byte("endobj")) {
		return pdfObject{}, verifyErr("object %d stream does not end with endobj", wantNum)
	}
	return pdfObject{num: wantNum, dict: dict, stream: stream}, nil
}

func findObjects(objs map[int]pdfObject, fn func([]byte) bool) []int {
	var ids []int
	for id, obj := range objs {
		if fn(obj.dict) {
			ids = append(ids, id)
		}
	}
	return ids
}

func imageRef(pageDict []byte) (int, bool) {
	idx := bytes.Index(pageDict, []byte("/Im0"))
	if idx < 0 {
		return 0, false
	}
	rest := pageDict[idx+len("/Im0"):]
	rest = bytes.TrimLeft(rest, " \t\r\n")
	fields := strings.Fields(string(rest))
	if len(fields) < 3 || fields[2] != "R" {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

func hasName(dict []byte, key, name string) bool {
	v, ok := dictName(dict, key)
	return ok && v == name
}

func dictName(dict []byte, key string) (string, bool) {
	rest, ok := afterKey(dict, key)
	if !ok {
		return "", false
	}
	if len(rest) == 0 || rest[0] != '/' {
		return "", false
	}
	i := 1
	for i < len(rest) && !isDelimiter(rest[i]) {
		i++
	}
	if i == 1 {
		return "", false
	}
	return string(rest[1:i]), true
}

func dictInt(dict []byte, key string) (int, bool) {
	rest, ok := afterKey(dict, key)
	if !ok {
		return 0, false
	}
	end := 0
	for end < len(rest) && !isDelimiter(rest[end]) {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(string(rest[:end]))
	if err != nil {
		return 0, false
	}
	return n, true
}

func dictRef(dict []byte, key string) (int, bool) {
	rest, ok := afterKey(dict, key)
	if !ok {
		return 0, false
	}
	fields := strings.Fields(string(rest))
	if len(fields) < 3 || fields[2] != "R" {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

func dictRefs(dict []byte, key string) ([]int, error) {
	rest, ok := afterKey(dict, key)
	if !ok {
		return nil, verifyErr("missing %s", key)
	}
	if len(rest) == 0 || rest[0] != '[' {
		return nil, verifyErr("%s is not an array", key)
	}
	end := bytes.IndexByte(rest, ']')
	if end < 0 {
		return nil, verifyErr("%s array is unterminated", key)
	}
	fields := strings.Fields(string(rest[1:end]))
	var refs []int
	for i := 0; i+2 < len(fields); i += 3 {
		if fields[i+2] != "R" {
			continue
		}
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return nil, verifyErr("%s contains a bad reference", key)
		}
		refs = append(refs, n)
	}
	return refs, nil
}

func dictNumbers(dict []byte, key string) ([]float64, error) {
	rest, ok := afterKey(dict, key)
	if !ok {
		return nil, verifyErr("missing %s", key)
	}
	if len(rest) == 0 || rest[0] != '[' {
		return nil, verifyErr("%s is not an array", key)
	}
	end := bytes.IndexByte(rest, ']')
	if end < 0 {
		return nil, verifyErr("%s array is unterminated", key)
	}
	fields := strings.Fields(string(rest[1:end]))
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, verifyErr("%s contains a bad number", key)
		}
		out = append(out, n)
	}
	return out, nil
}

// colorSpace returns the ICC object reference, whether the image uses an
// ICCBased color space, and an error for a malformed value.
func colorSpace(dict []byte) (int, bool, error) {
	rest, ok := afterKey(dict, "/ColorSpace")
	if !ok {
		return 0, false, verifyErr("image has no /ColorSpace")
	}
	if len(rest) == 0 {
		return 0, false, verifyErr("empty /ColorSpace")
	}
	if rest[0] != '[' {
		return 0, false, nil
	}
	end := bytes.IndexByte(rest, ']')
	if end < 0 {
		return 0, false, verifyErr("unterminated /ColorSpace array")
	}
	fields := strings.Fields(string(rest[1:end]))
	if len(fields) < 4 || fields[0] != "/ICCBased" || fields[3] != "R" {
		return 0, false, verifyErr("unsupported /ColorSpace array")
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false, verifyErr("bad ICCBased reference")
	}
	return n, true, nil
}

func afterKey(dict []byte, key string) ([]byte, bool) {
	needle := []byte(key)
	from := 0
	for {
		idx := bytes.Index(dict[from:], needle)
		if idx < 0 {
			return nil, false
		}
		idx += from
		end := idx + len(needle)
		if end == len(dict) || isDelimiter(dict[end]) {
			return bytes.TrimLeft(dict[end:], " \t\r\n"), true
		}
		from = end
	}
}

func isDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '/', '[', ']', '<', '>', '(', ')':
		return true
	}
	return false
}

func skipSpace(data []byte, pos int) int {
	for pos < len(data) {
		switch data[pos] {
		case ' ', '\t', '\r', '\n':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func verifyErr(format string, args ...any) error {
	return siphonerr.New(siphonerr.VerifyFailed, "PDF verification failed: "+fmt.Sprintf(format, args...))
}
