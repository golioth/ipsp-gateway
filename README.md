# 6LoWPAN IPSP Gateway to Golioth platform

### Pre-requisites

- Linux host
  - Make sure the Linux kernel has been built with Bluetooth 6LoWPAN module.
  - On our testing, kernel 5.0+ is required on a Raspberry Pi running Raspian. Update with `sudo rpi-update` if needed.
- Go installed on the system
  - Check [Go download](https://golang.org/dl/) page for latest
  - `wget https://golang.org/dl/go1.16.6.linux-armv6l.tar.gz`
  - `sudo tar -C /usr/local -xzf go1.16.6.linux-armv6l.tar.gz`
  - Add `PATH=$PATH:/usr/local/go/bin` to `~/.profile`
- `socat` tool installed
  - `sudo apt-get install socat`

### Installing and running

Install it with:

```
$ go install github.com/golioth/ipsp-gateway
```

Or you can download this git repo and compile if manually with:

```
$ go build main.go
$ sudo ./main
```

After installing it, run the cli with `sudo`. `sudo` is required due to the kernel interaction for 6LoWPAN.

```
sudo ipsp-gateway
```

### References

- Our Golioth LightDB Sample - https://github.com/golioth/samples/blob/ipsp/lightdb-led-ipsp
- Zephyr IPSP Sample - https://docs.zephyrproject.org/latest/samples/bluetooth/ipsp/README.html
- Update kernel - https://stackoverflow.com/questions/51700383/multi-connection-of-ble-6lowpan-border-router
