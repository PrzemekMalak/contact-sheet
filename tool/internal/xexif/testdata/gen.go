//go:build ignore

// gen writes the EXIF-only JPEG fixtures used by xexif tests.
// Run from tool/internal/xexif: go run testdata/gen.go
package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"sort"
)

type entry struct {
	tag, typ uint16
	count    uint32
	data     []byte // big-endian payload
}

func ascii(tag uint16, s string) entry {
	return entry{tag, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}

func short(tag, v uint16) entry {
	return entry{tag, 3, 1, binary.BigEndian.AppendUint16(nil, v)}
}

func rational(tag uint16, num, den uint32) entry {
	b := binary.BigEndian.AppendUint32(nil, num)
	return entry{tag, 5, 1, binary.BigEndian.AppendUint32(b, den)}
}

func long(tag uint16, v uint32) entry {
	return entry{tag, 4, 1, binary.BigEndian.AppendUint32(nil, v)}
}

// ifd serialises entries as an IFD located at absolute TIFF offset base.
func ifd(es []entry, base uint32) []byte {
	sort.Slice(es, func(i, j int) bool { return es[i].tag < es[j].tag })
	var head, extra bytes.Buffer
	binary.Write(&head, binary.BigEndian, uint16(len(es)))
	extraOff := base + 2 + uint32(len(es))*12 + 4
	for _, e := range es {
		binary.Write(&head, binary.BigEndian, e.tag)
		binary.Write(&head, binary.BigEndian, e.typ)
		binary.Write(&head, binary.BigEndian, e.count)
		if len(e.data) <= 4 {
			v := make([]byte, 4)
			copy(v, e.data)
			head.Write(v)
		} else {
			binary.Write(&head, binary.BigEndian, extraOff+uint32(extra.Len()))
			extra.Write(e.data)
			if extra.Len()%2 == 1 {
				extra.WriteByte(0)
			}
		}
	}
	head.Write([]byte{0, 0, 0, 0})
	return append(head.Bytes(), extra.Bytes()...)
}

// jpeg builds SOI + APP1(Exif) + EOI. Layout: header(8) | IFD0 | ExifIFD.
func jpeg(ifd0, exifIFD []entry) []byte {
	if exifIFD != nil {
		ifd0 = append(ifd0, long(0x8769, 0))
		size0 := uint32(len(ifd(append([]entry{}, ifd0...), 8)))
		ifd0[len(ifd0)-1] = long(0x8769, 8+size0)
	}
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8}
	tiff = append(tiff, ifd(ifd0, 8)...)
	if exifIFD != nil {
		tiff = append(tiff, ifd(exifIFD, uint32(len(tiff)))...)
	}
	app1 := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(app1)+2))
	out = append(out, app1...)
	return append(out, 0xFF, 0xD9)
}

func main() {
	full := jpeg(
		[]entry{ascii(0x010F, "SONY"), ascii(0x0110, "ILCE-6700"), short(0x0112, 6)},
		[]entry{
			rational(0x829A, 1, 320),
			rational(0x829D, 63, 10),
			short(0x8827, 100),
			ascii(0x9003, "2026:10:21 11:28:21"),
			rational(0x920A, 560, 1),
			ascii(0xA434, "FE 400mm F4.5 GM OSS + 1.4X Teleconverter"),
		},
	)
	nodate := jpeg([]entry{ascii(0x010F, "SONY"), ascii(0x0110, "ILCE-6700")}, nil)
	dtOnly := jpeg([]entry{ascii(0x0132, "2025:01:02 03:04:05")}, nil)
	must(os.WriteFile("testdata/noexif.jpg", []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644))
	must(os.WriteFile("testdata/dtonly.jpg", dtOnly, 0o644))
	must(os.WriteFile("testdata/full.jpg", full, 0o644))
	must(os.WriteFile("testdata/nodate.jpg", nodate, 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
