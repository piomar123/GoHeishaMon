#!/bin/ash

GOHEISHAMON_BIN=/usr/bin/GoHeishaMon_MIPSUPX

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

while true; do
    # press == `hi`
    ButtonReset=`awk '/gpio-0 /{print $5}' /sys/kernel/debug/gpio`
    # press == `hi`
    ButtonWPS=`awk '/gpio-1 /{print $5}' /sys/kernel/debug/gpio`
    # press == `lo`
    ButtonCheck=`awk '/gpio-16 /{print $5}' /sys/kernel/debug/gpio`
    # Pin for communication by serial port
    CNCNTLink=`awk '/gpio-10 /{print $5}' /sys/kernel/debug/gpio`

    # GoHeishaMon running
    if [ $(ps | grep "$GOHEISHAMON_BIN" | wc -l) -gt 1 ]; then
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
        kill $(ps | grep "$GOHEISHAMON_BIN" | head -n1 | awk '{ print $1 }')
        $GOHEISHAMON_BIN | tee /dev/ttyS0 | logger -t goheisha &
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

    if [ "$CNCNTLink" = 'hi' ] ; then
        echo low > /sys/class/gpio/gpio3/direction
    fi
    if [ "$CNCNTLink" = 'lo' ] ; then
        echo high > /sys/class/gpio/gpio3/direction
    fi

    sleep 1
done

exit 0
