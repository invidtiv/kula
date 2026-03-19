#!/usr/bin/env python3
"""
Utility to inspect Kula tier files (v1 JSON and v2 binary codec).

v2 binary layout mirrors internal/storage/codec.go exactly.
"""

import struct
import sys
import os
import math
import datetime
import json
from typing import Optional, Tuple, BinaryIO

MAGIC = b"KULASPIE"
HEADER_SIZE = 64
CODEC_V2 = 2

# Flag bits (preamble flags uint16)
FLAG_HAS_MIN  = 1 << 0
FLAG_HAS_MAX  = 1 << 1
FLAG_HAS_DATA = 1 << 2

FIXED_BLOCK_SIZE = 218  # must match fixedBlockSize in codec.go


# ---------------------------------------------------------------------------
# Header
# ---------------------------------------------------------------------------

def parse_header(buf: bytes) -> Tuple[bytes, int, int, int, int, int, int]:
    unpacked = struct.unpack("<8sQQQQqq8s", buf)
    return unpacked[:7]


# ---------------------------------------------------------------------------
# v2 binary helpers
# ---------------------------------------------------------------------------

def _get_u8(data: bytes, off: int) -> Tuple[int, int]:
    return data[off], off + 1

def _get_u16(data: bytes, off: int) -> Tuple[int, int]:
    return struct.unpack_from("<H", data, off)[0], off + 2

def _get_i32(data: bytes, off: int) -> Tuple[int, int]:
    return struct.unpack_from("<i", data, off)[0], off + 4

def _get_u32(data: bytes, off: int) -> Tuple[int, int]:
    return struct.unpack_from("<I", data, off)[0], off + 4

def _get_u64(data: bytes, off: int) -> Tuple[int, int]:
    return struct.unpack_from("<Q", data, off)[0], off + 8

def _get_f32(data: bytes, off: int) -> Tuple[float, int]:
    v = struct.unpack_from("<f", data, off)[0]
    return round(float(v), 6), off + 4

def _get_f64(data: bytes, off: int) -> Tuple[float, int]:
    v = struct.unpack_from("<d", data, off)[0]
    return v, off + 8

def _get_str(data: bytes, off: int) -> Tuple[str, int]:
    """uint8-length-prefixed UTF-8 string (matches appendStr / getStr in Go)."""
    if off >= len(data):
        return "", off
    n = data[off]
    off += 1
    end = min(off + n, len(data))
    return data[off:end].decode("utf-8", errors="replace"), end


# ---------------------------------------------------------------------------
# Fixed block decoder — mirrors decodeFixed() in codec.go
#
# Offsets match appendFixed() exactly:
#   [0:28]    cpu total × 7 float32 (usage,user,sys,iowait,irq,softirq,steal)
#   [28:30]   num_cores uint16
#   [30:34]   cpu_temp  float32
#   [34:46]   load[3]   float32  (load1, load5, load15)
#   [46:48]   load_running uint16
#   [48:50]   load_total   uint16
#   [50:106]  mem[7]    uint64   (total,free,avail,used,buffers,cached,shmem)
#   [106:110] mem_used_pct float32
#   [110:134] swap[3]   uint64   (total,free,used)
#   [134:138] swap_used_pct float32
#   [138:146] tcp_curr_estab uint64
#   [146:150] tcp_in_errs float32
#   [150:154] tcp_out_rsts float32
#   [154:158] sock_tcp_inuse int32
#   [158:162] sock_tcp_tw    int32
#   [162:166] sock_udp_inuse int32
#   [166:190] proc[6] int32 (total,running,sleeping,zombie,blocked,threads)
#   [190:198] uptime_sec float64  (NOT float32!)
#   [198:202] entropy    int32
#   [202:203] user_count uint8
#   [203:204] clock_sync uint8
#   [204:212] self_rss   uint64
#   [212:216] self_cpu_pct float32
#   [216:218] self_fds  uint16
# ---------------------------------------------------------------------------

def _decode_fixed(data: bytes, off: int) -> Tuple[dict, int]:
    if off + FIXED_BLOCK_SIZE > len(data):
        return {}, off + FIXED_BLOCK_SIZE

    base = off

    # CPU total (0..28)
    cpu_usage,   off = _get_f32(data, off)
    cpu_user,    off = _get_f32(data, off)
    cpu_sys,     off = _get_f32(data, off)
    cpu_iowait,  off = _get_f32(data, off)
    cpu_irq,     off = _get_f32(data, off)
    cpu_softirq, off = _get_f32(data, off)
    cpu_steal,   off = _get_f32(data, off)

    # CPU meta (28..34)
    num_cores, off = _get_u16(data, off)
    cpu_temp,  off = _get_f32(data, off)

    # Load average (34..50)
    load1,   off = _get_f32(data, off)
    load5,   off = _get_f32(data, off)
    load15,  off = _get_f32(data, off)
    running, off = _get_u16(data, off)
    total,   off = _get_u16(data, off)

    # Memory (50..110)
    mem_total,   off = _get_u64(data, off)
    mem_free,    off = _get_u64(data, off)
    mem_avail,   off = _get_u64(data, off)
    mem_used,    off = _get_u64(data, off)
    mem_buffers, off = _get_u64(data, off)
    mem_cached,  off = _get_u64(data, off)
    mem_shmem,   off = _get_u64(data, off)
    mem_pct,     off = _get_f32(data, off)

    # Swap (110..138)
    swap_total, off = _get_u64(data, off)
    swap_free,  off = _get_u64(data, off)
    swap_used,  off = _get_u64(data, off)
    swap_pct,   off = _get_f32(data, off)

    # TCP + sockets (138..166)
    tcp_estab,  off = _get_u64(data, off)
    tcp_inerrs, off = _get_f32(data, off)
    tcp_outrst, off = _get_f32(data, off)
    sock_tcp,   off = _get_i32(data, off)
    sock_tw,    off = _get_i32(data, off)
    sock_udp,   off = _get_i32(data, off)

    # Process (166..190)
    proc_total,   off = _get_i32(data, off)
    proc_running, off = _get_i32(data, off)
    proc_sleep,   off = _get_i32(data, off)
    proc_zombie,  off = _get_i32(data, off)
    proc_blocked, off = _get_i32(data, off)
    proc_threads, off = _get_i32(data, off)

    # System (190..204)  — note: uptime is float64!
    uptime,    off = _get_f64(data, off)
    entropy,   off = _get_i32(data, off)
    user_count, off = _get_u8(data, off)
    clock_sync, off = _get_u8(data, off)

    # Self (204..218)
    self_rss, off = _get_u64(data, off)
    self_cpu, off = _get_f32(data, off)
    self_fds, off = _get_u16(data, off)

    assert off == base + FIXED_BLOCK_SIZE, f"fixed decode mismatch: {off - base} != {FIXED_BLOCK_SIZE}"

    return {
        "cpu": {
            "usage": cpu_usage, "user": cpu_user, "system": cpu_sys,
            "iowait": cpu_iowait, "irq": cpu_irq, "softirq": cpu_softirq,
            "steal": cpu_steal, "num_cores": num_cores, "temp": cpu_temp,
        },
        "load": {"load1": load1, "load5": load5, "load15": load15,
                 "running": running, "total": total},
        "memory": {"total": mem_total, "free": mem_free, "available": mem_avail,
                   "used": mem_used, "buffers": mem_buffers, "cached": mem_cached,
                   "shmem": mem_shmem, "used_pct": mem_pct},
        "swap": {"total": swap_total, "free": swap_free, "used": swap_used,
                 "used_pct": swap_pct},
        "tcp": {"curr_estab": tcp_estab, "in_errs_ps": tcp_inerrs,
                "out_rsts_ps": tcp_outrst},
        "sockets": {"tcp_inuse": sock_tcp, "tcp_tw": sock_tw, "udp_inuse": sock_udp},
        "process": {"total": proc_total, "running": proc_running,
                    "sleeping": proc_sleep, "zombie": proc_zombie,
                    "blocked": proc_blocked, "threads": proc_threads},
        "system": {"uptime_sec": uptime, "entropy": entropy,
                   "user_count": user_count, "clock_synced": bool(clock_sync)},
        "self": {"cpu_pct": self_cpu, "mem_rss": self_rss, "fds": self_fds},
    }, off


# ---------------------------------------------------------------------------
# Variable block decoder — mirrors decodeVariable() in codec.go
#
# Section order (matches appendVariable):
#   1. Network interfaces:  uint16 count + per-iface entries
#   2. CPU sensors:         uint16 count + per-sensor entries
#   3. Disk devices:        uint16 count + per-device entries (with sub-sensors)
#   4. Filesystems:         uint16 count + per-fs entries
#   5. System strings:      hostname (str8), clock_source (str8)
#   6. GPU entries:         uint16 count + per-GPU entries
# ---------------------------------------------------------------------------

def _decode_variable(data: bytes, off: int, s: dict) -> Tuple[dict, int]:
    # 1. Network interfaces
    num_ifaces, off = _get_u16(data, off)
    ifaces = []
    for _ in range(num_ifaces):
        name,     off = _get_str(data, off)
        rx_mbps,  off = _get_f32(data, off)
        tx_mbps,  off = _get_f32(data, off)
        rx_pps,   off = _get_f32(data, off)
        tx_pps,   off = _get_f32(data, off)
        rx_bytes, off = _get_u64(data, off)
        tx_bytes, off = _get_u64(data, off)
        rx_pkts,  off = _get_u64(data, off)
        tx_pkts,  off = _get_u64(data, off)
        rx_errs,  off = _get_u64(data, off)
        tx_errs,  off = _get_u64(data, off)
        rx_drop,  off = _get_u64(data, off)
        tx_drop,  off = _get_u64(data, off)
        ifaces.append({"name": name,
                        "rx_mbps": rx_mbps, "tx_mbps": tx_mbps,
                        "rx_pps": rx_pps, "tx_pps": tx_pps,
                        "rx_bytes": rx_bytes, "tx_bytes": tx_bytes,
                        "rx_pkts": rx_pkts, "tx_pkts": tx_pkts,
                        "rx_errs": rx_errs, "tx_errs": tx_errs,
                        "rx_drop": rx_drop, "tx_drop": tx_drop})
    s.setdefault("network", {})["ifaces"] = ifaces

    # 2. CPU sensors
    num_sensors, off = _get_u16(data, off)
    sensors = []
    for _ in range(num_sensors):
        sname, off = _get_str(data, off)
        sval,  off = _get_f32(data, off)
        sensors.append({"name": sname, "value": sval})
    s.setdefault("cpu", {})["sensors"] = sensors

    # 3. Disk devices (each has nested sub-sensor list)
    num_devs, off = _get_u16(data, off)
    devs = []
    for _ in range(num_devs):
        dname,      off = _get_str(data, off)
        reads_ps,   off = _get_f32(data, off)
        writes_ps,  off = _get_f32(data, off)
        read_bps,   off = _get_f32(data, off)
        write_bps,  off = _get_f32(data, off)
        util_pct,   off = _get_f32(data, off)
        temp,       off = _get_f32(data, off)
        num_ds,     off = _get_u16(data, off)   # nested DiskTempSensor count
        dsensors = []
        for _ in range(num_ds):
            dsname, off = _get_str(data, off)
            dsval,  off = _get_f32(data, off)
            dsensors.append({"name": dsname, "value": dsval})
        devs.append({"name": dname,
                     "reads_ps": reads_ps, "writes_ps": writes_ps,
                     "read_bps": read_bps, "write_bps": write_bps,
                     "util_pct": util_pct, "temp": temp,
                     "sensors": dsensors})
    s.setdefault("disks", {})["devices"] = devs

    # 4. Filesystems
    num_fs, off = _get_u16(data, off)
    filesystems = []
    for _ in range(num_fs):
        dev_name,  off = _get_str(data, off)
        mountpoint, off = _get_str(data, off)
        fstype,    off = _get_str(data, off)
        fs_total,  off = _get_u64(data, off)
        fs_used,   off = _get_u64(data, off)
        fs_avail,  off = _get_u64(data, off)
        fs_pct,    off = _get_f32(data, off)
        filesystems.append({"device": dev_name, "mountpoint": mountpoint,
                             "fstype": fstype,
                             "total": fs_total, "used": fs_used,
                             "available": fs_avail, "used_pct": fs_pct})
    s.setdefault("disks", {})["filesystems"] = filesystems

    # 5. System strings
    hostname,    off = _get_str(data, off)
    clock_source, off = _get_str(data, off)
    s.setdefault("system", {})["hostname"] = hostname
    s.setdefault("system", {})["clock_source"] = clock_source

    # 6. GPU entries
    num_gpus, off = _get_u16(data, off)
    gpus = []
    for _ in range(num_gpus):
        idx,          off = _get_u16(data, off)
        gname,        off = _get_str(data, off)
        driver,       off = _get_str(data, off)
        gtemp,        off = _get_f32(data, off)
        vram_used,    off = _get_u64(data, off)
        vram_total,   off = _get_u64(data, off)
        vram_pct,     off = _get_f32(data, off)
        load_pct,     off = _get_f32(data, off)
        power_w,      off = _get_f32(data, off)
        gpus.append({"index": idx, "name": gname, "driver": driver,
                     "temp": gtemp, "vram_used": vram_used,
                     "vram_total": vram_total, "vram_pct": vram_pct,
                     "load_pct": load_pct, "power_w": power_w})
    if gpus:
        s["gpu"] = gpus

    return s, off


# ---------------------------------------------------------------------------
# Top-level record decoder
# ---------------------------------------------------------------------------

def decode_v2_record(payload: bytes) -> Optional[dict]:
    """Decode a v2 binary AggregatedSample payload (no 4-byte length prefix)."""
    if len(payload) < 18:
        return None
    ts_ns  = struct.unpack_from("<q", payload, 0)[0]
    dur_ns = struct.unpack_from("<q", payload, 8)[0]
    flags  = struct.unpack_from("<H", payload, 16)[0]
    off = 18

    result: dict = {
        "timestamp": (datetime.datetime.fromtimestamp(ts_ns / 1e9).astimezone().isoformat()
                      if ts_ns > 0 else "(zero)"),
        "duration_ms": round(dur_ns / 1e6, 3),
        "flags": {
            "has_data": bool(flags & FLAG_HAS_DATA),
            "has_min":  bool(flags & FLAG_HAS_MIN),
            "has_max":  bool(flags & FLAG_HAS_MAX),
        },
    }

    for label, flag in [("data", FLAG_HAS_DATA), ("min", FLAG_HAS_MIN), ("max", FLAG_HAS_MAX)]:
        if flags & flag:
            block, off = _decode_fixed(payload, off)
            block, off = _decode_variable(payload, off, block)
            result[label] = block

    return result


# ---------------------------------------------------------------------------
# Ring-buffer reader
# ---------------------------------------------------------------------------

def find_latest_record(f: BinaryIO, wrapped: bool, write_off: int, max_data: int) -> Optional[bytes]:
    segments = []
    if wrapped:
        segments.append((write_off, max_data - write_off))
        segments.append((0, write_off))
    else:
        segments.append((0, write_off))

    last_payload: Optional[bytes] = None
    for start, size in segments:
        f.seek(HEADER_SIZE + start)
        bytes_read = 0
        while bytes_read < size:
            if size - bytes_read < 4:
                break
            len_buf = f.read(4)
            if len(len_buf) < 4:
                break
            data_len = struct.unpack("<I", len_buf)[0]
            if data_len == 0 or data_len > max_data:
                break
            record_len = 4 + data_len
            if bytes_read + record_len > size:
                break
            payload = f.read(data_len)
            if len(payload) < data_len:
                break
            last_payload = payload
            bytes_read += record_len
    return last_payload


# ---------------------------------------------------------------------------
# Pretty printer
# ---------------------------------------------------------------------------

def print_record(payload: bytes, codec_ver: int) -> None:
    if codec_ver >= CODEC_V2:
        try:
            parsed = decode_v2_record(payload)
            print("\nLatest Record (v2 binary):")
            print(json.dumps(parsed, indent=2))
        except Exception as exc:
            print(f"\nLatest Record (v2 binary, decode error: {exc}): {payload[:32]!r}…")
    else:
        try:
            parsed = json.loads(payload.decode("utf-8"))
            print("\nLatest Record (v1 JSON):")
            print(json.dumps(parsed, indent=2))
        except (json.JSONDecodeError, UnicodeDecodeError):
            print(f"\nLatest Record (v1 JSON, decode failed): {payload!r}")


# ---------------------------------------------------------------------------
# Main inspector
# ---------------------------------------------------------------------------

def inspect_tier(filepath: str) -> None:
    try:
        file_size = os.path.getsize(filepath)
        with open(filepath, "rb") as f:
            buf = f.read(HEADER_SIZE)
            if len(buf) < HEADER_SIZE:
                print(f"Error: File too small ({len(buf)} bytes, expected {HEADER_SIZE})", file=sys.stderr)
                sys.exit(1)

            magic, codec_ver, max_data, write_off, count, oldest_nano, newest_nano = parse_header(buf)

            if magic != MAGIC:
                print(f"Error: Invalid magic: {magic.decode('utf-8', errors='replace')}", file=sys.stderr)
                sys.exit(1)

            wrapped = write_off > 0 and count > 0 and file_size >= HEADER_SIZE + max_data
            codec_label = f"v2 binary" if codec_ver >= CODEC_V2 else f"v1 JSON (legacy)"

            print(f"File:          {filepath}")
            print(f"Codec:         {codec_label} (version={codec_ver})")
            current_data = max_data if wrapped else write_off
            pct = (current_data / max_data * 100) if max_data > 0 else 0.0
            print(f"Data Size:     {current_data:,} / {max_data:,} bytes ({pct:.2f}%)")
            print(f"Write Offset:  {write_off:,}")
            print(f"Total Records: {count:,}")
            oldest_ts = (datetime.datetime.fromtimestamp(oldest_nano / 1e9).astimezone()
                         if oldest_nano > 0 else None)
            newest_ts = (datetime.datetime.fromtimestamp(newest_nano / 1e9).astimezone()
                         if newest_nano > 0 else None)
            print(f"Oldest:        {oldest_ts.isoformat() if oldest_ts else '(none)'}")
            print(f"Newest:        {newest_ts.isoformat() if newest_ts else '(none)'}")
            print(f"Wrapped:       {wrapped}")
            if oldest_ts and newest_ts:
                print(f"Time Covered:  {newest_ts - oldest_ts}")

            if count == 0:
                print("\nLatest Record: (none)")
                return

            payload = find_latest_record(f, wrapped, write_off, max_data)
            if payload:
                print_record(payload, codec_ver)
            else:
                print("\nLatest Record: (none found)")

    except OSError as err:
        print(f"Error inspecting tier file: {err}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python inspect_tier.py <path-to-tier-file>", file=sys.stderr)
        sys.exit(1)
    inspect_tier(sys.argv[1])
