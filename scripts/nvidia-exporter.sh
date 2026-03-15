#!/bin/bash
# Kula NVIDIA Exporter
# Periodically saves NVIDIA GPU metrics to a file for Kula to read safely.

umask 077

STORAGE_DIR=${1:-"/var/lib/kula"}
INTERVAL=${2:-1}

# 1. Path Validation
if ! command -v nvidia-smi &> /dev/null; then
    echo "Error: nvidia-smi not found"
    exit 1
fi

mkdir -p "$STORAGE_DIR" 2>/dev/null || { echo "Error: cannot create $STORAGE_DIR"; exit 1; }
touch "$STORAGE_DIR/.write-test" 2>/dev/null || { echo "Error: $STORAGE_DIR is not writable"; exit 1; }
rm -f "$STORAGE_DIR/.write-test"

# 2. Locking (prevents multiple instances writing to the same log)
LOCKFILE="$STORAGE_DIR/nvidia-exporter.lock"
exec 9>"$LOCKFILE"
flock -n 9 || { echo "Error: another instance is already running"; exit 1; }

# 3. Cleanup on exit
cleanup() {
    echo -e "\nShutting down..."
    rm -f "$STORAGE_DIR/nvidia.log" "$STORAGE_DIR/nvidia.log.tmp" "$LOCKFILE"
    exit 0
}
trap cleanup SIGTERM SIGINT

echo "Kula NVIDIA Exporter started. Writing to $STORAGE_DIR/nvidia.log every ${INTERVAL}s"

while true; do
    # Query format: pci.bus_id, temperature.gpu, utilization.gpu, memory.used, memory.total, power.draw
    # We use a tmp file and atomic mv to prevent Kula from reading partial data.
    nvidia-smi --query-gpu=pci.bus_id,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw --format=csv,noheader,nounits > "$STORAGE_DIR/nvidia.log.tmp" 2>/dev/null
    
    if [ $? -eq 0 ]; then
        mv "$STORAGE_DIR/nvidia.log.tmp" "$STORAGE_DIR/nvidia.log"
    fi

    sleep "$INTERVAL"
done
