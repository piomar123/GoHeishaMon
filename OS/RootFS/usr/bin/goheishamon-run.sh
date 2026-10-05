#!/bin/sh
# Runs GoHeishaMon once; /etc/init.d/goheishamon (procd) respawns it.
# stdout and stderr (incl. Go panics) go to syslog with the "goheisha" tag
# and to the ttyS0 console.
# After an abnormal exit the whole syslog buffer is saved to RAM, because the
# small logd ring buffer gets overwritten by the next run within minutes.

BIN=${1:-/usr/bin/GoHeishaMon_MIPSUPX}
CRASH_DIR=/tmp/goheishamon-crashes
KEEP=10
FIFO=/tmp/goheishamon.$$.fifo

stopping=0
pid=
on_term() {
	stopping=1
	[ -n "$pid" ] && kill "$pid" 2>/dev/null
}
trap on_term TERM INT

rm -f "$FIFO"
mkfifo "$FIFO" || exit 1
# Also copy to the ttyS0 console: the bottom LED (check_buttons.sh, gpio10 ->
# gpio3) follows that line, so it blinks while GoHeishaMon logs heat-pump data
tee /dev/ttyS0 < "$FIFO" | logger -t goheisha &
logger_pid=$!
"$BIN" > "$FIFO" 2>&1 &
pid=$!

wait "$pid"
rc=$?
if [ "$stopping" = 1 ]; then
	# wait was interrupted by the signal, wait for GoHeishaMon to exit
	wait "$pid"
	wait "$logger_pid"
	rm -f "$FIFO"
	exit 0
fi

wait "$logger_pid" # flush the last lines to syslog
rm -f "$FIFO"
logger -t goheishamon-run "GoHeishaMon exited with code $rc"

if [ "$rc" -ne 0 ]; then
	mkdir -p "$CRASH_DIR"
	f="$CRASH_DIR/$(date +%Y%m%d-%H%M%S)-exit$rc.log"
	{
		echo "exit code: $rc"
		echo "uptime: $(cat /proc/uptime)"
		logread
	} > "$f"
	logger -t goheishamon-run "log saved to $f"

	n=0
	for old in $(ls -1t "$CRASH_DIR"); do
		n=$((n + 1))
		[ "$n" -gt "$KEEP" ] && rm -f "$CRASH_DIR/$old"
	done
fi

exit "$rc"
