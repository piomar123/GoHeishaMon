# What is this fork about

My setup for the Panasonic CZ-TAW1 (Aquarea) adapter: the
[lsochanowski](https://github.com/lsochanowski/GoHeishaMon) firmware, which keeps the original
Panasonic firmware on the other flash side, running the GoHeishaMon app from
[pando85](https://github.com/pando85/GoHeishaMon), the actively maintained continuation of
lsochanowski's app. Fixes go upstream to pando85 where they make sense
([#1](https://github.com/pando85/GoHeishaMon/pull/1), [#2](https://github.com/pando85/GoHeishaMon/pull/2), [#7](https://github.com/pando85/GoHeishaMon/pull/7), [#8](https://github.com/pando85/GoHeishaMon/pull/8),
[#9](https://github.com/pando85/GoHeishaMon/pull/9) merged).

GitHub shows this repository as a fork of rondoval/GoHeishaMon for historical reasons; `main` is
rondoval's code plus this README and isn't what I run. The work is on these branches:

| Branch | Base | What |
|---|---|---|
| [wip/lsochanowski](https://github.com/piomar123/GoHeishaMon/tree/wip/lsochanowski) | lsochanowski | firmware with the app in overlayfs (replaceable without reflashing) |
| [feat/lsochanowski/procd-service](https://github.com/piomar123/GoHeishaMon/tree/feat/lsochanowski/procd-service) | wip/lsochanowski | procd service with respawn and crash logs, LEDs (bottom: heat-pump packets, middle: app running), how to update the binary in the overlay |
| [deploy/aquarea](https://github.com/piomar123/GoHeishaMon/tree/deploy/aquarea) | pando85 main | what runs on my device: pando85 with my merged fixes (#7-#9), plus the go1.24 pin for MIPS and the heartbeat file for the LED |
| [feat/pando/set-curves](https://github.com/piomar123/GoHeishaMon/tree/feat/pando/set-curves) | pando85 1.2.0 | HeishaMon `SetCurves` command, fix for swapped curve topics (not deployed yet) |
| [fix/pando/races-and-deps](https://github.com/piomar123/GoHeishaMon/tree/fix/pando/races-and-deps) | pando85 1.2.0 | previous deployed app: race fixes, dependencies, go1.24 pin, heartbeat |

Goals:

* [x] keep the lsochanowski firmware and its dual-side flash layout, app in the overlay
* [x] app based on pando85, with the fixes offered upstream
* [ ] HeishaMon-compatible [command topics](https://github.com/Egyras/HeishaMon/blob/master/MQTT-Topics.md#command-topics):
    * [x] raw decoded values, `SetZ1HeatRequestTemperature` and the other request temperatures
    * [x] `SetCurves` (feat/pando/set-curves)
* [ ] newer OpenWrt while keeping the dual-side layout: not possible with a current kernel
  (e.g. 6.6 is ~2.6 MB, the kernel slots are 1472 KB); rondoval's full OpenWrt port (below)
  is the way to a current system, at the cost of the original firmware side

## Recent updates (2026-10)

* Serial link recovery: when the USB serial adapter dropped, the app never reconnected (reads gave
  no error, and the adapter came back as `/dev/ttyUSB1`), so data stayed off until a restart. It
  now closes the port at once and reopens it with backoff; tested by unbinding the adapter.
  Queued commands are sent in arrival order (pando85 #9).
* Data race fixes: the previous app build crashed with memory corruption after ~56 h; the fixed
  build ran 7 days from RAM without a crash and is now in the overlay.
* procd service: respawn 30 s after an exit, syslog saved to `/tmp/goheishamon-crashes/` after a
  crash, logd buffer raised to 64 KB.
* Bottom LED: it used to follow the ttyS0 console line, not the heat-pump link; it now blinks
  while valid packets arrive (GoHeishaMon touches `/tmp/goheishamon.packet` on each one).
  The middle LED now also counts a renamed test binary (e.g. run from `/tmp`) as running.
* Docs for updating the binary in the ~2 MB JFFS2 overlay (delete, let the GC erase, copy).
* `SetCurves` and a fix for outside high/low being swapped for Z1 cool, Z2 heat and Z2 cool.
* Found why SSH and LuCI don't work after the first boot of the lsochanowski firmware: the
  Panasonic `fwupdate uci-load` (S09fw) wipes `/etc/config`, which hides the image's `dropbear`
  and `uhttpd` configs; LuCI works again after `cp /rom/etc/config/uhttpd /etc/config/`.

# What's running on my Aquarea (as of 2026-11-01)

* **Firmware:** lsochanowski-based firmware from
  [wip/lsochanowski](https://github.com/piomar123/GoHeishaMon/tree/wip/lsochanowski)
  (8c1efcf) - the app binary lives in overlayfs (`/usr/bin/GoHeishaMon_MIPSUPX`) instead of ROM,
  so it can be replaced without reflashing. The overlay is only ~2 MB, so the UPX-packed binary
  must stay below ~1.8 MB.
* **Service scripts (overlay):** from
  [feat/lsochanowski/procd-service](https://github.com/piomar123/GoHeishaMon/tree/feat/lsochanowski/procd-service)
  - GoHeishaMon runs as a procd service (`/etc/init.d/goheishamon`, started from rc.local),
  respawned 30 s after it exits; after an abnormal exit the syslog is saved to
  `/tmp/goheishamon-crashes/`. Logs: `logread | grep goheisha`. `goheishamon-run.sh` (fa72a91)
  runs the app with `GODEBUG=asyncpreemptoff=1`: Go's async preemption signals corrupt the heap
  now and then on this kernel (3.14, MIPS 24Kc), as in
  [golang/go#58212](https://github.com/golang/go/issues/58212). `check_buttons.sh` (ddd9db8)
  blinks the bottom LED (1 s on, 1 s off) while packets arrive from the heat pump and lights the
  middle LED white while GoHeishaMon runs.
* **App (overlay, since 2026-11-01):** [deploy/aquarea](https://github.com/piomar123/GoHeishaMon/tree/deploy/aquarea)
  at a6beb08 - pando85 main (561bcc8) with the missing topics (TOP95-143, decoding fixes), a
  heartbeat file (`/tmp/goheishamon.packet`) touched on every heat-pump packet, the read
  statistics log fix, and a 1 s limit on waiting for MQTT publish tokens (paho v1.5.0 can lose
  a QoS 0 token when the connection drops and the serial reader then blocked for good). Built
  with go1.24 + UPX 4.2.4.
* **Earlier apps:** [fix/pando/races-and-deps](https://github.com/piomar123/GoHeishaMon/tree/fix/pando/races-and-deps)
  d4d3d3f (overlay 2026-10-09 to 2026-11-01), deploy/aquarea 19c5d11 (from RAM; crashed 3 times
  with async preemption on, then ran 13 days with it off).
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

A completely rewritten GoHeishaMon app with a different approach to reading and writing
settings: instead of `SetXxx` command topics it uses `XxxState/set`, e.g.
`panasonic_heat_pump/main/Heatpump_State/set` next to the `panasonic_heat_pump/main/Heatpump_State`
state topic. It doesn't work with the
[NodeRed_Heishamon_control dashboard](https://github.com/edterbak/NodeRed_Heishamon_control)
without changes.

rondoval also maintains a full OpenWrt port for the CZ-TAW1
([rondoval/openwrt](https://github.com/rondoval/openwrt), branches `panasonic-cz-taw1-*`, up to
OpenWrt 24.10 with kernel 6.6): one firmware partition instead of the two Panasonic sides, so
much more space and a current system, but the original firmware is gone and installing needs a
serial console and TFTP. The images can also be booted from RAM over TFTP without touching the
flash, which is a safe way to try them.

# pando85/GoHeishaMon

https://github.com/pando85/GoHeishaMon

Continues lsochanowski's app (same `SetXxx` command topics, so the NodeRed dashboard works),
with releases up to 1.2.0 (serial communication rewrite). This is the app I run, from the
deploy/aquarea branch.
