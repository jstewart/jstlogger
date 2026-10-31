package adif

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads a complete ADI file. The file has a header only when its first
// byte is not '<'. Malformed input is reported as a *ParseError.
func Parse(r io.Reader) (File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return File{}, fmt.Errorf("adif: read input: %w", err)
	}
	p := parser{data: data}
	return p.parse()
}

// parser holds the state of a single Parse call.
type parser struct {
	data     []byte
	pos      int
	inHeader bool
	file     File
	cur      Record
}

// recordNum returns the number used in errors for the record being parsed:
// 0 in the header, otherwise the 1-based index of the current record.
func (p *parser) recordNum() int {
	if p.inHeader {
		return 0
	}
	return len(p.file.Records) + 1
}

// fail builds a *ParseError for the current record.
func (p *parser) fail(field string, err error) error {
	return &ParseError{Record: p.recordNum(), Field: field, Err: err}
}

// parse walks the input tag by tag, filling p.file.
func (p *parser) parse() (File, error) {
	if len(p.data) > 0 && p.data[0] != '<' {
		p.inHeader = true
		end := bytes.IndexByte(p.data, '<')
		if end < 0 {
			end = len(p.data)
		}
		p.file.Header = &Header{Preamble: strings.TrimSpace(string(p.data[:end]))}
		p.pos = end
	}

	for {
		i := bytes.IndexByte(p.data[p.pos:], '<')
		if i < 0 {
			break
		}
		if err := p.tag(p.pos + i); err != nil {
			return File{}, err
		}
	}

	if p.inHeader {
		return File{}, p.fail("", ErrMissingEOH)
	}
	if len(p.cur) > 0 {
		return File{}, p.fail("", ErrTruncated)
	}
	return p.file, nil
}

// tag parses the tag starting at data[start] (a '<') and, for a data
// specifier, the value that follows it. It advances p.pos past both.
func (p *parser) tag(start int) error {
	n := bytes.IndexByte(p.data[start:], '>')
	if n < 0 {
		return p.fail("", errors.New("unterminated tag"))
	}
	end := start + n
	parts := strings.Split(string(p.data[start+1:end]), ":")
	name := strings.ToUpper(parts[0])
	p.pos = end + 1

	if len(parts) == 1 {
		return p.marker(name)
	}
	if name == "" {
		return p.fail("", errors.New("empty field name"))
	}
	if strings.ContainsAny(name, " \t\r\n") {
		return p.fail("", fmt.Errorf("invalid field name %q", parts[0]))
	}
	if len(parts) > 3 {
		return p.fail(name, fmt.Errorf("malformed data specifier %q", p.data[start:end+1]))
	}
	length, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil {
		return p.fail(name, fmt.Errorf("invalid length %q: %w", parts[1], err))
	}
	if length > uint64(len(p.data)-p.pos) {
		return p.fail(name, fmt.Errorf("length %d runs past end of input", length))
	}
	if _, dup := p.cur.Get(name); dup {
		return p.fail(name, errors.New("duplicate field"))
	}
	valueEnd := p.pos + int(length)
	p.cur = append(p.cur, Field{Name: name, Value: string(p.data[p.pos:valueEnd])})
	p.pos = valueEnd
	return nil
}

// marker handles a tag without a length: <EOH> or <EOR>.
func (p *parser) marker(name string) error {
	switch {
	case name == "EOH" && p.inHeader:
		p.file.Header.Fields = p.cur
		p.cur = nil
		p.inHeader = false
	case name == "EOR" && !p.inHeader:
		p.file.Records = append(p.file.Records, p.cur)
		p.cur = nil
	case name == "EOH", name == "EOR":
		return p.fail("", fmt.Errorf("unexpected <%s>", name))
	default:
		return p.fail(name, errors.New("data specifier has no length"))
	}
	return nil
}
