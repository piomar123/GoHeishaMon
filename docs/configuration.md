# Configuration

## SSH Connection

SSH was not working and dropbear doesn't started automatically. The solution was to start it through
MQTT messages. Topic for sending messages: `panasonic_heat_pump/commands/OSCommand` Topic for
reading output: `panasonic_heat_pump/commands/OSCommand/out`

Just send a `/usr/sbin/dropbear`. Example:

```bash
mosquitto_pub -t "panasonic_heat_pump/commands/OSCommand" -m "/usr/sbin/dropbear" -h <MQTT BROKER IP>
```

For connecting to ssh weaker algorithms are needed:

```bash
ssh -oHostkeyAlgorithms=+ssh-rsa -oKexAlgorithms=+diffie-hellman-group1-sha1 root@${PANASONIC_IP}
```

## Changing hostname

`/etc/config/system`

```bash
uci set system.@system[0].hostname='cz-taw1b'
uci commit system
/etc/init.d/system reload
```

## Fix dropbear

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

Add RSA key to `/etc/dropbear/authorized_keys` or use LuCi web UI.

## Configure NTP

Change NTP servers to your preferred ones from LuCi web UI.

## Important files

- `OS/RootFS/etc/gh/nextboot.sh.disabled`
- `OS/RootFS/etc/rc.local`
- `OS/RootFS/usr/bin/check_buttons.sh`

## Check logs

```bash
logread
```
