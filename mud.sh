    #!/bin/bash
# service.sh: Start, stop, or restart the WillowdaleMUD program

# Define variables
DIR="/Users/jens/mud/willowdale/"
PROGRAM="WillowdaleMUD"
PIDFILE="WillowdaleMUD.pid"

# Set environment variables
export LOG_LEVEL=LOW
export LOG_PATH="/Users/jens/mud/willowdale/logs/log.txt"
export CONFIG_PATH="/Users/jens/mud/willowdale/_datafiles/willowdale-config.yaml"

# Change to the program directory
cd "$DIR" || { echo "Directory not found: $DIR"; exit 1; }

# Function to start the program
start_program() {
    # Check if the program executable exists, build it if not
    if [ ! -f "$PROGRAM" ]; then
        echo "$PROGRAM executable not found. Building first..."
        compile_program
    fi

    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            echo "$PROGRAM is already running with PID $PID."
            exit 0
        else
            echo "Stale PID file found. Removing."
            rm "$PIDFILE"
        fi
    fi

    echo "Starting $PROGRAM..."
    ./"$PROGRAM" &
    echo $! > "$PIDFILE"
    echo "$PROGRAM started with PID $(cat $PIDFILE)"
}

# Function to stop the program
stop_program() {
    if [ ! -f "$PIDFILE" ]; then
        echo "No PID file found. Is $PROGRAM running?"
        exit 1
    fi

    PID=$(cat "$PIDFILE")
    echo "Stopping $PROGRAM with PID $PID..."
    kill "$PID"
    sleep 2
    if ps -p "$PID" > /dev/null 2>&1; then
        echo "$PROGRAM did not stop gracefully; forcing termination..."
        kill -9 "$PID"
    fi
    rm "$PIDFILE"
    echo "$PROGRAM stopped."
}

# Function to compile the program
compile_program() {
    echo "Compiling $PROGRAM..."
    go build -o "$PROGRAM"
    if [ $? -ne 0 ]; then
        echo "Build failed! Aborting restart."
        exit 1
    fi
    echo "Compilation successful."
}

# Function to restart the program
restart_program() {
    stop_program
    compile_program
    start_program
}

# Main: process command-line argument
case "$1" in
    start)
        start_program
        ;;
    stop)
        stop_program
        ;;
    restart)
        restart_program
        ;;
    *)
        echo "Usage: $0 {start|stop|restart}"
        exit 1
        ;;
esac