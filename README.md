# What is this fork about

Trying to join [lsochanowski](https://github.com/lsochanowski/GoHeishaMon) and [rondoval](https://github.com/rondoval/GoHeishaMon) efforts into one:

* [ ] keep simplicity of firmware modification with sysupgrade from USB (lsochanowski)
* [ ] use new sensors and bugfixes from rondoval
* [ ] adjust GoHeishaMon to be more compatible with HeishaMon:
    * [x] raw decoded values
    * [ ] handle [Set command topics](https://github.com/Egyras/HeishaMon/blob/master/MQTT-Topics.md#command-topics)
* [ ] (if possible) upgrade OpenWRT to a newer version (while keeping two-sided memory layout)


# What's running on my Aquarea (as of 2026-10-09)

* **Firmware:** lsochanowski-based firmware from
  [wip/lsochanowski](https://github.com/piomar123/GoHeishaMon/tree/wip/lsochanowski)
  (8c1efcf) - the app binary lives in overlayfs (`/usr/bin/GoHeishaMon_MIPSUPX`) instead of ROM,
  so it can be replaced without reflashing. The overlay is only ~2 MB, so the UPX-packed binary
  must stay below ~1.8 MB.
* **Service scripts (overlay):** from
  [feat/lsochanowski/procd-service](https://github.com/piomar123/GoHeishaMon/tree/feat/lsochanowski/procd-service)
  at efad4c3 - GoHeishaMon runs as a procd service (`/etc/init.d/goheishamon`, started from
  rc.local), respawned 30 s after it exits; after an abnormal exit the syslog is saved to
  `/tmp/goheishamon-crashes/`. Logs: `logread | grep goheisha`. `check_buttons.sh` blinks the
  bottom LED (1 s on, 1 s off) while packets arrive from the heat pump.
* **App (overlay, since 2026-10-09):**
  [fix/pando/races-and-deps](https://github.com/piomar123/GoHeishaMon/tree/fix/pando/races-and-deps)
  at d4d3d3f - newest pando (1.2.0 + HA template fix) with data race fixes (the previous app
  crashed with memory corruption after ~56 h), updated dependencies, built with go1.24 +
  UPX 4.2.4, plus a heartbeat file (`/tmp/goheishamon.packet`) touched on every heat-pump packet.
  The same code without the heartbeat (19f178d) ran 7 days from RAM without a crash.
* **Previous app:** [pando85/GoHeishaMon](https://github.com/pando85/GoHeishaMon) at `aec71c1`
  (before the serial rewrite in 1.2.0), built with the newer dependencies from wip/lsochanowski
  (867e7c4) - a local build that was never committed.

# lsochanowski/GoHeishaMon

https://github.com/lsochanowski/GoHeishaMon

This repository with my recent changes is in on 
[wip/lsochanowski](https://github.com/piomar123/GoHeishaMon/tree/wip/lsochanowski) branch.
I'm using the firmware from that branch (see above for the app version)
with [NodeRed_Heishamon_control dashboard](https://github.com/edterbak/NodeRed_Heishamon_control).

The original repository [lsochanowski/GoHeishaMon](https://github.com/lsochanowski/GoHeishaMon) is outdated 
and seems abandoned but includes a neat way to modify the firmware while keeping the original firmware 
on the other memory side.

# rondoval/GoHeishaMon

https://github.com/rondoval/GoHeishaMon

This fork contains a completely rewritten GoHeishaMon app with a different approach to reading and writing settings to Panasonic heat pumps.
It's not working correctly with [NodeRed_Heishamon_control dashboard](https://github.com/edterbak/NodeRed_Heishamon_control) 
hence I need to make some changes first.

Instead of `SetXxxState` messages it uses `XxxState/set` topic to allow writing values to the heat pump in a more uniform way, 
e.g. instead of `panasonic_heat_pump/commands/SetHeatpump` it uses `panasonic_heat_pump/main/Heatpump_State/set` topic 
where `panasonic_heat_pump/main/Heatpump_State` can used to read the value from the heat pump.

It's worth to mention that this version of GoHeishaMon can be compiled and used with lsochanowski firmware as it still fits 
the dual-side Flash layout.
