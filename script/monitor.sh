#!/bin/bash
# tinycode performance monitor — run in a separate terminal
# Usage: ./script/monitor.sh

OLLAMA_HOST="${TINYCODE_OLLAMA_HOST:-http://localhost:11434}"

while true; do
  clear
  echo "=== tinycode Monitor $(date +%H:%M:%S) ==="
  echo ""

  # Model status
  PS_JSON=$(curl -sf "${OLLAMA_HOST}/api/ps" 2>/dev/null)
  if [ -n "$PS_JSON" ]; then
    MODEL=$(echo "$PS_JSON" | grep -oE '"name":"[^"]*"' | head -1 | sed 's/"name":"//;s/"//')
    VRAM=$(echo "$PS_JSON" | grep -oE '"size_vram":[0-9]+' | head -1 | sed 's/"size_vram"://')
    EXPIRES=$(echo "$PS_JSON" | grep -oE '"expires_at":"[^"]*"' | head -1 | sed 's/"expires_at":"//;s/"//')
    if [ -n "$MODEL" ]; then
      VRAM_GB=$(echo "scale=1; ${VRAM:-0} / 1000000000" | bc 2>/dev/null || echo "?")
      echo "Model: $MODEL (${VRAM_GB}GB VRAM)"
      echo "Expires: $EXPIRES"
    else
      echo "Model: NOT LOADED (next request will cold-start)"
    fi
  else
    echo "Ollama: NOT RESPONDING"
  fi

  echo ""

  # Memory
  FREE_PAGES=$(vm_stat 2>/dev/null | awk '/Pages free/ {gsub(/\./,"",$3); print $3}')
  INACTIVE=$(vm_stat 2>/dev/null | awk '/Pages inactive/ {gsub(/\./,"",$3); print $3}')
  PAGEOUTS=$(vm_stat 2>/dev/null | awk '/Pageouts/ {gsub(/\./,"",$2); print $2}')
  if [ -n "$FREE_PAGES" ]; then
    FREE_MB=$(( (FREE_PAGES + INACTIVE) * 4096 / 1048576 ))
    echo "Free RAM: ${FREE_MB}MB  Pageouts: $PAGEOUTS"
  fi

  SWAP=$(sysctl vm.swapusage 2>/dev/null | grep -oE "used = [0-9.]+" | grep -oE "[0-9.]+")
  if [ -n "$SWAP" ]; then
    echo "Swap used: ${SWAP}MB"
  fi

  echo ""

  # tinycode process
  TC_PID=$(pgrep -f "bun.*tinycode.*src/index" | head -1)
  if [ -n "$TC_PID" ]; then
    TC_MEM=$(ps -o rss= -p "$TC_PID" 2>/dev/null | awk '{printf "%.0f", $1/1024}')
    echo "tinycode PID: $TC_PID (${TC_MEM}MB)"
  else
    echo "tinycode: not running"
  fi

  echo ""
  echo "--- Press Ctrl+C to stop ---"
  sleep 5
done
