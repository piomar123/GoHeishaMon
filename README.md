# GoHeishaMon in overlayfs

**This branch uses overlayfs (jffs2) to store GoHeishaMon executable.** 
That way you can easily update the app without reflashing whole firmware. 
For that to work, before firmware flashing, the executable `GoHeishaMon_MIPSUPX` 
**needs to be placed in the root directory of the USB stick** and will be copied 
right after the first boot to the overlayfs. (This is to be able to activate dropbear SSH 
through MQQT command as I tried different approaches to run it but nothing else worked :/ ). 

For testing and frequent updates, it's recommended to use RAM `/tmp` partition to store 
the GoHeishaMon exec to give the Flash memory some relief.


## CZ-TAW1/CZ-TAW1B

This project is to modify Panasonic CZ-TAW1 Firmware to send data from heat pump to MQTT instead to
Aquarea Cloud (there is some POC work proving there is a posiblity to send data concurently to
Aquarea Cloud and MQTT host using only modified CZ-TAW1 ,but it's not yet implemented in this
project )

## This Project Contains

- Main software (called GoHeishaMon) responsible for parsing data from Heat Pump - it's golang
  implementation of project https://github.com/Egyras/HeishaMon All MQTT topics are compatible with
  HeishaMon project: https://github.com/Egyras/HeishaMon/blob/master/MQTT-Topics.md and there are
  two aditional topics to run command's in system runing the software but it need's another manual.

GoHeishaMon can be used without the CZ-TAW1 module on every platform supported by golang
(RaspberyPi, Windows, Linux, OpenWrt routers for example) after connecting it to Heat Pump over
rs232-ttl interface. If you need help with this project you can try Slack of Heishamon project there
is some people who manage this one :)

- OpenWRT Image with preinstalled GoHeishaMon (and removed A2Wmain due to copyright issues)

CZ-TAW1 flash memory is divided for two parts called "sides". During Smart Cloud update A2Wmain
software programing other side then actually it boots ,and change the side just before reboot. In
this way, normally in CZ-TAW1 there are two versions of firmware: actual and previous. Updating
firmware with GoHeishaMon we use one side , and we can very easly change the side to Smart Cloud
(A2Wmain software) by pressing all three buttons on CZ-TAW1 when GoHeishaMon works ( middle LED will
change the color to RED and shortly after this it reboots to orginal SmartCloud). Unfortunatly from
Smart Cloud software changing the side without having acces to ssh console is possible only when
updating other side was take place succesfully.

Summary:

It is possible to go back to orginal software (A2Wmain with SmartCloud) very quick, without
preparing pendrive, because this solution doesn't remove firmware with A2Wmain (it is still on the other
"Side" in the flash).

Even though the GoHeishaMon is on other side you can't just change the side in orginal software to
GoHeishaMon without an access to the console. You have to install GoHeishaMon again.

## WiFi configuration

WiFi should be configured in original firmware.

### Setting up WiFi Without WPS on CZ-TAW1

In the paper instructions that come with the CZ-TAW1, there's no mention of setting up WiFi without
WPS. However, in the PDF instructions found on the CD-ROM included with the device and various
online manuals, you'll find a procedure for configuring WiFi settings without using WPS.

1. **Using the HTML Utility**: The CD-ROM contains a small HTML utility that simplifies the process
   of configuring WiFi settings. This utility allows you to enter your WiFi SSID and password, which
   it then saves in a `settings.txt` file for you.

2. **USB Drive Preparation**: To proceed, insert a USB drive into your computer. We recommend using
   an 8GB FAT32-formatted USB drive (although your mileage may vary with other configurations).

3. **Create `settings.txt`**: In the utility, you'll specify your WiFi settings. The `settings.txt`
   file should have the following content (without the quotes, and there should be a newline after
   each key):

   ```plaintext
   SSID=YourSSIDHere
   KEY=APasswordBetterThanThis
   ```

4. **Transfer `settings.txt`**: Save the `settings.txt` file to the root directory of your USB
   drive.

5. **WiFi Configuration**: With the `settings.txt` file on the USB drive, insert it into the
   CZ-TAW1.

6. **Configure WiFi**: To configure the WiFi settings, press and hold the WPS button on the CZ-TAW1
   for 10 seconds. The device will read the `settings.txt` file and set up the WiFi accordingly.

7. **Final Steps**: Once the WiFi configuration is complete, you can remove the USB drive and
   install the transmitter wherever you prefer.

These steps allow you to set up your WiFi on the CZ-TAW1 without the need for WPS.

## Install instructions

To install the software, follow these steps:

1. Format a USB drive to FAT32 and copy the following files to it:

   - `openwrt-ar71xx-generic-cus531-16M-kernel.bin`
   - `openwrt-ar71xx-generic-cus531-16M-rootfs-squashfs.bin`
   - `GoHeishaMon_MIPSUPX`

2. Additionally, configure and copy the file named `GoHeishaMonConfig.new`.

3. Insert the USB drive with these files into your CZ-TAW1 device.

4. Press all three buttons simultaneously and hold them for more than 10 seconds. Wait until the
   middle LED on the CZ-TAW1 begins changing colors, cycling through green, blue, and red. You may
   also notice the LED on the USB drive blinking if it has one.

5. The update process will start, and it will take approximately 3 minutes. During this time, the
   CZ-TAW1 will reboot. After a while, you will see the middle LED light up in white.

6. Do not remove the drive from the module until the white LED turns off again. This indicates that
   the GoHeishaMon has copied the config file from the drive and rebooted the CZ-TAW1. Remove the
   drive before the white LED turns on again, as leaving the drive with the config file present will
   result in it being copied again and triggering another reboot.

## Updating GoHeishaMon in the overlay

The overlay is a ~2 MB JFFS2 partition and the binary is ~1.7 MB, so the old and the new binary
don't fit side by side, and `df` can't be trusted right after deleting a file. Test a new binary
from `/tmp` (RAM) first, e.g. `/usr/bin/goheishamon-run.sh /tmp/GoHeishaMon_MIPSUPX` with the
service stopped, then:

```bash
/etc/init.d/goheishamon stop; sleep 2; ps | grep -i '[h]eish'   # nothing may hold the old file
cp /usr/bin/GoHeishaMon_MIPSUPX /tmp/GoHeishaMon_MIPSUPX.old      # backup in RAM (also keep a copy off the device)
rm /usr/bin/GoHeishaMon_MIPSUPX && sync
kill -HUP $(pidof jffs2_gcd_mtd3); sleep 30; df /overlay          # let the JFFS2 GC erase the freed blocks
cp /tmp/GoHeishaMon_MIPSUPX /usr/bin/.GoHeishaMon_MIPSUPX.new && sync
md5sum /tmp/GoHeishaMon_MIPSUPX /usr/bin/.GoHeishaMon_MIPSUPX.new  # must match
mv /usr/bin/.GoHeishaMon_MIPSUPX.new /usr/bin/GoHeishaMon_MIPSUPX && chmod +x /usr/bin/GoHeishaMon_MIPSUPX && sync
/etc/init.d/goheishamon start
```

Don't `cp` over the old file: until the copy completes, the old data is still live, so both
versions need space at once. If `cp` fails with "No space left on device", remove the partial
file, don't reboot (there is no binary in the overlay now), and run the binary from `/tmp` until
it's sorted out.

**About the JFFS2 garbage collector.** Deleted data isn't freed at once: its flash blocks become
*dirty* and are only reusable after the GC thread (`jffs2_gcd_mtd3` for the overlay on mtd3) has
erased them. `df` already counts dirty space as available (`avail = dirty_size + free_size` in
`jffs2_statfs()`), so it shows the space before it is really free. The kernel wakes the GC thread
with SIGHUP itself (`jffs2_garbage_collect_trigger()`), e.g. when a block becomes completely
obsolete and waits to be erased, and `kill -HUP` from userspace makes it run one extra GC pass
(see `fs/jffs2/background.c`). This isn't a documented interface (`Documentation/` doesn't
mention it); it's what the kernel source does, and is harmless. Writes also run GC themselves
when they run out of free blocks, so the signal and the wait mostly make the copy faster and
less likely to hit the reserved blocks. The overlay is mtd3 (`rootfs_data`, 2176 KB in 64 KB
erase blocks) on this board; check with `cat /proc/mtd` and `ps | grep jffs2_gcd`.

## Board Functionality: Buttons and LEDs

### Buttons

- **WPS Button**: This button does not have a specific function and remains inactive.

- **Reset Button**: Pressing the reset button will restart the GoHeishaMon application.

- **Check Button**: The check button does not trigger any specific action.

- **Simultaneous Button Press**: When all buttons are pressed together, the board will switch back to its original firmware.

### LEDs

- **Top LED**: This LED, illuminated in green, indicates whether the operating system (OS) is currently running.

- **Mid LED**: The white light emitted from this LED signifies that the GoHeishaMon application is in operation.

- **Bottom LED**: Green, blinks (1 s on, 1 s off) while valid packets arrive from the heat pump,
  off when none came for more than 3 s. GoHeishaMon touches `/tmp/goheishamon.packet` on every
  packet (needs a GoHeishaMon build with that, e.g. piomar123 fix/pando/races-and-deps d4d3d3f+).

## Configuration

### SSH Connection

SSH was not working and dropbear doesn't started automatically. The solution was to start it through
MQTT messages. Topic for sending messages: `panasonic_heat_pump/commands/OSCommand` Topic for
reading output: `panasonic_heat_pump/commands/OSCommand/out`

Just send a `/usr/sbin/dropbear` command. Example:

```bash
mosquitto_pub -t "panasonic_heat_pump/commands/OSCommand" -m "/usr/sbin/dropbear" -h <MQTT BROKER IP>
```

For connecting to SSH, weaker algorithms are needed:

```bash
ssh -oHostkeyAlgorithms=+ssh-rsa -oKexAlgorithms=+diffie-hellman-group1-sha1 root@${PANASONIC_IP}
```

Default root password: `GoHeishaMonpass`. It's recommended to change it by command:

```bash
/root/pass.sh YourNewPassword
```

### Changing hostname

`/etc/config/system`

```bash
uci set system.@system[0].hostname='cz-taw1b'
uci commit system
/etc/init.d/system reload
```

### Fix dropbear

```bash
root@cz-taw1b:~# cat /etc/config/dropbear
config dropbear
    option PasswordAuth 'on'
    option RootPasswordAuth 'on'
    option Port            '22'
#    option BannerFile    '/etc/banner'
```

Set `~/.ssh/config`:

```conf
Host cz-taw1b
    User root
    PubkeyAcceptedKeyTypes +ssh-rsa
    HostkeyAlgorithms +ssh-rsa
    KexAlgorithms +diffie-hellman-group1-sha1
```

Add rsa key to `/etc/dropbear/authorized_keys` or use LuCi web UI.

### Configure NTP

Change NTP servers to your preferred ones.

Screenshot from Homeassistant: ![Screenshot from Homeassistant](PompaCieplaScreen.PNG)

## TODO

- queue command from a2wmain
- flag to point to config file
- manuals
- tests

..... more....
