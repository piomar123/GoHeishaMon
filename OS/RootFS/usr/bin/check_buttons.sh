#!/bin/ash

GOHEISHAMON_BIN=/usr/bin/GoHeishaMon_MIPSUPX
# GoHeishaMon sets its mtime on every valid packet from the heat pump
HEARTBEAT=/tmp/goheishamon.packet

logger -t check_buttons.sh "Init GPIOs"

# LED
echo 2 > /sys/class/gpio/export  # middle blue
echo 3 > /sys/class/gpio/export  # bottom green
echo 13 > /sys/class/gpio/export  # middle green
echo 15 > /sys/class/gpio/export  # middle red

# link
echo 10 > /sys/class/gpio/export

# buttons
echo 0 > /sys/class/gpio/export
echo 1 > /sys/class/gpio/export
echo 16 > /sys/class/gpio/export

# initial purple LED
echo high > /sys/class/gpio/gpio2/direction
echo low > /sys/class/gpio/gpio13/direction
echo high > /sys/class/gpio/gpio15/direction

sleep 1

bottom_led=0
while true; do
    # press == `hi`
    ButtonReset=`awk '/gpio-0 /{print $5}' /sys/kernel/debug/gpio`
    # press == `hi`
    ButtonWPS=`awk '/gpio-1 /{print $5}' /sys/kernel/debug/gpio`
    # press == `lo`
    ButtonCheck=`awk '/gpio-16 /{print $5}' /sys/kernel/debug/gpio`

    # GoHeishaMon running (by name, so it also works when testing a binary from /tmp;
    # `ps | grep` raced with grep matching itself and made the LED blink)
    if pidof "$(basename "$GOHEISHAMON_BIN")" > /dev/null; then
        # white LED
        echo high > /sys/class/gpio/gpio2/direction
        echo high > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
    else
        # off LED
        echo low > /sys/class/gpio/gpio2/direction
        echo low > /sys/class/gpio/gpio13/direction
        echo low > /sys/class/gpio/gpio15/direction
    fi

    if [ "$ButtonReset" = 'hi' ] && [ "$ButtonWPS" = 'lo' ] && [ "$ButtonCheck" = 'hi' ] ; then
        # yellow LED
        echo low > /sys/class/gpio/gpio2/direction
        echo high > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
        logger -t check_buttons.sh "Restart GoHeishaMon"
        /etc/init.d/goheishamon restart
    fi

    # fw side switch
    if [ "$ButtonReset" = 'hi' ] && [ "$ButtonWPS" = 'hi' ] && [ "$ButtonCheck" = 'lo' ] ; then
        # red LED
        echo low > /sys/class/gpio/gpio2/direction
        echo low > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
        fwupdate sw > /dev/null 2>&1
        sync
        reboot
    fi

    # Bottom LED blinks (1 s on, 1 s off: one loop each) while heat-pump packets
    # arrive and is off when none came for more than 3 s. It used to mirror
    # gpio10, which follows the ttyS0 console, not the heat-pump link (ttyUSB0).
    heartbeat=$(date -r "$HEARTBEAT" +%s 2>/dev/null)
    if [ -n "$heartbeat" ] && [ $(( $(date +%s) - heartbeat )) -le 3 ]; then
        bottom_led=$((1 - bottom_led))
    else
        bottom_led=0
    fi
    if [ "$bottom_led" = 1 ]; then
        echo high > /sys/class/gpio/gpio3/direction
    else
        echo low > /sys/class/gpio/gpio3/direction
    fi

    sleep 1
done

exit 0
