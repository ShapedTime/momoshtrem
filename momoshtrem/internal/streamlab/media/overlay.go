package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// A container overlay replaces a few bytes of generated content with
// headers that streaming.DetectFormat recognises. Everything between them
// stays generated: the lab checks bytes, it doesn't decode video.

// patch is bytes laid over the content at a torrent-absolute offset.
type patch struct {
	off  int64
	data []byte
}

// mkvHeaderLen is the EBML header plus the Segment's element header.
const mkvHeaderLen = 40 + 12

// mp4FtypLen is the ftyp box; mdat follows it.
const mp4FtypLen = 32

func validateContainer(f FileSpec) error {
	switch f.Container {
	case Raw, MKV:
		if f.MoovSize != 0 {
			return errors.New("moov_size is for MP4 files")
		}
		if f.Container == MKV && f.Size < mkvHeaderLen {
			return fmt.Errorf("an MKV needs at least %d bytes", mkvHeaderLen)
		}
	case MP4:
		moov := moovSize(f.MoovSize)
		if moov < 8 || moov > math.MaxUint32 {
			return fmt.Errorf("moov size %d out of range", moov)
		}
		if minSize := mp4FtypLen + 16 + moov; f.Size < minSize {
			return fmt.Errorf("an MP4 with a %d-byte moov needs at least %d bytes", moov, minSize)
		}
	default:
		return fmt.Errorf("unknown container %q", f.Container)
	}
	return nil
}

func moovSize(n int64) int64 {
	if n == 0 {
		return DefaultMoovSize
	}
	return n
}

// containerPatches returns f's header patches, in offset order.
func containerPatches(f File, moov int64) []patch {
	switch f.Container {
	case MKV:
		return []patch{{off: f.Offset, data: mkvHeader(f.Size)}}
	case MP4:
		return mp4Patches(f, moovSize(moov))
	default:
		return nil
	}
}

// mkvHeader is an EBML header (DocType matroska) and the start of a
// Segment that runs to the end of a file of size bytes.
func mkvHeader(size int64) []byte {
	h := []byte{
		0x1A, 0x45, 0xDF, 0xA3, 0xA3, // EBML, 35 bytes of children
		0x42, 0x86, 0x81, 0x01, // EBMLVersion 1
		0x42, 0xF7, 0x81, 0x01, // EBMLReadVersion 1
		0x42, 0xF2, 0x81, 0x04, // EBMLMaxIDLength 4
		0x42, 0xF3, 0x81, 0x08, // EBMLMaxSizeLength 8
		0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a', // DocType
		0x42, 0x87, 0x81, 0x04, // DocTypeVersion 4
		0x42, 0x85, 0x81, 0x02, // DocTypeReadVersion 2
		0x18, 0x53, 0x80, 0x67, // Segment
	}
	// The Segment's size as an 8-byte vint: 0x01, then 7 bytes.
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], uint64(size-mkvHeaderLen))
	sz[0] = 0x01
	return append(h, sz[:]...)
}

// mp4Patches lays out ftyp at 0, mdat after it up to the moov box, and moov
// at the end of the file. mdat takes a 64-bit size when it needs one.
func mp4Patches(f File, moov int64) []patch {
	ftyp := []byte{
		0, 0, 0, mp4FtypLen, 'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', 0, 0, 2, 0, // major brand, minor version
		'i', 's', 'o', 'm', 'i', 's', 'o', '2', 'a', 'v', 'c', '1', 'm', 'p', '4', '1',
	}
	moovOff := f.Size - moov
	mdatSize := moovOff - mp4FtypLen
	var mdat []byte
	if mdatSize <= math.MaxUint32 {
		mdat = binary.BigEndian.AppendUint32(nil, uint32(mdatSize))
		mdat = append(mdat, "mdat"...)
	} else {
		mdat = binary.BigEndian.AppendUint32(nil, 1)
		mdat = append(mdat, "mdat"...)
		mdat = binary.BigEndian.AppendUint64(mdat, uint64(mdatSize))
	}
	moovHdr := binary.BigEndian.AppendUint32(nil, uint32(moov))
	moovHdr = append(moovHdr, "moov"...)
	return []patch{
		{off: f.Offset, data: ftyp},
		{off: f.Offset + mp4FtypLen, data: mdat},
		{off: f.Offset + moovOff, data: moovHdr},
	}
}

// applyPatches lays the patches over p, which holds the content at off.
func (l *Layout) applyPatches(p []byte, off int64) {
	end := off + int64(len(p))
	// The first patch that ends after off.
	i := sort.Search(len(l.patches), func(i int) bool {
		pt := l.patches[i]
		return pt.off+int64(len(pt.data)) > off
	})
	for ; i < len(l.patches) && l.patches[i].off < end; i++ {
		pt := l.patches[i]
		src := pt.data
		dst := pt.off - off
		if dst < 0 {
			src = src[-dst:]
			dst = 0
		}
		copy(p[dst:], src)
	}
}
