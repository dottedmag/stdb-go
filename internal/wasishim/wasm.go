package wasishim


// WASM binary encoding helpers.

// wasmBuilder constructs a WASM binary module.
type wasmBuilder struct {
	buf []byte
}

func newWASMBuilder() *wasmBuilder {
	return &wasmBuilder{
		buf: []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, // magic + version
	}
}

func (w *wasmBuilder) bytes() []byte { return w.buf }

func (w *wasmBuilder) writeSection(id byte, content []byte) {
	w.buf = append(w.buf, id)
	w.buf = appendULEB128(w.buf, uint32(len(content)))
	w.buf = append(w.buf, content...)
}

func appendULEB128(buf []byte, v uint32) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func appendSLEB128(buf []byte, v int64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			buf = append(buf, b)
			break
		}
		buf = append(buf, b|0x80)
	}
	return buf
}

func appendString(buf []byte, s string) []byte {
	buf = appendULEB128(buf, uint32(len(s)))
	buf = append(buf, s...)
	return buf
}

// WASM type constants.
const (
	wasmI32    byte = 0x7f
	wasmI64    byte = 0x7e
	wasmFuncTy byte = 0x60
)

// WASM section IDs.
const (
	sectionCustom    byte = 0
	sectionType      byte = 1
	sectionImport    byte = 2
	sectionFunc      byte = 3
	sectionTable     byte = 4
	sectionMemory    byte = 5
	sectionGlobal    byte = 6
	sectionExport    byte = 7
	sectionStart     byte = 8
	sectionElement   byte = 9
	sectionCode      byte = 10
	sectionData      byte = 11
	sectionDataCount byte = 12
)

// WASM opcodes.
const (
	opUnreachable byte = 0x00
	opNop         byte = 0x01
	opBlock       byte = 0x02
	opLoop        byte = 0x03
	opIf          byte = 0x04
	opElse        byte = 0x05
	opEnd         byte = 0x0b
	opBr          byte = 0x0c
	opBrIf        byte = 0x0d
	opReturn      byte = 0x0f
	opCall        byte = 0x10
	opCallIndirect byte = 0x11
	opLocalGet    byte = 0x20
	opLocalSet    byte = 0x21
	opGlobalGet   byte = 0x23
	opGlobalSet   byte = 0x24
	opI32Load     byte = 0x28
	opI64Store    byte = 0x37
	opI32Store    byte = 0x36
	opI32Store8   byte = 0x3a
	opI32Const    byte = 0x41
	opI64Const    byte = 0x42
	opI32Add      byte = 0x6a
	opI32Mul      byte = 0x6c
	opI32And      byte = 0x71
	opI32Ne       byte = 0x47
	opI32GeU      byte = 0x4f
	opI32WrapI64  byte = 0xa7
	opI64Add      byte = 0x7c
	opI64Xor      byte = 0x85
	opI64Shl      byte = 0x86
	opI64ShrU     byte = 0x88
	opRefFunc     byte = 0xd2
	opBlockVoid   byte = 0x40
)

// funcType defines a WASM function signature.
type funcType struct {
	params  []byte
	results []byte
}

// buildShimWASM constructs the WASI shim WASM module programmatically.
// See shim.wat for the reference WAT source.
func buildShimWASM() []byte {
	w := newWASMBuilder()

	// Function type definitions.
	// Index 0: (i32, i32) -> i32         [args_get, args_sizes_get, environ_get, environ_sizes_get, fd_fdstat_get, fd_fdstat_set_flags, fd_prestat_get]
	// Index 1: (i32, i64, i32) -> i32    [clock_time_get]
	// Index 2: (i32, i32, i32, i32) -> i32 [fd_write, poll_oneoff]
	// Index 3: (i32, i32) -> i32         [random_get] (same as 0, reuse)
	// Index 4: (i32) -> void             [proc_exit]
	// Index 5: () -> i32                 [sched_yield]
	// Index 6: (i32) -> i32              [fd_close]
	// Index 7: (i32, i32, i32) -> i32    [fd_prestat_dir_name]
	types := []funcType{
		{params: []byte{wasmI32, wasmI32}, results: []byte{wasmI32}},           // 0
		{params: []byte{wasmI32, wasmI64, wasmI32}, results: []byte{wasmI32}},  // 1
		{params: []byte{wasmI32, wasmI32, wasmI32, wasmI32}, results: []byte{wasmI32}}, // 2
		{params: []byte{wasmI32}, results: nil},                                // 3: proc_exit
		{params: nil, results: []byte{wasmI32}},                                // 4: sched_yield
		{params: []byte{wasmI32}, results: []byte{wasmI32}},                    // 5: fd_close
		{params: []byte{wasmI32, wasmI32, wasmI32}, results: []byte{wasmI32}},  // 6: fd_prestat_dir_name
	}

	// Type section
	var typeSec []byte
	typeSec = appendULEB128(typeSec, uint32(len(types)))
	for _, t := range types {
		typeSec = append(typeSec, wasmFuncTy)
		typeSec = appendULEB128(typeSec, uint32(len(t.params)))
		typeSec = append(typeSec, t.params...)
		typeSec = appendULEB128(typeSec, uint32(len(t.results)))
		typeSec = append(typeSec, t.results...)
	}
	w.writeSection(sectionType, typeSec)

	// Import section: memory from "main"
	var importSec []byte
	importSec = appendULEB128(importSec, 1) // 1 import
	importSec = appendString(importSec, "main")
	importSec = appendString(importSec, "memory")
	importSec = append(importSec, 0x02)    // import kind: memory
	importSec = append(importSec, 0x00)    // limits: no max
	importSec = appendULEB128(importSec, 0) // min pages: 0
	w.writeSection(sectionImport, importSec)

	// Function section: type indices for each function
	// Order: args_get(0), args_sizes_get(0), clock_time_get(1), environ_get(0),
	//        environ_sizes_get(0), fd_write(2), random_get(0), poll_oneoff(2),
	//        proc_exit(3), sched_yield(4), fd_close(5), fd_fdstat_get(0),
	//        fd_fdstat_set_flags(0), fd_prestat_get(0), fd_prestat_dir_name(6)
	funcTypeIndices := []uint32{0, 0, 1, 0, 0, 2, 0, 2, 3, 4, 5, 0, 0, 0, 6}
	var funcSec []byte
	funcSec = appendULEB128(funcSec, uint32(len(funcTypeIndices)))
	for _, idx := range funcTypeIndices {
		funcSec = appendULEB128(funcSec, idx)
	}
	w.writeSection(sectionFunc, funcSec)

	// Global section: clock_time (mut i64) and prng_state (mut i64)
	var globalSec []byte
	globalSec = appendULEB128(globalSec, 2) // 2 globals

	// global 0: clock_time = 1700000000000000000i64 (mut)
	globalSec = append(globalSec, wasmI64, 0x01) // i64, mutable
	globalSec = append(globalSec, opI64Const)
	globalSec = appendSLEB128(globalSec, 1700000000000000000)
	globalSec = append(globalSec, opEnd)

	// global 1: prng_state = 88172645463325252i64 (mut)
	globalSec = append(globalSec, wasmI64, 0x01) // i64, mutable
	globalSec = append(globalSec, opI64Const)
	globalSec = appendSLEB128(globalSec, 88172645463325252)
	globalSec = append(globalSec, opEnd)

	w.writeSection(sectionGlobal, globalSec)

	// Export section
	exportNames := []string{
		"args_get", "args_sizes_get", "clock_time_get", "environ_get",
		"environ_sizes_get", "fd_write", "random_get", "poll_oneoff",
		"proc_exit", "sched_yield", "fd_close", "fd_fdstat_get",
		"fd_fdstat_set_flags", "fd_prestat_get", "fd_prestat_dir_name",
	}
	var exportSec []byte
	exportSec = appendULEB128(exportSec, uint32(len(exportNames)))
	for i, name := range exportNames {
		exportSec = appendString(exportSec, name)
		exportSec = append(exportSec, 0x00) // export kind: func
		exportSec = appendULEB128(exportSec, uint32(i)) // func index
	}
	w.writeSection(sectionExport, exportSec)

	// Code section
	codeBodies := [][]byte{
		buildArgsGet(),
		buildArgsSizesGet(),
		buildClockTimeGet(),
		buildEnvironGet(),
		buildEnvironSizesGet(),
		buildFdWrite(),
		buildRandomGet(),
		buildPollOneoff(),
		buildProcExit(),
		buildSchedYield(),
		buildFdClose(),
		buildFdFdstatGet(),
		buildFdFdstatSetFlags(),
		buildFdPrestatGet(),
		buildFdPrestatDirName(),
	}

	var codeSec []byte
	codeSec = appendULEB128(codeSec, uint32(len(codeBodies)))
	for _, body := range codeBodies {
		codeSec = appendULEB128(codeSec, uint32(len(body)))
		codeSec = append(codeSec, body...)
	}
	w.writeSection(sectionCode, codeSec)

	return w.bytes()
}

// codeBody creates a function body with locals and instructions.
func codeBody(locals []byte, instrs []byte) []byte {
	var body []byte
	body = append(body, locals...)
	body = append(body, instrs...)
	body = append(body, opEnd)
	return body
}

// noLocals returns a "0 locals" declaration.
func noLocals() []byte {
	return []byte{0x00} // 0 local declarations
}

// localDecls returns encoded local declarations.
// Each pair is (count, type).
func localDecls(pairs ...byte) []byte {
	if len(pairs) == 0 {
		return []byte{0x00}
	}
	nDecls := len(pairs) / 2
	var buf []byte
	buf = appendULEB128(buf, uint32(nDecls))
	for i := 0; i < len(pairs); i += 2 {
		buf = appendULEB128(buf, uint32(pairs[i]))
		buf = append(buf, pairs[i+1])
	}
	return buf
}

// i32Const encodes an i32.const instruction.
func i32Const(v int32) []byte {
	var buf []byte
	buf = append(buf, opI32Const)
	buf = appendSLEB128(buf, int64(v))
	return buf
}

// i64Const encodes an i64.const instruction.
func i64Const(v int64) []byte {
	var buf []byte
	buf = append(buf, opI64Const)
	buf = appendSLEB128(buf, v)
	return buf
}

// localGet encodes a local.get instruction.
func localGetB(idx uint32) []byte {
	var buf []byte
	buf = append(buf, opLocalGet)
	buf = appendULEB128(buf, idx)
	return buf
}

// localSet encodes a local.set instruction.
func localSetB(idx uint32) []byte {
	var buf []byte
	buf = append(buf, opLocalSet)
	buf = appendULEB128(buf, idx)
	return buf
}

// globalGet encodes a global.get instruction.
func globalGetB(idx uint32) []byte {
	var buf []byte
	buf = append(buf, opGlobalGet)
	buf = appendULEB128(buf, idx)
	return buf
}

// globalSet encodes a global.set instruction.
func globalSetB(idx uint32) []byte {
	var buf []byte
	buf = append(buf, opGlobalSet)
	buf = appendULEB128(buf, idx)
	return buf
}

// i32Store encodes an i32.store with alignment and offset.
func i32StoreB(align, offset uint32) []byte {
	var buf []byte
	buf = append(buf, opI32Store)
	buf = appendULEB128(buf, align)
	buf = appendULEB128(buf, offset)
	return buf
}

// i64Store encodes an i64.store with alignment and offset.
func i64StoreB(align, offset uint32) []byte {
	var buf []byte
	buf = append(buf, opI64Store)
	buf = appendULEB128(buf, align)
	buf = appendULEB128(buf, offset)
	return buf
}

// i32Load encodes an i32.load with alignment and offset.
func i32LoadB(align, offset uint32) []byte {
	var buf []byte
	buf = append(buf, opI32Load)
	buf = appendULEB128(buf, align)
	buf = appendULEB128(buf, offset)
	return buf
}

// -- Function body builders --

// args_get(argv_ptr, argv_buf_ptr) -> i32: return 0
func buildArgsGet() []byte {
	return codeBody(noLocals(), i32Const(0))
}

// args_sizes_get(argc_ptr, argv_buf_size_ptr) -> i32: store 0 to both, return 0
func buildArgsSizesGet() []byte {
	var code []byte
	// i32.store(argc_ptr, 0)
	code = append(code, localGetB(0)...)
	code = append(code, i32Const(0)...)
	code = append(code, i32StoreB(2, 0)...)
	// i32.store(argv_buf_size_ptr, 0)
	code = append(code, localGetB(1)...)
	code = append(code, i32Const(0)...)
	code = append(code, i32StoreB(2, 0)...)
	// return 0
	code = append(code, i32Const(0)...)
	return codeBody(noLocals(), code)
}

// clock_time_get(id, precision, time_ptr) -> i32
func buildClockTimeGet() []byte {
	var code []byte
	// global.set $clock_time (i64.add(global.get $clock_time, 1_000_000))
	code = append(code, globalGetB(0)...)
	code = append(code, i64Const(1000000)...)
	code = append(code, opI64Add)
	code = append(code, globalSetB(0)...)
	// i64.store(time_ptr, global.get $clock_time)
	code = append(code, localGetB(2)...)  // time_ptr (param 2, after id:i32 and precision:i64)
	code = append(code, globalGetB(0)...) // clock_time
	code = append(code, i64StoreB(3, 0)...)
	// return 0
	code = append(code, i32Const(0)...)
	return codeBody(noLocals(), code)
}

// environ_get(environ_ptr, environ_buf_ptr) -> i32: return 0
func buildEnvironGet() []byte {
	return codeBody(noLocals(), i32Const(0))
}

// environ_sizes_get(count_ptr, size_ptr) -> i32: store 0 to both, return 0
func buildEnvironSizesGet() []byte {
	var code []byte
	code = append(code, localGetB(0)...)
	code = append(code, i32Const(0)...)
	code = append(code, i32StoreB(2, 0)...)
	code = append(code, localGetB(1)...)
	code = append(code, i32Const(0)...)
	code = append(code, i32StoreB(2, 0)...)
	code = append(code, i32Const(0)...)
	return codeBody(noLocals(), code)
}

// fd_write(fd, iovs_ptr, iovs_len, nwritten_ptr) -> i32
// Params: 0=fd, 1=iovs_ptr, 2=iovs_len, 3=nwritten_ptr
// Locals: 4=i, 5=total, 6=iov_offset, 7=buf_len
func buildFdWrite() []byte {
	var code []byte

	// if (fd != 1 && fd != 2) return 8
	code = append(code, localGetB(0)...)  // fd
	code = append(code, i32Const(1)...)
	code = append(code, opI32Ne)
	code = append(code, localGetB(0)...)  // fd
	code = append(code, i32Const(2)...)
	code = append(code, opI32Ne)
	code = append(code, opI32And)
	code = append(code, opIf, opBlockVoid)
	code = append(code, i32Const(8)...)
	code = append(code, opReturn)
	code = append(code, opEnd)

	// i = 0, total = 0
	code = append(code, i32Const(0)...)
	code = append(code, localSetB(4)...)
	code = append(code, i32Const(0)...)
	code = append(code, localSetB(5)...)

	// block $break
	code = append(code, opBlock, opBlockVoid)
	// loop $loop
	code = append(code, opLoop, opBlockVoid)

	// br_if $break (i >= iovs_len)
	code = append(code, localGetB(4)...)  // i
	code = append(code, localGetB(2)...)  // iovs_len
	code = append(code, opI32GeU)
	code = append(code, opBrIf)
	code = appendULEB128(code, 1) // break to outer block

	// iov_offset = iovs_ptr + i * 8
	code = append(code, localGetB(1)...)  // iovs_ptr
	code = append(code, localGetB(4)...)  // i
	code = append(code, i32Const(8)...)
	code = append(code, opI32Mul)
	code = append(code, opI32Add)
	code = append(code, localSetB(6)...)

	// buf_len = i32.load(iov_offset + 4)
	code = append(code, localGetB(6)...)
	code = append(code, i32Const(4)...)
	code = append(code, opI32Add)
	code = append(code, i32LoadB(2, 0)...)
	code = append(code, localSetB(7)...)

	// total += buf_len
	code = append(code, localGetB(5)...)
	code = append(code, localGetB(7)...)
	code = append(code, opI32Add)
	code = append(code, localSetB(5)...)

	// i++
	code = append(code, localGetB(4)...)
	code = append(code, i32Const(1)...)
	code = append(code, opI32Add)
	code = append(code, localSetB(4)...)

	// br $loop
	code = append(code, opBr)
	code = appendULEB128(code, 0) // continue loop

	code = append(code, opEnd) // end loop
	code = append(code, opEnd) // end block

	// i32.store(nwritten_ptr, total)
	code = append(code, localGetB(3)...)
	code = append(code, localGetB(5)...)
	code = append(code, i32StoreB(2, 0)...)

	// return 0
	code = append(code, i32Const(0)...)

	// 4 local variables: i, total, iov_offset, buf_len (all i32)
	return codeBody(localDecls(4, wasmI32), code)
}

// random_get(buf_ptr, buf_len) -> i32
// Params: 0=buf_ptr, 1=buf_len
// Locals: 2=i(i32), 3=state(i64)
func buildRandomGet() []byte {
	var code []byte

	// state = global.get $prng_state
	code = append(code, globalGetB(1)...)
	code = append(code, localSetB(3)...)

	// i = 0
	code = append(code, i32Const(0)...)
	code = append(code, localSetB(2)...)

	// block $break
	code = append(code, opBlock, opBlockVoid)
	// loop $loop
	code = append(code, opLoop, opBlockVoid)

	// br_if $break (i >= buf_len)
	code = append(code, localGetB(2)...)
	code = append(code, localGetB(1)...)
	code = append(code, opI32GeU)
	code = append(code, opBrIf)
	code = appendULEB128(code, 1)

	// state ^= state << 13
	code = append(code, localGetB(3)...)
	code = append(code, localGetB(3)...)
	code = append(code, i64Const(13)...)
	code = append(code, opI64Shl)
	code = append(code, opI64Xor)
	code = append(code, localSetB(3)...)

	// state ^= state >> 7
	code = append(code, localGetB(3)...)
	code = append(code, localGetB(3)...)
	code = append(code, i64Const(7)...)
	code = append(code, opI64ShrU)
	code = append(code, opI64Xor)
	code = append(code, localSetB(3)...)

	// state ^= state << 17
	code = append(code, localGetB(3)...)
	code = append(code, localGetB(3)...)
	code = append(code, i64Const(17)...)
	code = append(code, opI64Shl)
	code = append(code, opI64Xor)
	code = append(code, localSetB(3)...)

	// i32.store8(buf_ptr + i, i32.wrap_i64(state))
	code = append(code, localGetB(0)...) // buf_ptr
	code = append(code, localGetB(2)...) // i
	code = append(code, opI32Add)
	code = append(code, localGetB(3)...) // state
	code = append(code, opI32WrapI64)
	code = append(code, opI32Store8)
	code = appendULEB128(code, 0) // align
	code = appendULEB128(code, 0) // offset

	// i++
	code = append(code, localGetB(2)...)
	code = append(code, i32Const(1)...)
	code = append(code, opI32Add)
	code = append(code, localSetB(2)...)

	// br $loop
	code = append(code, opBr)
	code = appendULEB128(code, 0)

	code = append(code, opEnd) // end loop
	code = append(code, opEnd) // end block

	// global.set $prng_state = state
	code = append(code, localGetB(3)...)
	code = append(code, globalSetB(1)...)

	// return 0
	code = append(code, i32Const(0)...)

	// locals: 1 i32 (i), 1 i64 (state)
	return codeBody(localDecls(1, wasmI32, 1, wasmI64), code)
}

// poll_oneoff(in_ptr, out_ptr, nsubscriptions, nevents_ptr) -> i32
func buildPollOneoff() []byte {
	var code []byte
	// i32.store(nevents_ptr, nsubscriptions)
	code = append(code, localGetB(3)...) // nevents_ptr
	code = append(code, localGetB(2)...) // nsubscriptions
	code = append(code, i32StoreB(2, 0)...)
	// return 0
	code = append(code, i32Const(0)...)
	return codeBody(noLocals(), code)
}

// proc_exit(code) -> void: no-op (matches Rust wasi_stubs.rs behavior)
func buildProcExit() []byte {
	return codeBody(noLocals(), nil)
}

// sched_yield() -> i32: return 0
func buildSchedYield() []byte {
	return codeBody(noLocals(), i32Const(0))
}

// fd_close(fd) -> i32: return ERRNO_BADF (8)
func buildFdClose() []byte {
	return codeBody(noLocals(), i32Const(8))
}

// fd_fdstat_get(fd, stat_ptr) -> i32
// For fd 0/1/2 (stdin/stdout/stderr): zero the fdstat struct and return SUCCESS.
// Go's runtime calls this during init to check stdio file descriptors.
// For other fds: return ERRNO_BADF (8).
// Params: 0=fd, 1=stat_ptr
func buildFdFdstatGet() []byte {
	var code []byte
	// if (fd > 2) return 8
	code = append(code, localGetB(0)...) // fd
	code = append(code, i32Const(2)...)
	code = append(code, 0x4b) // i32.gt_u
	code = append(code, opIf, opBlockVoid)
	code = append(code, i32Const(8)...)
	code = append(code, opReturn)
	code = append(code, opEnd)

	// Zero 24 bytes of fdstat struct at stat_ptr.
	// fdstat is: fs_filetype(u8) + pad(1) + fs_flags(u16) + fs_rights_base(u64) + fs_rights_inheriting(u64) = 24 bytes
	// Store three i64 zeros to cover it.
	code = append(code, localGetB(1)...)   // stat_ptr
	code = append(code, i64Const(0)...)
	code = append(code, i64StoreB(0, 0)...) // bytes 0-7
	code = append(code, localGetB(1)...)
	code = append(code, i64Const(0)...)
	code = append(code, i64StoreB(0, 8)...) // bytes 8-15
	code = append(code, localGetB(1)...)
	code = append(code, i64Const(0)...)
	code = append(code, i64StoreB(0, 16)...) // bytes 16-23

	// return 0
	code = append(code, i32Const(0)...)
	return codeBody(noLocals(), code)
}

// fd_fdstat_set_flags(fd, flags) -> i32: return ERRNO_NOSYS (52)
func buildFdFdstatSetFlags() []byte {
	return codeBody(noLocals(), i32Const(52))
}

// fd_prestat_get(fd, prestat_ptr) -> i32: return ERRNO_BADF (8)
func buildFdPrestatGet() []byte {
	return codeBody(noLocals(), i32Const(8))
}

// fd_prestat_dir_name(fd, path_ptr, path_len) -> i32: return ERRNO_BADF (8)
func buildFdPrestatDirName() []byte {
	return codeBody(noLocals(), i32Const(8))
}

