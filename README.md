# What is this fork about

Trying to join [lsochanowski](https://github.com/lsochanowski/GoHeishaMon) and [rondoval](https://github.com/rondoval/GoHeishaMon) efforts into one:

* [ ] keep simplicity of firmware modification with sysupgrade from USB (lsochanowski)
* [ ] use new sensors and bugfixes from rondoval
* [ ] adjust GoHeishaMon to be more compatible with HeishaMon:
    * [x] raw decoded values
    * [ ] handle [Set command topics](https://github.com/Egyras/HeishaMon/blob/master/MQTT-Topics.md#command-topics)
* [ ] (if possible) upgrade OpenWRT to a newer version (while keeping two-sided memory layout)


# lsochanowski/GoHeishaMon

https://github.com/lsochanowski/GoHeishaMon

This repository is outdated and seems abandoned but includes a neat way to modify the firmware while keeping the original firmware on the other memory side.

# rondoval/GoHeishaMon

https://github.com/rondoval/GoHeishaMon

This fork contains a completely rewritten GoHeishaMon app with a different approach to reading and writing settings to Panasonic heat pumps.

Instead of `SetXxxState` messages it uses `XxxState/set` topic to allow writing values to the heat pump in a more uniform way, 
e.g. instead of `panasonic_heat_pump/commands/SetHeatpump` it uses `panasonic_heat_pump/main/Heatpump_State/set` topic 
where `panasonic_heat_pump/main/Heatpump_State` can used to read the value from the heat pump.

