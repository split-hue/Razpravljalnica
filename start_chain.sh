#!/bin/bash

# ANSI barve
RESET="\033[0m"
BOLD="\033[1m"

RED="\033[31m"
GREEN="\033[32m"
PINK="\033[95m"
BLUE="\033[34m"
MAGENTA="\033[35m"
CYAN="\033[36m"
GRAY="\033[90m"

# Skripta za zagon chain replication klastra
# Uporaba: ./start_chain.sh [število_vozlišč]

NUM_NODES=${1:-3}

if [ "$NUM_NODES" -lt 2 ]; then
    echo -e "${RED}Potrebna so vsaj 2 vozlišča (HEAD in TAIL)${RESET}"
    exit 1
fi

echo -e "${CYAN}${BOLD}============================================================${RESET}"
echo -e "${CYAN}${BOLD}      RAZPRAVLJALNICA - CHAIN REPLICATION STARTUP          ${RESET}"
echo -e "${CYAN}${BOLD}============================================================${RESET}"
echo ""
echo -e "Starting ${BOLD}$NUM_NODES${RESET} node(s)..."
echo ""

# Osnovni port
BASE_PORT=9876

# Pripravi seznam vseh naslovov
ALL_ADDRS=""
for i in $(seq 0 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))
    if [ $i -eq 0 ]; then
        ALL_ADDRS="localhost:$PORT"
    else
        ALL_ADDRS="$ALL_ADDRS,localhost:$PORT"
    fi
done

echo -e "${MAGENTA}Topology:${RESET} $ALL_ADDRS"
echo ""

# Preveri, ali so proto datoteke prevedene
if [ ! -d "razpravljalnica" ]; then
    echo -e "${PINK}Compiling proto files...${RESET}"
    make proto
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to compile proto files!${RESET}"
        exit 1
    fi
fi

# Zaženi vozlišča
PIDS=()
LOG_FILES=()

for i in $(seq 0 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))

    # Določi vlogo
    if [ $i -eq 0 ]; then
        ROLE="head"
        ROLE_STR="HEAD"
    elif [ $i -eq $((NUM_NODES-1)) ]; then
        ROLE="tail"
        ROLE_STR="TAIL"
    else
        ROLE="intermediate"
        ROLE_STR="INTERMEDIATE"
    fi

    # Določi naslednika (če ni TAIL)
    SUCCESSOR=""
    if [ $i -lt $((NUM_NODES-1)) ]; then
        NEXT_PORT=$((BASE_PORT + i + 1))
        SUCCESSOR="-successor localhost:$NEXT_PORT"
    fi

    LOG_FILE="node_${i}_${ROLE}.log"
    LOG_FILES+=("$LOG_FILE")

    echo -e "${BLUE}Starting node $((i+1))/$NUM_NODES [$ROLE_STR] on port $PORT...${RESET}"

    go run . -role "$ROLE" -p "$PORT" $SUCCESSOR -all "$ALL_ADDRS" > "$LOG_FILE" 2>&1 &
    PID=$!
    PIDS+=($PID)

    echo -e "   PID: ${GREEN}$PID${RESET}, Log: $LOG_FILE"

    sleep 1
done

echo ""
echo -e "${GREEN}All nodes started!${RESET}"
echo ""

echo -e "${PINK}Waiting for chain to initialize...${RESET}"
sleep 3

# Preveri, ali vsa vozlišča še tečejo
ALL_RUNNING=true
for PID in "${PIDS[@]}"; do
    if ! kill -0 $PID 2>/dev/null; then
        echo -e "${RED}Node with PID $PID has crashed!${RESET}"
        ALL_RUNNING=false
    fi
done

if [ "$ALL_RUNNING" = false ]; then
    echo ""
    echo -e "${RED}Some nodes failed to start. Check log files:${RESET}"
    for LOG in "${LOG_FILES[@]}"; do
        echo "   - $LOG"
    done

    for PID in "${PIDS[@]}"; do
        kill $PID 2>/dev/null
    done

    exit 1
fi

echo -e "${GREEN}Chain initialized successfully!${RESET}"
echo ""
echo -e "${CYAN}${BOLD}======================== CLUSTER READY =======================${RESET}"
echo ""
echo "HEAD node: localhost:$BASE_PORT"
echo "TAIL node: localhost:$((BASE_PORT + NUM_NODES - 1))"
echo ""
echo "Node PIDs: ${PIDS[*]}"
echo "Log files: ${LOG_FILES[*]}"
echo ""
echo "--------------------------------------------------------------"
echo "To test:"
echo "  go run . -head localhost:$BASE_PORT -tail localhost:$((BASE_PORT + NUM_NODES - 1)) -test"
echo ""
echo "To run interactive client:"
echo "  go run . -head localhost:$BASE_PORT -tail localhost:$((BASE_PORT + NUM_NODES - 1))"
echo ""
echo "To view logs:"
echo "  tail -f ${LOG_FILES[0]}"
echo ""
echo "To stop all nodes:"
echo "  kill ${PIDS[*]}"
echo "--------------------------------------------------------------"
echo ""

# Cleanup funkcija
cleanup() {
    echo ""
    echo -e "${YELLOW}Stopping all nodes...${RESET}"
    for PID in "${PIDS[@]}"; do
        kill $PID 2>/dev/null
    done
    echo -e "${GREEN}All nodes stopped${RESET}"

    read -p "Delete log files? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -f "${LOG_FILES[@]}"
        echo -e "${GREEN}Log files deleted${RESET}"
    fi

    exit 0
}

trap cleanup INT TERM

if [ "$2" = "-test" ]; then
    echo "Running automatic test..."
    sleep 2
    go run . -head localhost:$BASE_PORT -tail localhost:$((BASE_PORT + NUM_NODES - 1)) -test
    cleanup
else
    echo "Press Ctrl+C to stop all nodes..."
    wait
fi