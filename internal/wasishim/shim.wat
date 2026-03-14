;; WASI Preview 1 stub implementations for Go wasip1 modules.
;; These match the behavior in SpacetimeDB's wasi_stubs.rs.
;;
;; This file is the reference source for the WASM binary generated
;; programmatically in shim.go. It is NOT compiled or embedded directly.
;;
;; After wasm-merge combines this module (named "wasi_snapshot_preview1")
;; with the Go module (named "main"), all wasi_snapshot_preview1 imports
;; in the Go module are resolved to these local stub functions.

(module
  (import "main" "memory" (memory $memory 0))

  ;; ---- Mutable globals for clock and PRNG ----

  ;; Clock counter: starts at ~2023 epoch in nanoseconds
  (global $clock_time (mut i64) (i64.const 1700000000000000000))

  ;; PRNG state for random_get (xorshift64, non-zero seed)
  (global $prng_state (mut i64) (i64.const 88172645463325252))

  ;; ---- args_get(argv_ptr: i32, argv_buf_ptr: i32) -> i32 ----
  (func $args_get (param $argv_ptr i32) (param $argv_buf_ptr i32) (result i32)
    (i32.const 0)
  )

  ;; ---- args_sizes_get(argc_ptr: i32, argv_buf_size_ptr: i32) -> i32 ----
  (func $args_sizes_get (param $argc_ptr i32) (param $argv_buf_size_ptr i32) (result i32)
    (i32.store (local.get $argc_ptr) (i32.const 0))
    (i32.store (local.get $argv_buf_size_ptr) (i32.const 0))
    (i32.const 0)
  )

  ;; ---- clock_time_get(id: i32, precision: i64, time_ptr: i32) -> i32 ----
  ;; Increment counter by 1ms each call. Go panics on "nanotime returning zero".
  (func $clock_time_get (param $id i32) (param $precision i64) (param $time_ptr i32) (result i32)
    (global.set $clock_time
      (i64.add (global.get $clock_time) (i64.const 1000000))
    )
    (i64.store (local.get $time_ptr) (global.get $clock_time))
    (i32.const 0)
  )

  ;; ---- environ_get(environ_ptr: i32, environ_buf_ptr: i32) -> i32 ----
  (func $environ_get (param $environ_ptr i32) (param $environ_buf_ptr i32) (result i32)
    (i32.const 0)
  )

  ;; ---- environ_sizes_get(count_ptr: i32, size_ptr: i32) -> i32 ----
  (func $environ_sizes_get (param $count_ptr i32) (param $size_ptr i32) (result i32)
    (i32.store (local.get $count_ptr) (i32.const 0))
    (i32.store (local.get $size_ptr) (i32.const 0))
    (i32.const 0)
  )

  ;; ---- fd_write(fd: i32, iovs_ptr: i32, iovs_len: i32, nwritten_ptr: i32) -> i32 ----
  ;; Sum iov lengths for fd 1/2, write total to nwritten. Return BADF for other fds.
  (func $fd_write (param $fd i32) (param $iovs_ptr i32) (param $iovs_len i32) (param $nwritten_ptr i32) (result i32)
    (local $i i32)
    (local $total i32)
    (local $iov_offset i32)
    (local $buf_len i32)
    (if (i32.and
          (i32.ne (local.get $fd) (i32.const 1))
          (i32.ne (local.get $fd) (i32.const 2)))
      (then (return (i32.const 8)))
    )
    (local.set $i (i32.const 0))
    (local.set $total (i32.const 0))
    (block $break
      (loop $loop
        (br_if $break (i32.ge_u (local.get $i) (local.get $iovs_len)))
        (local.set $iov_offset
          (i32.add (local.get $iovs_ptr)
                   (i32.mul (local.get $i) (i32.const 8))))
        (local.set $buf_len
          (i32.load (i32.add (local.get $iov_offset) (i32.const 4))))
        (local.set $total (i32.add (local.get $total) (local.get $buf_len)))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $loop)
      )
    )
    (i32.store (local.get $nwritten_ptr) (local.get $total))
    (i32.const 0)
  )

  ;; ---- random_get(buf_ptr: i32, buf_len: i32) -> i32 ----
  ;; Fill with pseudo-random bytes using xorshift64.
  (func $random_get (param $buf_ptr i32) (param $buf_len i32) (result i32)
    (local $i i32)
    (local $state i64)
    (local.set $state (global.get $prng_state))
    (local.set $i (i32.const 0))
    (block $break
      (loop $loop
        (br_if $break (i32.ge_u (local.get $i) (local.get $buf_len)))
        (local.set $state
          (i64.xor (local.get $state)
                   (i64.shl (local.get $state) (i64.const 13))))
        (local.set $state
          (i64.xor (local.get $state)
                   (i64.shr_u (local.get $state) (i64.const 7))))
        (local.set $state
          (i64.xor (local.get $state)
                   (i64.shl (local.get $state) (i64.const 17))))
        (i32.store8
          (i32.add (local.get $buf_ptr) (local.get $i))
          (i32.wrap_i64 (local.get $state)))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $loop)
      )
    )
    (global.set $prng_state (local.get $state))
    (i32.const 0)
  )

  ;; ---- poll_oneoff(in_ptr: i32, out_ptr: i32, nsubscriptions: i32, nevents_ptr: i32) -> i32 ----
  (func $poll_oneoff (param $in_ptr i32) (param $out_ptr i32) (param $nsubscriptions i32) (param $nevents_ptr i32) (result i32)
    (i32.store (local.get $nevents_ptr) (local.get $nsubscriptions))
    (i32.const 0)
  )

  ;; ---- proc_exit(code: i32) -> void ----
  ;; No-op. Matches Rust wasi_stubs.rs behavior.
  (func $proc_exit (param $code i32)
    (nop)
  )

  ;; ---- sched_yield() -> i32 ----
  (func $sched_yield (result i32)
    (i32.const 0)
  )

  ;; ---- fd_close(fd: i32) -> i32 ----
  (func $fd_close (param $fd i32) (result i32)
    (i32.const 8)
  )

  ;; ---- fd_fdstat_get(fd: i32, stat_ptr: i32) -> i32 ----
  ;; Return SUCCESS with zeroed fdstat for fd 0/1/2, BADF for others.
  ;; Go's runtime checks stdio descriptors during init.
  (func $fd_fdstat_get (param $fd i32) (param $stat_ptr i32) (result i32)
    (if (i32.gt_u (local.get $fd) (i32.const 2))
      (then (return (i32.const 8)))
    )
    ;; Zero 24-byte fdstat struct
    (i64.store offset=0 (local.get $stat_ptr) (i64.const 0))
    (i64.store offset=8 (local.get $stat_ptr) (i64.const 0))
    (i64.store offset=16 (local.get $stat_ptr) (i64.const 0))
    (i32.const 0)
  )

  ;; ---- fd_fdstat_set_flags(fd: i32, flags: i32) -> i32 ----
  (func $fd_fdstat_set_flags (param $fd i32) (param $flags i32) (result i32)
    (i32.const 52)
  )

  ;; ---- fd_prestat_get(fd: i32, prestat_ptr: i32) -> i32 ----
  (func $fd_prestat_get (param $fd i32) (param $prestat_ptr i32) (result i32)
    (i32.const 8)
  )

  ;; ---- fd_prestat_dir_name(fd: i32, path_ptr: i32, path_len: i32) -> i32 ----
  (func $fd_prestat_dir_name (param $fd i32) (param $path_ptr i32) (param $path_len i32) (result i32)
    (i32.const 8)
  )

  ;; ---- Exports ----
  (export "args_get" (func $args_get))
  (export "args_sizes_get" (func $args_sizes_get))
  (export "clock_time_get" (func $clock_time_get))
  (export "environ_get" (func $environ_get))
  (export "environ_sizes_get" (func $environ_sizes_get))
  (export "fd_write" (func $fd_write))
  (export "random_get" (func $random_get))
  (export "poll_oneoff" (func $poll_oneoff))
  (export "proc_exit" (func $proc_exit))
  (export "sched_yield" (func $sched_yield))
  (export "fd_close" (func $fd_close))
  (export "fd_fdstat_get" (func $fd_fdstat_get))
  (export "fd_fdstat_set_flags" (func $fd_fdstat_set_flags))
  (export "fd_prestat_get" (func $fd_prestat_get))
  (export "fd_prestat_dir_name" (func $fd_prestat_dir_name))
)
