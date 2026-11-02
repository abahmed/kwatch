package transport

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
)

// fakeResolver answers DNS queries from a table of name to addresses, in
// process: a *net.Resolver that uses it never leaves the machine. A name
// it does not know has no addresses.
type fakeResolver map[string][]string

// resolver returns a pure-Go resolver whose every query r answers.
func (r fakeResolver) resolver() *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(
		context.Context, string, string,
	) (net.Conn, error) {
		client, server := net.Pipe()
		go r.serve(server)
		return client, nil
	}}
}

// serve answers the length-prefixed queries of one stream connection.
func (r fakeResolver) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		var size [2]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return
		}
		query := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}
		answer := r.answer(query)
		out := binary.BigEndian.AppendUint16(nil, uint16(len(answer)))
		if _, err := conn.Write(append(out, answer...)); err != nil {
			return
		}
	}
}

// answer builds the response to query: its question, then one record per
// address of the asked family.
func (r fakeResolver) answer(query []byte) []byte {
	end := 12
	var labels []string
	for query[end] != 0 {
		n := int(query[end])
		labels = append(labels, string(query[end+1:end+1+n]))
		end += n + 1
	}
	end += 5 // the root label, then type and class
	qtype := binary.BigEndian.Uint16(query[end-4:])
	var records [][]byte
	for _, raw := range r[strings.Join(labels, ".")] {
		ip := net.ParseIP(raw)
		if v4 := ip.To4(); v4 != nil && qtype == 1 {
			records = append(records, v4)
		} else if v4 == nil && qtype == 28 {
			records = append(records, ip.To16())
		}
	}
	out := append([]byte(nil), query[:2]...)
	out = append(out, 0x81, 0x80, 0, 1)
	out = binary.BigEndian.AppendUint16(out, uint16(len(records)))
	out = append(out, 0, 0, 0, 0)
	out = append(out, query[12:end]...)
	for _, rdata := range records {
		out = append(out, 0xc0, 12)
		out = binary.BigEndian.AppendUint16(out, qtype)
		out = append(out, 0, 1, 0, 0, 0, 60)
		out = binary.BigEndian.AppendUint16(out, uint16(len(rdata)))
		out = append(out, rdata...)
	}
	return out
}
