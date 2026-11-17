// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
)

// memDumpBlock is the copy block size of writeELFCore. An all-zero block
// becomes a hole in the output.
const memDumpBlock = 1 << 20

// memDumpAlign is the file alignment of each PT_LOAD segment.
const memDumpAlign = 4096

// memRegion is one range of guest physical memory and its offset in a memory
// file.
type memRegion struct {
	Start  uint64 `json:"start"`
	Size   uint64 `json:"size"`
	Offset uint64 `json:"offset"`
}

// elfCoreOutput is the output of writeELFCore. *os.File implements it.
type elfCoreOutput interface {
	io.WriterAt
	Truncate(size int64) error
}

func memAlignUp(v, a uint64) uint64 {
	return (v + a - 1) / a * a
}

// writeELFCore writes an ELF64 x86_64 core file to out, with one PT_LOAD
// segment for each region of the memory file in. The physical and virtual
// address of each segment is the guest physical address of its region. It
// reads one block at a time, leaves holes for all-zero blocks, and calls
// progress, when not nil, with the percent that is copied.
func writeELFCore(out elfCoreOutput, in io.ReaderAt, regions []memRegion, progress func(float64)) error {
	if len(regions) == 0 || len(regions) >= 0xffff {
		return fmt.Errorf("invalid number of memory regions: %d", len(regions))
	}

	phoff := uint64(binary.Size(elf.Header64{}))
	phsize := uint64(binary.Size(elf.Prog64{}))

	hdr := elf.Header64{
		Type:      uint16(elf.ET_CORE),
		Machine:   uint16(elf.EM_X86_64),
		Version:   uint32(elf.EV_CURRENT),
		Phoff:     phoff,
		Ehsize:    uint16(phoff),
		Phentsize: uint16(phsize),
		Phnum:     uint16(len(regions)),
	}
	copy(hdr.Ident[:], elf.ELFMAG)
	hdr.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	hdr.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	hdr.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	hdr.Ident[elf.EI_OSABI] = byte(elf.ELFOSABI_NONE)

	var total uint64
	progs := make([]elf.Prog64, len(regions))

	off := memAlignUp(phoff+phsize*uint64(len(regions)), memDumpAlign)
	for i, r := range regions {
		if r.Size == 0 {
			return fmt.Errorf("memory region %d has size 0", i)
		}

		progs[i] = elf.Prog64{
			Type:   uint32(elf.PT_LOAD),
			Flags:  uint32(elf.PF_R | elf.PF_W | elf.PF_X),
			Off:    off,
			Vaddr:  r.Start,
			Paddr:  r.Start,
			Filesz: r.Size,
			Memsz:  r.Size,
			Align:  memDumpAlign,
		}

		total += r.Size
		off = memAlignUp(off+r.Size, memDumpAlign)
	}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, &hdr); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.LittleEndian, progs); err != nil {
		return err
	}
	if _, err := out.WriteAt(buf.Bytes(), 0); err != nil {
		return err
	}

	block := make([]byte, memDumpBlock)
	zero := make([]byte, memDumpBlock)

	var done uint64
	for i, r := range regions {
		for pos := uint64(0); pos < r.Size; {
			b := block[:min(uint64(len(block)), r.Size-pos)]

			n, err := in.ReadAt(b, int64(r.Offset+pos))
			if n < len(b) {
				if err == nil || err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return fmt.Errorf("read memory at offset %d: %v", r.Offset+pos, err)
			}

			if !bytes.Equal(b, zero[:len(b)]) {
				if _, err := out.WriteAt(b, int64(progs[i].Off+pos)); err != nil {
					return err
				}
			}

			pos += uint64(len(b))
			done += uint64(len(b))
			if progress != nil {
				progress(float64(done) * 100 / float64(total))
			}
		}
	}

	last := progs[len(progs)-1]

	return out.Truncate(int64(last.Off + last.Filesz))
}
